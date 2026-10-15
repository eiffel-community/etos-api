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
"""Namespace-qualified API ownership and domain metric routing."""

from unittest.mock import Mock

import pytest
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import InMemoryMetricReader

from etos_api.library import cluster, metrics


@pytest.fixture(name="reader")
def fixture_reader(monkeypatch):
    """Publish the ETOS API instruments to an in-memory OpenTelemetry reader."""
    reader = InMemoryMetricReader()
    provider = MeterProvider(metric_readers=[reader])
    monkeypatch.setattr(
        metrics, "INSTRUMENTS", metrics.create_instruments(provider.get_meter("test"))
    )
    yield reader
    provider.shutdown()


def cluster_identities(reader: InMemoryMetricReader) -> dict:
    """Get the etos_cluster attribute values published per metric."""
    identities = {}
    for resource_metrics in reader.get_metrics_data().resource_metrics:
        for scope_metrics in resource_metrics.scope_metrics:
            for metric in scope_metrics.metrics:
                identities[metric.name] = {
                    point.attributes["etos_cluster"] for point in metric.data.data_points
                }
    return identities


@pytest.mark.parametrize("namespace", ["first", "second"])
def test_explicit_cluster_routes_api_and_suite_metrics(monkeypatch, reader, namespace):
    """The same Cluster CR name in two namespaces must never merge series."""
    monkeypatch.setattr(cluster.Path, "exists", lambda _: False)
    monkeypatch.setenv("ETOS_NAMESPACE", namespace)
    monkeypatch.setenv("ETOS_CLUSTER", "shared")
    assert cluster.cluster_identity() == f"{namespace}/shared"
    metrics.record_suite_operation("v1beta1", "suite_download", "success", 0.5)
    assert cluster_identities(reader) == {
        "etos_api.suite.operations": {f"{namespace}/shared"},
        "etos_api.suite.operation.duration": {f"{namespace}/shared"},
    }


def test_missing_or_already_qualified_config_is_unassigned(monkeypatch):
    """An absent namespace or malformed bare-name config never invents ownership."""
    monkeypatch.setattr(cluster.Path, "exists", lambda _: False)
    monkeypatch.delenv("ETOS_NAMESPACE", raising=False)
    monkeypatch.setenv("ETOS_CLUSTER", "shared")
    assert cluster.cluster_identity() == "unassigned"
    monkeypatch.setenv("ETOS_NAMESPACE", "first")
    monkeypatch.setenv("ETOS_CLUSTER", "second/shared")
    assert cluster.cluster_identity() == "unassigned"


def test_namespace_matches_kubernetes_service_account_precedence(monkeypatch):
    """Use the same namespace resolution precedence as the Kubernetes client."""
    monkeypatch.setattr(cluster.Path, "exists", lambda _: True)
    monkeypatch.setattr(cluster.Path, "read_text", lambda _, **__: "actual\n")
    monkeypatch.setenv("ETOS_NAMESPACE", "ignored")
    monkeypatch.setenv("ETOS_CLUSTER", "shared")
    assert cluster.cluster_identity() == "actual/shared"


@pytest.mark.asyncio
async def test_all_api_domain_collectors_keep_two_clusters_separate(monkeypatch, reader):
    """Exercise real counters/histograms, including legacy decorated operations."""
    monkeypatch.setattr(cluster.Path, "exists", lambda _: False)
    monkeypatch.setenv("ETOS_CLUSTER", "routing-test")

    @metrics.COUNT_REQUESTS(
        {"endpoint": "/routing-test", "operation": "start_testrun"}, Mock(), "v1beta1"
    )
    async def operation():
        return {}

    for namespace in ("first", "second"):
        monkeypatch.setenv("ETOS_NAMESPACE", namespace)
        await operation()
        metrics.record_suite_operation("v1beta1", "suite_download", "success", 0.5)

    identities = cluster_identities(reader)
    assert set(identities) == {
        "etos_api.http.request.duration",
        "etos_api.http.requests",
        "etos_api.requests",
        "etos_api.request.duration",
        "etos_api.suite.operations",
        "etos_api.suite.operation.duration",
    }
    for published in identities.values():
        assert published == {"first/routing-test", "second/routing-test"}
