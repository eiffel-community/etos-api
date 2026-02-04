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
