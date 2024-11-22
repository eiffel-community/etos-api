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
import re
from uuid import uuid4
from typing import Any

import requests
from etos_lib import ETOS
from etos_lib.kubernetes.schemas.testrun import (
    TestRun as TestRunSchema,
    TestRunSpec,
    Providers,
    Image,
    Metadata,
    Retention,
    TestRunner,
)
from etos_lib.kubernetes import TestRun, Environment, Kubernetes
from fastapi import FastAPI, HTTPException
from starlette.responses import Response
from opentelemetry import trace
from opentelemetry.trace import Span

from etos_api.library.validator import SuiteValidator

from .schemas import AbortTestrunResponse, StartTestrunRequest, StartTestrunResponse
from .utilities import wait_for_artifact_created

ETOSv1Alpha = FastAPI(
    title="ETOS",
    version="v1alpha",
    summary="API endpoints for ETOS v1 Alpha",
    root_path_in_servers=False,
)
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


def convert_to_rfc1123(value: str) -> str:
    """Convert string to RFC-1123 accepted string.

    https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names

    Some resource types require their names to follow the DNS label standard as defined in RFC 1123.
    This means the name must:

        contain at most 63 characters
        contain only lowercase alphanumeric characters or '-'
        start with an alphanumeric character
        end with an alphanumeric character

    This method does not care about the length of the string since ETOS uses generateName for
    creating Kubernetes resources and that function will truncate the string down to 63-5 and
    then add 5 random characters.
    """
    # Replace all characters that are not alphanumeric (A-Z, a-z, 0-9) with a hyphen
    result = re.sub(r"[^A-Z\d]", "-", value, flags=re.IGNORECASE)
    # Remove leading hyphens
    result = re.sub(r"^-+", "", result)
    # Remove trailing hyphens
    result = re.sub(r"-+$", "", result)
    # Replace multiple consecutive hyphens with a single hyphen
    result = re.sub(r"-+", "-", result)
    return result.lower()


async def _create_testrun(etos: StartTestrunRequest, span: Span) -> dict:
    """Create a testrun for ETOS to execute.

    :param etos: Testrun pydantic model.
    :param span: An opentelemetry span for tracing.
    :return: JSON dictionary with response.
    """
    testrun_id = str(uuid4())
    LOGGER.identifier.set(testrun_id)
    span.set_attribute("etos.id", testrun_id)

    LOGGER.info("Download test suite.")
    test_suite = await download_suite(etos.test_suite_url)
    LOGGER.info("Test suite downloaded.")

    LOGGER.info("Validating test suite.")
    await validate_suite(test_suite)
    LOGGER.info("Test suite validated.")

    etos_library = ETOS("ETOS API", os.getenv("HOSTNAME", "localhost"), "ETOS API")

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

    try:
        # Since the TERCC that we use can have multiple names, it's quite difficult to get a
        # single name that describes the entire TERCC. However ETOS mostly only gets a single
        # test suite or gets a suite that has a similar name for all suites in the TERCC and
        # for this reason we get the name of the first suite and that should be okay.
        name = test_suite[0].get("name")
        # Convert to kubernetes accepted name
        name = convert_to_rfc1123(name)
        # Truncate and Add a hyphen at the end, if possible since it makes the generated name
        # easier to read. This truncation does not need to be validated since the generateName we
        # use when creating a TestRun will truncate the string if necessary.
        if not name.endswith("-"):
            # 63 is the max length, 5 is the random characters added by generateName and
            # 1 is to be able to fit a hyphen at the end so we truncate to 57 to fit everything.
            name = f"{name[:57]}-"
    except (IndexError, TypeError, ValueError):
        name = f"testrun-{testrun_id}-"
        LOGGER.error("Could not get name from test suite, defaulting to %s", name)

    retention = Retention(
        failure=os.getenv("TESTRUN_FAILURE_RETENTION"),
        success=os.getenv("TESTRUN_SUCCESS_RETENTION"),
    )

    kubernetes = Kubernetes()
    testrun_spec = TestRunSchema(
        metadata=Metadata(
            generateName=name,
            namespace=kubernetes.namespace,
            labels={
                "etos.eiffel-community.github.io/id": testrun_id,
                "etos.eiffel-community.github.io/cluster": os.getenv("ETOS_CLUSTER", "Unknown"),
            },
        ),
        spec=TestRunSpec(
            cluster=os.getenv("ETOS_CLUSTER", "Unknown"),
            id=testrun_id,
            retention=retention,
            suiteRunner=Image(
                image=os.getenv(
                    "SUITE_RUNNER_IMAGE", "registry.nordix.org/eiffel/etos-suite-runner:latest"
                ),
                imagePullPolicy=os.getenv("SUITE_RUNNER_IMAGE_PULL_POLICY", "IfNotPresent"),
            ),
            logListener=Image(
                image=os.getenv(
                    "LOG_LISTENER_IMAGE", "registry.nordix.org/eiffel/etos-log-listener:latest"
                ),
                imagePullPolicy=os.getenv("LOG_LISTENER_IMAGE_PULL_POLICY", "IfNotPresent"),
            ),
            environmentProvider=Image(
                image=os.getenv(
                    "ENVIRONMENT_PROVIDER_IMAGE",
                    "registry.nordix.org/eiffel/etos-environment-provider:latest",
                ),
                imagePullPolicy=os.getenv("ENVIRONMENT_PROVIDER_IMAGE_PULL_POLICY", "IfNotPresent"),
            ),
            artifact=artifact_id,
            identity=identity,
            testRunner=TestRunner(version=os.getenv("ETR_VERSION", "Unknown")),
            providers=Providers(
                iut=etos.iut_provider,
                executionSpace=etos.execution_space_provider,
                logArea=etos.log_area_provider,
            ),
            suites=TestRunSpec.from_tercc(test_suite, etos.dataset),
        ),
    )

    testrun_client = TestRun(kubernetes)
    if not testrun_client.create(testrun_spec):
        raise HTTPException("Failed to create testrun")

    LOGGER.info("ETOS triggered successfully.")
    return {
        "tercc": testrun_id,
        "artifact_id": artifact_id,
        "artifact_identity": identity,
        "event_repository": etos_library.debug.graphql_server,
    }


async def _abort(suite_id: str) -> dict:
    """Abort a testrun by deleting the testrun resource."""
    testrun_client = TestRun(Kubernetes())
    response = testrun_client.client.delete(
        type="TestRun",
        namespace=testrun_client.namespace,
        label_selector=f"etos.eiffel-community.github.io/id={suite_id}",
    )  # type:ignore
    if not response.items:
        raise HTTPException(status_code=404, detail="Suite ID not found.")
    return {"message": f"Abort triggered for suite id: {suite_id}."}


@ETOSv1Alpha.get("/ping", tags=["etos"], status_code=204)
async def health_check():
    """Check the status of the API and verify the client version."""
    return Response(status_code=204)


@ETOSv1Alpha.post("/testrun", tags=["etos"], response_model=StartTestrunResponse)
async def start_testrun(etos: StartTestrunRequest):
    """Start ETOS testrun on post.

    :param etos: ETOS pydantic model.
    :type etos: :obj:`etos_api.routers.etos.schemas.StartTestrunRequest`
    :return: JSON dictionary with response.
    :rtype: dict
    """
    with TRACER.start_as_current_span("start-etos") as span:
        return await _create_testrun(etos, span)


@ETOSv1Alpha.delete("/testrun/{suite_id}", tags=["etos"], response_model=AbortTestrunResponse)
async def abort_testrun(suite_id: str):
    """Abort ETOS testrun on delete.

    :param suite_id: ETOS suite id
    :type suite_id: str
    :return: JSON dictionary with response.
    :rtype: dict
    """
    with TRACER.start_as_current_span("abort-etos"):
        return await _abort(suite_id)


@ETOSv1Alpha.get("/testrun/{sub_suite_id}", tags=["etos"])
async def get_subsuite(sub_suite_id: str) -> dict:
    """Get sub suite returns the sub suite definition for the ETOS test runner.

    :param sub_suite_id: The name of the Environment kubernetes resource.
    :return: JSON dictionary with the Environment spec. Formatted to TERCC format.
    """
    environment_client = Environment(Kubernetes())
    environment_resource = environment_client.get(sub_suite_id)
    if not environment_resource:
        raise HTTPException(404, "Failed to get environment")
    environment_spec = environment_resource.to_dict().get("spec", {})
    recipes = await recipes_from_tests(environment_spec["recipes"])
    environment_spec["recipes"] = recipes
    return environment_spec


async def recipes_from_tests(tests: list[dict]) -> list[dict]:
    """Load Eiffel TERCC recipes from test.

    :param tests: The tests defined in a Test model.
    :return: A list of Eiffel TERCC recipes.
    """
    recipes: list[dict] = []
    for test in tests:
        recipes.append(
            {
                "id": test["id"],
                "testCase": test["testCase"],
                "constraints": [
                    {
                        "key": "ENVIRONMENT",
                        "value": test["execution"]["environment"],
                    },
                    {
                        "key": "COMMAND",
                        "value": test["execution"]["command"],
                    },
                    {
                        "key": "EXECUTE",
                        "value": test["execution"]["execute"],
                    },
                    {
                        "key": "CHECKOUT",
                        "value": test["execution"]["checkout"],
                    },
                    {
                        "key": "PARAMETERS",
                        "value": test["execution"]["parameters"],
                    },
                    {
                        "key": "TEST_RUNNER",
                        "value": test["execution"]["testRunner"],
                    },
                ],
            }
        )
    return recipes
