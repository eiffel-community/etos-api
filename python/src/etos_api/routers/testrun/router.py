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
"""ETOS testrun router."""
import logging
import os
from uuid import uuid4
from typing import Any

import requests
from eiffellib.events import EiffelTestExecutionRecipeCollectionCreatedEvent
from etos_lib import ETOS
from fastapi import APIRouter, HTTPException
from kubernetes import dynamic
from kubernetes.client import api_client
from opentelemetry import trace
from opentelemetry.trace import Span

from etos_api.library.utilities import sync_to_async
from etos_api.library.validator import SuiteValidator
from etos_api.routers.lib.kubernetes import namespace

from .schemas import AbortTestrunResponse, StartTestrunRequest, StartTestrunResponse
from .testrun import TestRun, TestRunSpec, Providers
from .utilities import wait_for_artifact_created

ROUTER = APIRouter()
TRACER = trace.get_tracer("etos_api.routers.testrun.router")
LOGGER = logging.getLogger(__name__)
logging.getLogger("pika").setLevel(logging.WARNING)


async def download_suite(test_suite_url):
    """Attempt to download suite.

    :param test_suite_url: URL to test suite to download.
    :type test_suite_url: str
    :return: Downloaded test suite as JSON.
    :rtype: list
    """
    try:
        suite = requests.get(test_suite_url, timeout=60)
        suite.raise_for_status()
    except Exception as exception:  # pylint:disable=broad-except
        raise AssertionError(f"Unable to download suite from {test_suite_url}") from exception
    return suite.json()


async def validate_suite(test_suite: list[dict[str, Any]]) -> None:
    """Validate the ETOS test suite through the SuiteValidator.

    :param test_suite_url: The URL to the test suite to validate.
    """
    span = trace.get_current_span()

    try:
        await SuiteValidator().validate(test_suite)
    except AssertionError as exception:
        LOGGER.error("Test suite validation failed!")
        LOGGER.error(exception)
        span.add_event("Test suite validation failed")
        raise HTTPException(
            status_code=400, detail=f"Test suite validation failed. {exception}"
        ) from exception


async def _create_testrun(etos: StartTestrunRequest, span: Span) -> dict:
    """Create a testrun for ETOS to execute.

    :param etos: Testrun pydantic model.
    :param span: An opentelemetry span for tracing.
    :return: JSON dictionary with response.
    """
    tercc = EiffelTestExecutionRecipeCollectionCreatedEvent()
    LOGGER.identifier.set(tercc.meta.event_id)
    span.set_attribute("etos.id", tercc.meta.event_id)

    LOGGER.info("Download test suite.")
    test_suite = await download_suite(etos.test_suite_url)
    LOGGER.info("Test suite downloaded.")

    LOGGER.info("Validating test suite.")
    await validate_suite(test_suite)
    LOGGER.info("Test suite validated.")

    etos_library = ETOS("ETOS API", os.getenv("HOSTNAME", "localhost"), "ETOS API")
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

    testrun_spec = TestRun(
        metadata={"name": f"testrun-{event.meta.event_id}", "namespace": namespace()},
        spec=TestRunSpec(
            id=event.meta.event_id,
            suiteRunnerImage=os.getenv(
                "SUITE_RUNNER_IMAGE", "registry.nordix.org/eiffel/etos-suite-runner:latest"
            ),
            artifact=artifact_id,
            identity=identity,
            providers=Providers(
                iut=etos.iut_provider,
                executionSpace=etos.execution_space_provider,
                logArea=etos.log_area_provider,
            ),
            suites=TestRunSpec.from_tercc(test_suite),
        ),
    )

    k8s = dynamic.DynamicClient(api_client.ApiClient())
    testrun = k8s.resources.get(
        api_version="etos.eiffel-community.github.io/v1alpha1", kind="TestRun"
    )
    testrun.create(body=testrun_spec.model_dump())

    LOGGER.info("ETOS triggered successfully.")
    return {
        "tercc": event.meta.event_id,
        "artifact_id": artifact_id,
        "artifact_identity": identity,
        "event_repository": etos_library.debug.graphql_server,
    }


async def _abort(suite_id: str) -> dict:
    """Abort a testrun by deleting the testrun resource."""
    ns = namespace()

    k8s = dynamic.DynamicClient(api_client.ApiClient())
    testrun_resource = k8s.resources.get(
        api_version="etos.eiffel-community.github.io/v1alpha1", kind="TestRun"
    )
    if testrun_resource.get(namespace=ns, name=f"testrun-{suite_id}"):
        testrun_resource.delete(namespace=ns, name=f"testrun-{suite_id}")
    else:
        raise HTTPException(status_code=404, detail="Suite ID not found.")

    return {"message": f"Abort triggered for suite id: {suite_id}."}


@ROUTER.post("/v1alpha/testrun", tags=["etos", "testrun"], response_model=StartTestrunResponse)
async def start_testrun(etos: StartTestrunRequest):
    """Start ETOS testrun on post.

    :param etos: ETOS pydantic model.
    :type etos: :obj:`etos_api.routers.etos.schemas.StartTestrunRequest`
    :return: JSON dictionary with response.
    :rtype: dict
    """
    with TRACER.start_as_current_span("start-etos") as span:
        return await _create_testrun(etos, span)


@ROUTER.delete("/v1alpha/testrun/{suite_id}", tags=["etos"], response_model=AbortTestrunResponse)
async def abort_testrun(suite_id: str):
    """Abort ETOS testrun on delete.

    :param suite_id: ETOS suite id
    :type suite_id: str
    :return: JSON dictionary with response.
    :rtype: dict
    """
    with TRACER.start_as_current_span("abort-etos"):
        return await _abort(suite_id)
