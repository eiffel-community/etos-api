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
from etos_api.library import cluster, metrics


@pytest.mark.parametrize("namespace", ["first", "second"])
def test_explicit_cluster_routes_api_and_suite_metrics(monkeypatch, namespace):
    """The same Cluster CR name in two namespaces must never merge series."""
    monkeypatch.setattr(cluster.Path, "exists", lambda _: False)
    monkeypatch.setenv("ETOS_NAMESPACE", namespace)
    monkeypatch.setenv("ETOS_CLUSTER", "shared")
    assert cluster.cluster_identity() == f"{namespace}/shared"
    counter, histogram = Mock(), Mock()
    monkeypatch.setattr(metrics, "SUITE_OPERATIONS_TOTAL", counter)
    monkeypatch.setattr(metrics, "SUITE_OPERATION_DURATION", histogram)
    metrics.record_suite_operation("v1beta1", "suite_download", "success", 0.5)
    labels = {
        "api_version": "v1beta1",
        "operation": "suite_download",
        "outcome": "success",
        "etos_cluster": f"{namespace}/shared",
    }
    counter.labels.assert_called_once_with(**labels)
    histogram.labels.assert_called_once_with(**labels)


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
async def test_all_api_domain_collectors_keep_two_clusters_separate(monkeypatch):
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

    for collector in (
        metrics.REQUEST_TIME,
        metrics.REQUESTS_TOTAL,
        metrics.API_REQUESTS_TOTAL,
        metrics.API_REQUEST_DURATION,
        metrics.SUITE_OPERATIONS_TOTAL,
        metrics.SUITE_OPERATION_DURATION,
    ):
        identities = {
            sample.labels["etos_cluster"]
            for family in collector.collect()
            for sample in family.samples
            if sample.labels.get("etos_cluster", "").endswith("/routing-test")
        }
        assert identities == {"first/routing-test", "second/routing-test"}
