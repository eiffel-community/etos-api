# Copyright 2020-2021 Axis Communications AB.
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
"""ETOS API module."""

import os
from importlib.metadata import PackageNotFoundError, version

from etos_lib.logging.logger import setup_logging

from etos_api.library.context_logging import ContextLogging
from etos_api.library.opentelemetry import setup_opentelemetry

from .library.providers.register import RegisterProviders
from .main import APP

# The API shall not send logs to RabbitMQ as it
# is too early in the ETOS test run.
os.environ["ETOS_ENABLE_SENDING_LOGS"] = "false"

try:
    VERSION = version("etos_api")
except PackageNotFoundError:
    VERSION = "Unknown"

DEV = os.getenv("DEV", "false").lower() == "true"

if os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT"):
    OTEL_RESOURCE = setup_opentelemetry(APP, VERSION)
    setup_logging("ETOS API", VERSION, otel_resource=OTEL_RESOURCE)
else:
    setup_logging("ETOS API", VERSION)

RegisterProviders()
