# Copyright 2020 Axis Communications AB.
#
# For a full list of individual contributors, please see the commit history.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""ETOS API router."""
from uuid import uuid4
import os
from fastapi import APIRouter, HTTPException
from etos_lib import ETOS
from eiffellib.events import EiffelTestExecutionRecipeCollectionCreatedEvent

from etos_api.library.graphql import GraphqlQueryHandler
from etos_api.library.graphql_queries import ARTIFACT_QUERY
from etos_api.library.validator import SuiteValidator
from etos_api.library.utilities import sync_to_async, aclosing
from etos_api.routers.environment_provider.router import configure_environment_provider
from etos_api.routers.environment_provider.schemas import (
    ConfigureEnvironmentProviderRequest,
)
from .schemas import StartEtosRequest, StartEtosResponse

ROUTER = APIRouter()


@ROUTER.post("/etos", tags=["etos"], response_model=StartEtosResponse)
async def start_etos(etos: StartEtosRequest):
    """Start ETOS execution on post.

    :param etos: ETOS pydantic model.
    :type etos: :obj:`etos_api.routers.etos.schemas.StartEtosRequest`
    :return: JSON dictionary with response.
    :rtype: dict
    """
    await SuiteValidator().validate(etos.test_suite_url)

    etos_library = ETOS("ETOS API", os.getenv("HOSTNAME"), "ETOS API")
    await sync_to_async(etos_library.config.rabbitmq_publisher_from_environment)

    query_handler = GraphqlQueryHandler(etos_library)
    artifact = await query_handler.execute(ARTIFACT_QUERY % etos.artifact_identity)
    if artifact is None:
        raise HTTPException(
            status_code=400,
            detail=f"Unable to find artifact with identity '{etos.artifact_identity}'",
        )
    # There are assumptions here. Since "edges" list is already tested
    # and we know that the return from GraphQL must be 'node'.'meta'.'id'
    # if there are "edges", this is fine.
    artifact_id = artifact["artifactCreated"]["edges"][0]["node"]["meta"]["id"]

    links = {"CAUSE": artifact_id}
    data = {
        "selectionStrategy": {"tracker": "Suite Builder", "id": str(uuid4())},
        "batchesUri": etos.test_suite_url,
    }
    tercc = EiffelTestExecutionRecipeCollectionCreatedEvent()
    request = ConfigureEnvironmentProviderRequest(
        suite_id=tercc.meta.event_id,
        dataset=etos.dataset,
        execution_space_provider=etos.execution_space_provider,
        iut_provider=etos.iut_provider,
        log_area_provider=etos.log_area_provider,
    )
    await configure_environment_provider(request)

    await sync_to_async(etos_library.start_publisher)
    async with aclosing(etos_library.publisher):
        event = etos_library.events.send(tercc, links, data)

    return {
        "tercc": event.meta.event_id,
        "event_repository": etos_library.debug.graphql_server,
    }
