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
"""ETOS opentelemetry helpers."""

from typing import Annotated

from fastapi import Header
from opentelemetry import context as otel_context
from opentelemetry import trace
from opentelemetry.baggage.propagation import W3CBaggagePropagator
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.propagate import extract, set_global_textmap
from opentelemetry.propagators.composite import CompositePropagator
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
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator
from pydantic import BaseModel


def setup_opentelemetry(app, version: str):
    """Set up OpenTelemetry for ETOS API."""
    otel_resource = Resource.create(
        {
            SERVICE_NAME: "etos-api",
            SERVICE_VERSION: version,
        },
    )

    otel_resource = get_aggregated_resources(
        [OTELResourceDetector(), ProcessResourceDetector()],
    ).merge(otel_resource)

    provider = TracerProvider(resource=otel_resource)
    exporter = OTLPSpanExporter()
    processor = BatchSpanProcessor(exporter)
    provider.add_span_processor(processor)
    trace.set_tracer_provider(provider)
    propagator = CompositePropagator(
        [
            TraceContextTextMapPropagator(),
            W3CBaggagePropagator(),
        ]
    )
    set_global_textmap(propagator)
    FastAPIInstrumentor().instrument_app(app, tracer_provider=provider, excluded_urls=".*/ping")
    return otel_resource


class OTELHeaders(BaseModel):
    """Headers model."""

    baggage: str | None = None
    traceparent: str | None = None


def context(headers: Annotated[OTELHeaders, Header()]) -> otel_context.Context:
    """Extract OpenTelemetry context from headers."""
    return extract(headers.model_dump(), context=otel_context.get_current())
