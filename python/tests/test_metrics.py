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
"""ETOS API request and operation metrics tests."""

import asyncio
import inspect
import json
import logging
import sys
from unittest import TestCase
from unittest.mock import MagicMock, Mock, patch

import pytest
from fastapi import Body, FastAPI, HTTPException
from fastapi.routing import APIRoute
from fastapi.testclient import TestClient
from prometheus_client import REGISTRY

from etos_api.library.metrics import COUNT_REQUESTS, outcome_for_status, record_suite_operation
from etos_api.main import APP

logging.basicConfig(level=logging.DEBUG, stream=sys.stdout)

SUBSUITE_LABELS = {
    "endpoint": "/api/v1beta1/testrun/{sub_suite_id}",
    "operation": "get_subsuite",
}
LABELS = {"endpoint": "/api/v1beta1/testrun", "operation": "start_testrun"}
CLUSTER_LABELS = {**LABELS, "etos_cluster": "unassigned"}


def _requests_total(status: int, labels: dict = None) -> float:
    """Get the current value of http_requests_total for labels and status."""
    labels = {**(labels or SUBSUITE_LABELS), "etos_cluster": "unassigned"}
    value = REGISTRY.get_sample_value("http_requests_total", {**labels, "status": str(status)})
    return value or 0.0


def _duration_count(labels: dict = None) -> float:
    """Get the current observation count of http_request_duration_seconds for labels."""
    labels = {**(labels or SUBSUITE_LABELS), "etos_cluster": "unassigned"}
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

    def test_count_requests_measures_awaited_duration(self):
        """Test that COUNT_REQUESTS observes the time spent awaiting the coroutine.

        Approval criteria:
            - The observed duration shall include the time the coroutine awaited.

        Test steps::
            1. Decorate a coroutine that sleeps with COUNT_REQUESTS.
            2. Run the coroutine.
            3. Verify that the observed duration sum increased by at least the sleep time.
        """
        labels = {"endpoint": "/test/time_requests", "operation": "get_subsuite"}

        @COUNT_REQUESTS(labels, logging.getLogger(__name__), "v1beta1")
        async def sleeper():
            await asyncio.sleep(0.05)
            return "done"

        labels = {**labels, "etos_cluster": "unassigned"}
        before = REGISTRY.get_sample_value("http_request_duration_seconds_sum", labels) or 0.0
        self.assertEqual(asyncio.run(sleeper()), "done")
        after = REGISTRY.get_sample_value("http_request_duration_seconds_sum", labels)
        self.assertGreaterEqual(after - before, 0.05)


@pytest.fixture(autouse=True)
def explicit_unassigned_cluster(monkeypatch):
    """Keep existing operation tests independent of the host environment."""
    monkeypatch.setattr("etos_api.library.metrics.cluster_identity", lambda: "unassigned")


def test_status_outcomes_are_bounded():
    """Verify HTTP statuses map to the bounded operation outcome vocabulary."""
    assert outcome_for_status(200) == "success"
    assert outcome_for_status(404) == "not_found"
    assert outcome_for_status(422) == "validation_rejected"
    assert outcome_for_status(504) == "timed_out"
    assert outcome_for_status(503) == "server_error"


def test_suite_operation_records_bounded_labels_and_duration():
    """Verify suite observations record only bounded labels and elapsed duration."""
    counter = Mock()
    histogram = Mock()
    with (
        patch("etos_api.library.metrics.SUITE_OPERATIONS_TOTAL", counter),
        patch("etos_api.library.metrics.SUITE_OPERATION_DURATION", histogram),
    ):
        record_suite_operation("v1beta1", "suite_download", "client_error", 1.25)

    labels = {
        "etos_cluster": "unassigned",
        "api_version": "v1beta1",
        "operation": "suite_download",
        "outcome": "client_error",
    }
    counter.labels.assert_called_once_with(**labels)
    counter.labels.return_value.inc.assert_called_once_with()
    histogram.labels.assert_called_once_with(**labels)
    histogram.labels.return_value.observe.assert_called_once_with(1.25)


def test_suite_operation_rejects_unbounded_label_values():
    """Verify unsupported suite dimensions are rejected before metric emission."""
    for values in (
        ("v1", "suite_download", "success"),
        ("v1alpha", "download", "success"),
        ("v1alpha", "suite_validation", "failure"),
    ):
        with pytest.raises(ValueError):
            record_suite_operation(*values, duration=0.1)


@pytest.mark.asyncio
async def test_count_requests_records_versioned_success_and_legacy_counter():
    """Verify successful operations emit versioned and legacy observations once."""
    request_counter = Mock()
    request_time = Mock()
    api_counter = Mock()
    api_duration = Mock()
    logger = Mock()

    @COUNT_REQUESTS(LABELS, logger, "v1beta1")
    async def operation():
        return {"created": True}

    with (
        patch("etos_api.library.metrics.REQUEST_TIME", request_time),
        patch("etos_api.library.metrics.REQUESTS_TOTAL", request_counter),
        patch("etos_api.library.metrics.API_REQUESTS_TOTAL", api_counter),
        patch("etos_api.library.metrics.API_REQUEST_DURATION", api_duration),
        patch("etos_api.library.metrics.perf_counter", side_effect=[10.0, 11.25]),
    ):
        assert await operation() == {"created": True}

    request_time.labels.assert_called_once_with(**CLUSTER_LABELS)
    request_time.labels.return_value.observe.assert_called_once_with(1.25)
    request_counter.labels.assert_called_once_with(**CLUSTER_LABELS, status=200)
    request_counter.labels.return_value.inc.assert_called_once_with()
    api_counter.labels.assert_called_once_with(
        etos_cluster="unassigned",
        api_version="v1beta1",
        operation="start_testrun",
        status_class="2xx",
        outcome="success",
    )
    api_counter.labels.return_value.inc.assert_called_once_with()
    api_duration.labels.assert_called_once_with(
        api_version="v1beta1",
        operation="start_testrun",
        outcome="success",
        etos_cluster="unassigned",
    )
    api_duration.labels.return_value.observe.assert_called_once_with(1.25)
    logger.exception.assert_not_called()


@pytest.mark.asyncio
async def test_count_requests_records_http_exception_and_preserves_legacy_status():
    """Verify HTTP errors preserve their status and emit the normalized outcome."""
    request_counter = Mock()
    api_counter = Mock()
    api_duration = Mock()
    logger = Mock()

    @COUNT_REQUESTS(LABELS, logger, "v1alpha")
    async def operation():
        raise HTTPException(status_code=404, detail="Not found")

    with (
        patch("etos_api.library.metrics.REQUESTS_TOTAL", request_counter),
        patch("etos_api.library.metrics.API_REQUESTS_TOTAL", api_counter),
        patch("etos_api.library.metrics.API_REQUEST_DURATION", api_duration),
        patch("etos_api.library.metrics.perf_counter", side_effect=[20.0, 20.5]),
    ):
        with pytest.raises(HTTPException) as exception:
            await operation()
    assert exception.value.status_code == 404

    request_counter.labels.assert_called_once_with(**CLUSTER_LABELS, status=404)
    request_counter.labels.return_value.inc.assert_called_once_with()
    api_counter.labels.assert_called_once_with(
        etos_cluster="unassigned",
        api_version="v1alpha",
        operation="start_testrun",
        status_class="4xx",
        outcome="not_found",
    )
    api_counter.labels.return_value.inc.assert_called_once_with()
    api_duration.labels.assert_called_once_with(
        api_version="v1alpha",
        operation="start_testrun",
        outcome="not_found",
        etos_cluster="unassigned",
    )
    api_duration.labels.return_value.observe.assert_called_once_with(0.5)
    logger.exception.assert_not_called()


@pytest.mark.asyncio
async def test_count_requests_records_unhandled_exception_as_server_error():
    """Verify unexpected errors are logged, re-raised, and counted as server errors."""
    request_counter = Mock()
    api_counter = Mock()
    api_duration = Mock()
    logger = Mock()

    @COUNT_REQUESTS(LABELS, logger, "v1beta1")
    async def operation():
        raise RuntimeError("unexpected")

    with (
        patch("etos_api.library.metrics.REQUESTS_TOTAL", request_counter),
        patch("etos_api.library.metrics.API_REQUESTS_TOTAL", api_counter),
        patch("etos_api.library.metrics.API_REQUEST_DURATION", api_duration),
        patch("etos_api.library.metrics.perf_counter", side_effect=[30.0, 32.0]),
    ):
        with pytest.raises(RuntimeError, match="unexpected"):
            await operation()

    request_counter.labels.assert_called_once_with(**CLUSTER_LABELS, status=500)
    request_counter.labels.return_value.inc.assert_called_once_with()
    api_counter.labels.assert_called_once_with(
        etos_cluster="unassigned",
        api_version="v1beta1",
        operation="start_testrun",
        status_class="5xx",
        outcome="server_error",
    )
    api_counter.labels.return_value.inc.assert_called_once_with()
    api_duration.labels.assert_called_once_with(
        api_version="v1beta1",
        operation="start_testrun",
        outcome="server_error",
        etos_cluster="unassigned",
    )
    api_duration.labels.return_value.observe.assert_called_once_with(2.0)
    logger.exception.assert_called_once_with("Unhandled exception occurred, setting status to 500")


@pytest.mark.asyncio
async def test_fastapi_registered_handler_counts_reached_operation_only_once():
    """Verify registered handlers count once while framework rejects remain excluded."""
    app = FastAPI()
    request_counter = Mock()
    api_counter = Mock()
    api_duration = Mock()
    logger = Mock()
    handler = Mock()

    @app.post("/operation")
    @COUNT_REQUESTS(LABELS, logger, "v1beta1")
    async def operation(value: int = Body(embed=True)):
        handler(value)
        return {"value": value}

    async def request(method: str, path: str, body: dict):
        request_messages = [
            {
                "type": "http.request",
                "body": json.dumps(body).encode(),
                "more_body": False,
            }
        ]
        response_messages = []

        async def receive():
            if request_messages:
                return request_messages.pop(0)
            return {"type": "http.disconnect"}

        async def send(message):
            response_messages.append(message)

        raw_path = path.encode()
        await app(
            {
                "type": "http",
                "asgi": {"version": "3.0", "spec_version": "2.3"},
                "http_version": "1.1",
                "method": method,
                "scheme": "http",
                "path": path,
                "raw_path": raw_path,
                "query_string": b"",
                "root_path": "",
                "headers": [
                    (b"host", b"testserver"),
                    (b"content-type", b"application/json"),
                    (b"content-length", str(len(json.dumps(body).encode())).encode()),
                ],
                "client": ("127.0.0.1", 123),
                "server": ("testserver", 80),
            },
            receive,
            send,
        )
        response_start = next(
            message for message in response_messages if message["type"] == "http.response.start"
        )
        response_body = b"".join(
            message.get("body", b"")
            for message in response_messages
            if message["type"] == "http.response.body"
        )
        return response_start["status"], json.loads(response_body)

    with (
        patch("etos_api.library.metrics.REQUESTS_TOTAL", request_counter),
        patch("etos_api.library.metrics.API_REQUESTS_TOTAL", api_counter),
        patch("etos_api.library.metrics.API_REQUEST_DURATION", api_duration),
    ):
        status, body = await request("POST", "/operation", {"value": 7})
        invalid_status, _ = await request("POST", "/operation", {"value": "not an integer"})
        missing_status, _ = await request("POST", "/unmatched", {"value": 7})

    assert status == 200
    assert body == {"value": 7}
    assert invalid_status == 422
    assert missing_status == 404
    handler.assert_called_once_with(7)
    request_counter.labels.assert_called_once_with(**CLUSTER_LABELS, status=200)
    request_counter.labels.return_value.inc.assert_called_once_with()
    api_counter.labels.assert_called_once_with(
        etos_cluster="unassigned",
        api_version="v1beta1",
        operation="start_testrun",
        status_class="2xx",
        outcome="success",
    )
    api_counter.labels.return_value.inc.assert_called_once_with()
    api_duration.labels.assert_called_once_with(
        api_version="v1beta1",
        operation="start_testrun",
        outcome="success",
        etos_cluster="unassigned",
    )
    api_duration.labels.return_value.observe.assert_called_once()


def test_mounted_api_route_handlers_are_coroutine_functions():
    """Verify metric decorators keep every mounted API handler awaitable by FastAPI.

    A synchronous wrapper, such as Prometheus' ``Histogram.time()`` decorator, makes FastAPI run
    the handler in a threadpool and return the un-awaited coroutine as the response.
    """
    endpoints = [
        route.endpoint
        for mount in APP.routes
        for route in getattr(getattr(mount, "app", None), "routes", [])
        if isinstance(route, APIRoute)
    ]

    assert endpoints
    for endpoint in endpoints:
        assert inspect.iscoroutinefunction(endpoint), endpoint.__qualname__
