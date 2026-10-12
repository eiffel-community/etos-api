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
"""ETOS API metrics."""

from enum import Enum
from functools import wraps
from logging import Logger
from time import perf_counter
from typing import Callable

from fastapi import HTTPException
from prometheus_client import Counter, Histogram

from etos_api.library.cluster import cluster_identity

OPERATIONS = Enum(
    "OPERATIONS",
    [
        "start_testrun",
        "get_subsuite",
        "stop_testrun",
    ],
)

REQUEST_TIME = Histogram(
    "http_request_duration_seconds",
    "Time spent processing request",
    ["endpoint", "operation", "etos_cluster"],
)
REQUESTS_TOTAL = Counter(
    "http_requests_total",
    "Total number of requests",
    ["endpoint", "operation", "status", "etos_cluster"],
)

API_REQUESTS_TOTAL = Counter(
    "etos_api_requests_total",
    "Total decorated ETOS API operations by version, operation, status class, and outcome; "
    "excludes HTTP requests that do not reach a decorated handler",
    ["api_version", "operation", "status_class", "outcome", "etos_cluster"],
)
API_REQUEST_DURATION = Histogram(
    "etos_api_request_duration_seconds",
    "Duration in seconds of decorated ETOS API operations; excludes HTTP requests that do not "
    "reach a decorated handler",
    ["api_version", "operation", "outcome", "etos_cluster"],
)
SUITE_OPERATIONS_TOTAL = Counter(
    "etos_api_suite_operations_total",
    "Total suite download and validation operations",
    ["api_version", "operation", "outcome", "etos_cluster"],
)
SUITE_OPERATION_DURATION = Histogram(
    "etos_api_suite_operation_duration_seconds",
    "Suite download and validation duration in seconds",
    ["api_version", "operation", "outcome", "etos_cluster"],
)

SUITE_OPERATION_NAMES = ("suite_download", "suite_validation")
SUITE_OPERATION_OUTCOMES = ("success", "client_error", "server_error")


def outcome_for_status(status_code: int) -> str:
    """Map an HTTP status to the bounded dashboard outcome vocabulary."""
    if 200 <= status_code < 300:
        return "success"
    if status_code == 404:
        return "not_found"
    if status_code == 422:
        return "validation_rejected"
    if status_code == 504:
        return "timed_out"
    if 400 <= status_code < 500:
        return "client_error"
    return "server_error"


def record_suite_operation(api_version: str, operation: str, outcome: str, duration: float) -> None:
    """Record one bounded suite admission operation observation."""
    if api_version not in ("v1alpha", "v1beta1"):
        raise ValueError(f"Unsupported API version: {api_version}")
    if operation not in SUITE_OPERATION_NAMES:
        raise ValueError(f"Unsupported suite operation: {operation}")
    if outcome not in SUITE_OPERATION_OUTCOMES:
        raise ValueError(f"Unsupported suite operation outcome: {outcome}")

    labels = {
        "etos_cluster": cluster_identity(),
        "api_version": api_version,
        "operation": operation,
        "outcome": outcome,
    }
    SUITE_OPERATIONS_TOTAL.labels(**labels).inc()
    SUITE_OPERATION_DURATION.labels(**labels).observe(duration)


# I like the idea of all operations in this file is upper-case.
def COUNT_REQUESTS(labels: dict, logger: Logger, api_version: str):  # pylint:disable=invalid-name
    """Count requests that reach a decorated operation handler.

    These operation metrics do not include FastAPI request-validation rejects or unmatched
    routes because those requests never reach the decorated handler. FastAPI auto-instrumentation
    handles generic HTTP instrumentation separately; this decorator emits operation metrics only.

    The decorator also observes REQUEST_TIME. Prometheus' ``Histogram.time()`` decorator must not
    be used on the async route handlers because it returns the un-awaited coroutine to FastAPI.
    """

    def decorator(func: Callable):
        @wraps(func)
        async def wrapper(*args, **kwargs):
            started = perf_counter()
            status_code = 200

            def record(status_code: int):
                duration = perf_counter() - started
                cluster = cluster_identity()
                REQUEST_TIME.labels(**labels, etos_cluster=cluster).observe(duration)
                REQUESTS_TOTAL.labels(**labels, status=status_code, etos_cluster=cluster).inc()
                API_REQUESTS_TOTAL.labels(
                    etos_cluster=cluster,
                    api_version=api_version,
                    operation=labels["operation"],
                    status_class=f"{status_code // 100}xx",
                    outcome=outcome_for_status(status_code),
                ).inc()
                API_REQUEST_DURATION.labels(
                    etos_cluster=cluster,
                    api_version=api_version,
                    operation=labels["operation"],
                    outcome=outcome_for_status(status_code),
                ).observe(duration)

            try:
                return await func(*args, **kwargs)
            except HTTPException as http_exception:
                status_code = http_exception.status_code
                raise
            except Exception:  # pylint:disable=bare-except
                status_code = 500
                logger.exception("Unhandled exception occurred, setting status to 500")
                raise
            finally:
                record(status_code)

        return wrapper

    return decorator
