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
"""Tests for ETOS API OpenTelemetry setup."""

from fastapi import FastAPI
from fastapi.testclient import TestClient
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.instrumentation.system_metrics import SystemMetricsInstrumentor
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import InMemoryMetricReader
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from prometheus_client import make_asgi_app

from etos_api.library.opentelemetry import OTEL_EXCLUDED_URLS, RUNTIME_METRICS


def test_infrastructure_endpoints_are_not_traced():
    """Verify Prometheus scrapes and pings create no spans while API routes still do."""
    app = FastAPI()
    app.mount("/metrics", make_asgi_app(), "Metrics")

    @app.get("/api/ping")
    async def ping():
        """Return a probe response."""
        return {}

    @app.get("/api/selftest")
    async def selftest():
        """Return a normal API response."""
        return {}

    exporter = InMemorySpanExporter()
    provider = TracerProvider()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    FastAPIInstrumentor().instrument_app(
        app, tracer_provider=provider, excluded_urls=OTEL_EXCLUDED_URLS
    )
    try:
        client = TestClient(app)
        assert client.get("/metrics/").status_code == 200
        assert client.get("/api/ping").status_code == 200
        assert not exporter.get_finished_spans()

        assert client.get("/api/selftest").status_code == 200
        assert exporter.get_finished_spans()
    finally:
        FastAPIInstrumentor().uninstrument_app(app)


def test_runtime_metrics_exclude_host_wide_metrics():
    """Verify only process runtime metrics are produced."""
    reader = InMemoryMetricReader()
    provider = MeterProvider(metric_readers=[reader])
    instrumentor = SystemMetricsInstrumentor(config=RUNTIME_METRICS)
    instrumentor.instrument(meter_provider=provider)
    try:
        data = reader.get_metrics_data()
        names = {
            metric.name
            for rm in data.resource_metrics
            for sm in rm.scope_metrics
            for metric in sm.metrics
        }
        assert names
        assert all(name.startswith("process.runtime.") for name in names)
        assert any(name.endswith(".memory") for name in names)
    finally:
        instrumentor.uninstrument()
