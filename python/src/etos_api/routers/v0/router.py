# Copyright Axis Communications AB.
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
import logging
import os
from uuid import uuid4

from eiffellib.events import EiffelTestExecutionRecipeCollectionCreatedEvent
from etos_lib import ETOS
from etos_lib.kubernetes import Kubernetes
from fastapi import FastAPI, HTTPException
from starlette.responses import Response
from kubernetes import client
from opentelemetry import trace
from opentelemetry.trace import Span
import requests

from etos_api.library.environment import Configuration, configure_testrun
from etos_api.library.utilities import sync_to_async
from etos_api.library.validator import SuiteValidator

from .schemas import AbortEtosResponse, StartEtosRequest, StartEtosResponse
from .utilities import wait_for_artifact_created

ETOSv0 = FastAPI(
    title="ETOS",
    version="v0",
    summary="API endpoints for ETOS v0 - I.e. the version before versions",
    root_path_in_servers=False,
)
TRACER = trace.get_tracer("etos_api.routers.etos.router")
LOGGER = logging.getLogger(__name__)
logging.getLogger("pika").setLevel(logging.WARNING)


async def download_suite(test_suite_url: str) -> dict:
    """Attempt to download suite.

    :param test_suite_url: URL to test suite to download.
    :return: Downloaded test suite as JSON.
    """
    try:
        suite = requests.get(test_suite_url, timeout=60)
        suite.raise_for_status()
    except Exception as exception:  # pylint:disable=broad-except
        raise AssertionError(f"Unable to download suite from {test_suite_url}") from exception
    return suite.json()


async def validate_suite(test_suite_url: str) -> None:
    """Validate the ETOS test suite through the SuiteValidator.

    :param test_suite_url: The URL to the test suite to validate.
    """
    span = trace.get_current_span()

    try:
        test_suite = await download_suite(test_suite_url)
        await SuiteValidator().validate(test_suite)
    except AssertionError as exception:
        LOGGER.error("Test suite validation failed!")
        LOGGER.error(exception)
        span.add_event("Test suite validation failed")
        raise HTTPException(
            status_code=400, detail=f"Test suite validation failed. {exception}"
        ) from exception


async def _start(etos: StartEtosRequest, span: Span) -> dict:
    """Start ETOS execution.

    :param etos: ETOS pydantic model.
    :param span: An opentelemetry span for tracing.
    :return: JSON dictionary with response.
    """
    tercc = EiffelTestExecutionRecipeCollectionCreatedEvent()
    LOGGER.identifier.set(tercc.meta.event_id)
    span.set_attribute("etos.id", tercc.meta.event_id)

    LOGGER.info("Validating test suite.")
    await validate_suite(etos.test_suite_url)
    LOGGER.info("Test suite validated.")

    etos_library = ETOS("ETOS API", os.getenv("HOSTNAME"), "ETOS API")
    await sync_to_async(etos_library.config.rabbitmq_publisher_from_environment)

    LOGGER.info("Get artifact created %r", (etos.artifact_identity or str(etos.artifact_id)))
    try:
        artifact = await wait_for_artifact_created(
            etos_library, etos.artifact_identity, etos.artifact_id
        )
    except Exception as exception:  # pylint:disable=broad-except
        LOGGER.critical(exception)
        raise HTTPException(
            status_code=400, detail=f"Could not connect to GraphQL. {exception}"
        ) from exception
    if artifact is None:
        identity = etos.artifact_identity or str(etos.artifact_id)
        raise HTTPException(
            status_code=400,
            detail=f"Unable to find artifact with identity '{identity}'",
        )
    LOGGER.info("Found artifact created %r", artifact)
    # There are assumptions here. Since "edges" list is already tested
    # and we know that the return from GraphQL must be 'node'.'meta'.'id'
    # if there are "edges", this is fine.
    # Same goes for 'data'.'identity'.
    artifact_id = artifact[0]["node"]["meta"]["id"]
    identity = artifact[0]["node"]["data"]["identity"]
    span.set_attribute("etos.artifact.id", artifact_id)
    span.set_attribute("etos.artifact.identity", identity)

    links = {"CAUSE": artifact_id}
    data = {
        "selectionStrategy": {"tracker": "Suite Builder", "id": str(uuid4())},
        "batchesUri": etos.test_suite_url,
    }
    config = Configuration(
        suite_id=tercc.meta.event_id,
        dataset=etos.dataset,
        execution_space_provider=etos.execution_space_provider,
        iut_provider=etos.iut_provider,
        log_area_provider=etos.log_area_provider,
    )
    try:
        await configure_testrun(config)
    except AssertionError as exception:
        LOGGER.critical(exception)
        raise HTTPException(
            status_code=400,
            detail=f"Could not configure environment provider. {exception}",
        ) from exception
    LOGGER.info("Environment provider configured.")

    LOGGER.info("Start event publisher.")
    await sync_to_async(etos_library.start_publisher)
    if not etos_library.debug.disable_sending_events:
        await sync_to_async(etos_library.publisher.wait_start)
    LOGGER.info("Event published started successfully.")
    LOGGER.info("Publish TERCC event.")
    try:
        event = etos_library.events.send(tercc, links, data)
        await sync_to_async(etos_library.publisher.wait_for_unpublished_events)
    finally:
        if not etos_library.debug.disable_sending_events:
            await sync_to_async(etos_library.publisher.stop)
            await sync_to_async(etos_library.publisher.wait_close)
    LOGGER.info("Event published.")

    LOGGER.info("ETOS triggered successfully.")
    return {
        "tercc": event.meta.event_id,
        "artifact_id": artifact_id,
        "artifact_identity": identity,
        "event_repository": etos_library.debug.graphql_server,
    }


async def _abort(suite_id: str) -> dict:
    kubernetes = Kubernetes()

    batch_api = client.BatchV1Api()
    jobs = batch_api.list_namespaced_job(namespace=kubernetes.namespace)

    delete_options = client.V1DeleteOptions(
        propagation_policy="Background"  # asynchronous cascading deletion
    )

    for job in jobs.items:
        if (
            job.metadata.labels.get("app") == "suite-runner"
            and job.metadata.labels.get("id") == suite_id
        ):
            batch_api.delete_namespaced_job(
                name=job.metadata.name, namespace=kubernetes.namespace, body=delete_options
            )
            LOGGER.info("Deleted suite-runner job: %s", job.metadata.name)
            break
    else:
        raise HTTPException(status_code=404, detail="Suite ID not found.")

    return {"message": f"Abort triggered for suite id: {suite_id}."}


@ETOSv0.get("/etos/ping", tags=["etos"], status_code=204)
async def ping():
    """Ping the ETOS service in order to check if it is up and running.

    :return: HTTP 204 response.
    :rtype: :obj:`starlette.responses.Response`
    """
    return Response(status_code=204)


@ETOSv0.post("/etos", tags=["etos"], response_model=StartEtosResponse)
async def start_etos(etos: StartEtosRequest):
    """Start ETOS execution on post.

    :param etos: ETOS pydantic model.
    :type etos: :obj:`etos_api.routers.etos.schemas.StartEtosRequest`
    :return: JSON dictionary with response.
    :rtype: dict
    """
    with TRACER.start_as_current_span("start-etos") as span:
        return await _start(etos, span)


@ETOSv0.delete("/etos/{suite_id}", tags=["etos"], response_model=AbortEtosResponse)
async def abort_etos(suite_id: str):
    """Abort ETOS execution on delete.

    :param suite_id: ETOS suite id
    :type suite_id: str
    :return: JSON dictionary with response.
    :rtype: dict
    """
    with TRACER.start_as_current_span("abort-etos"):
        return await _abort(suite_id)
