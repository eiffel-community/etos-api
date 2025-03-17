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
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.sdk.resources import (
    SERVICE_NAME,
    SERVICE_VERSION,
    OTELResourceDetector,
    ProcessResourceDetector,
    Resource,
    get_aggregated_resources,
)
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

from etos_api.library.context_logging import ContextLogging

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
    OTEL_RESOURCE = Resource.create(
        {
            SERVICE_NAME: "etos-api",
            SERVICE_VERSION: VERSION,
        },
    )

    OTEL_RESOURCE = get_aggregated_resources(
        [OTELResourceDetector(), ProcessResourceDetector()],
    ).merge(OTEL_RESOURCE)

    PROVIDER = TracerProvider(resource=OTEL_RESOURCE)
    EXPORTER = OTLPSpanExporter()
    PROCESSOR = BatchSpanProcessor(EXPORTER)
    PROVIDER.add_span_processor(PROCESSOR)
    trace.set_tracer_provider(PROVIDER)
    setup_logging("ETOS API", VERSION, otel_resource=OTEL_RESOURCE)

    FastAPIInstrumentor().instrument_app(APP, tracer_provider=PROVIDER, excluded_urls=".*/ping")
else:
    setup_logging("ETOS API", VERSION)

RegisterProviders()
