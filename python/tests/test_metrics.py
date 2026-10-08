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
"""ETOS API request metrics tests."""

import asyncio
import logging
import sys
from unittest import TestCase
from unittest.mock import MagicMock, patch

from fastapi.testclient import TestClient
from prometheus_client import REGISTRY

from etos_api.library.metrics import TIME_REQUESTS
from etos_api.main import APP

logging.basicConfig(level=logging.DEBUG, stream=sys.stdout)

SUBSUITE_LABELS = {
    "endpoint": "/api/v1beta1/testrun/{sub_suite_id}",
    "operation": "get_subsuite",
}


def _requests_total(status: int, labels: dict = None) -> float:
    """Get the current value of http_requests_total for labels and status."""
    labels = labels or SUBSUITE_LABELS
    value = REGISTRY.get_sample_value("http_requests_total", {**labels, "status": str(status)})
    return value or 0.0


def _duration_count(labels: dict = None) -> float:
    """Get the current observation count of http_request_duration_seconds for labels."""
    labels = labels or SUBSUITE_LABELS
    value = REGISTRY.get_sample_value("http_request_duration_seconds_count", labels)
    return value or 0.0


class TestMetrics(TestCase):
    """Test that ETOS API routes record request metrics."""

    client = TestClient(APP, raise_server_exceptions=False)

    def _get_subsuite(self, get_mock: MagicMock) -> int:
        """Request a v1beta1 sub suite with a mocked Environment client and return the status."""
        with (
            patch("etos_api.routers.v1beta1.router.Kubernetes"),
            patch("etos_api.routers.v1beta1.router.Environment") as environment_mock,
        ):
            environment_mock.return_value.get = get_mock
            response = self.client.get("/api/v1beta1/testrun/my-environment")
        return response.status_code

    def test_successful_request_is_counted_and_timed(self):
        """Test that a successful route request records request metrics.

        Approval criteria:
            - http_requests_total with status 200 shall increase by one.
            - http_request_duration_seconds shall get one more observation.

        Test steps::
            1. Request an existing v1beta1 sub suite.
            2. Verify that the request was counted and timed.
        """
        resource = MagicMock()
        resource.to_dict.return_value = {"spec": {"name": "suite"}}
        before_total = _requests_total(200)
        before_count = _duration_count()

        self.assertEqual(self._get_subsuite(MagicMock(return_value=resource)), 200)

        self.assertEqual(_requests_total(200), before_total + 1)
        self.assertEqual(_duration_count(), before_count + 1)

    def test_http_exception_is_counted_with_its_status(self):
        """Test that a route raising an HTTPException records its status code.

        Approval criteria:
            - http_requests_total with status 404 shall increase by one.
            - http_request_duration_seconds shall get one more observation.

        Test steps::
            1. Request a v1beta1 sub suite that does not exist.
            2. Verify that the request was counted with status 404 and timed.
        """
        before_total = _requests_total(404)
        before_count = _duration_count()

        self.assertEqual(self._get_subsuite(MagicMock(return_value=None)), 404)

        self.assertEqual(_requests_total(404), before_total + 1)
        self.assertEqual(_duration_count(), before_count + 1)

    def test_unhandled_exception_is_counted_as_500(self):
        """Test that a route raising an unhandled exception is counted as status 500.

        Approval criteria:
            - The request shall return 500.
            - http_requests_total with status 500 shall increase by one.
            - http_request_duration_seconds shall get one more observation.

        Test steps::
            1. Request a v1beta1 sub suite where the Environment client fails.
            2. Verify that the request was counted with status 500 and timed.
        """
        before_total = _requests_total(500)
        before_count = _duration_count()

        self.assertEqual(self._get_subsuite(MagicMock(side_effect=RuntimeError("boom"))), 500)

        self.assertEqual(_requests_total(500), before_total + 1)
        self.assertEqual(_duration_count(), before_count + 1)

    def test_v0_and_v1alpha_routes_are_instrumented(self):
        """Test that v0 and v1alpha routes record request metrics.

        Approval criteria:
            - http_requests_total shall increase for the v0 stop and v1alpha sub suite routes.

        Test steps::
            1. Request a v0 abort and a v1alpha sub suite that do not exist.
            2. Verify that each request was counted with its status code.
        """
        v0_labels = {"endpoint": "/api/v0/etos/{suite_id}", "operation": "stop_testrun"}
        v1alpha_labels = {
            "endpoint": "/api/v1alpha/testrun/{suite_id}",
            "operation": "get_subsuite",
        }
        with patch("etos_api.routers.v0.router._abort", side_effect=RuntimeError("boom")):
            before = _requests_total(500, v0_labels)
            self.assertEqual(self.client.delete("/api/v0/etos/my-suite").status_code, 500)
            self.assertEqual(_requests_total(500, v0_labels), before + 1)
        with (
            patch("etos_api.routers.v1alpha.router.Kubernetes"),
            patch("etos_api.routers.v1alpha.router.Environment") as environment_mock,
        ):
            environment_mock.return_value.get.return_value = None
            before = _requests_total(404, v1alpha_labels)
            self.assertEqual(self.client.get("/api/v1alpha/testrun/my-env").status_code, 404)
            self.assertEqual(_requests_total(404, v1alpha_labels), before + 1)

    def test_time_requests_measures_awaited_duration(self):
        """Test that TIME_REQUESTS observes the time spent awaiting the coroutine.

        Approval criteria:
            - The observed duration shall include the time the coroutine awaited.

        Test steps::
            1. Decorate a coroutine that sleeps with TIME_REQUESTS.
            2. Run the coroutine.
            3. Verify that the observed duration sum increased by at least the sleep time.
        """
        labels = {"endpoint": "/test/time_requests", "operation": "get_subsuite"}

        @TIME_REQUESTS(labels)
        async def sleeper():
            await asyncio.sleep(0.05)
            return "done"

        before = REGISTRY.get_sample_value("http_request_duration_seconds_sum", labels) or 0.0
        self.assertEqual(asyncio.run(sleeper()), "done")
        after = REGISTRY.get_sample_value("http_request_duration_seconds_sum", labels)
        self.assertGreaterEqual(after - before, 0.05)
