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
from typing import Callable

from fastapi import HTTPException
from prometheus_client import Counter, Histogram

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
    ["endpoint", "operation"],
)
REQUESTS_TOTAL = Counter(
    "http_requests_total",
    "Total number of requests",
    ["endpoint", "operation", "status"],
)


# I like the idea of all operations in this file is upper-case.
def COUNT_REQUESTS(labels: dict, logger: Logger):  # pylint:disable=invalid-name
    """Count number of requests to server using the REQUESTS_TOTAL counter."""

    def decorator(func: Callable):
        @wraps(func)
        async def wrapper(*args, **kwargs):
            try:
                response = await func(*args, **kwargs)
                REQUESTS_TOTAL.labels(**labels, status=200).inc()
                return response
            except HTTPException as http_exception:
                REQUESTS_TOTAL.labels(**labels, status=http_exception.status_code).inc()
                raise
            except Exception:  # pylint:disable=bare-except
                logger.exception("Unhandled exception occurred, setting status to 500")
                REQUESTS_TOTAL.labels(**labels, status=500).inc()
                raise

        return wrapper

    return decorator
