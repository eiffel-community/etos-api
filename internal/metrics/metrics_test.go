// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/julienschmidt/httprouter"
	"github.com/stretchr/testify/require"
)

// TestHTTPMetricsUsesRouteTemplates verifies request identifiers stay out of metric labels.
func TestHTTPMetricsUsesRouteTemplates(t *testing.T) {
	m := NewHTTPMetrics("test")
	router := httprouter.New()
	router.GET("/items/:id", m.Wrap("/items/:id", http.MethodGet, func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/items/secret-id", nil))
	require.Equal(t, http.StatusNoContent, recorder.Code)

	output := httptest.NewRecorder()
	m.Render(output)
	require.Contains(t, output.Body.String(), `route="/items/:id"`)
	require.NotContains(t, output.Body.String(), "secret-id")
}

// TestSSEMetricsBoundsEventClassAndTracksConnections verifies bounded labels and connection counts.
func TestSSEMetricsBoundsEventClassAndTracksConnections(t *testing.T) {
	t.Setenv("ETOS_NAMESPACE", "")
	t.Setenv("ETOS_CLUSTER", "")
	m := NewSSEMetrics()
	m.ConnectionOpened()
	m.EventDelivered("unexpected-user-event")
	m.EventDelivered("ping")
	m.ReconnectRequested()
	m.StreamError("sensitive-backend-error")
	m.ConnectionClosedWithReason("client_disconnect")

	recorder := httptest.NewRecorder()
	m.Render(recorder)
	output := recorder.Body.String()
	require.Contains(t, output, `etos_sse_connections_active{etos_cluster="unassigned"} 0`)
	require.Contains(t, output, `etos_sse_connections_total{etos_cluster="unassigned"} 1`)
	require.Contains(t, output, `event_class="data"`)
	require.Contains(t, output, `event_class="ping"`)
	require.Contains(t, output, `etos_sse_disconnects_total{etos_cluster="unassigned",reason="client_disconnect"} 1`)
	require.Contains(t, output, `etos_sse_errors_total{category="unknown",etos_cluster="unassigned"} 1`)
	require.Contains(t, output, `etos_sse_reconnects_total{etos_cluster="unassigned"} 1`)
	require.NotContains(t, output, "unexpected-user-event")
	require.NotContains(t, output, "sensitive-backend-error")
}

// TestSSEConnectionsCloseOnceWithoutAffectingOtherClients verifies independent client accounting.
func TestSSEConnectionsCloseOnceWithoutAffectingOtherClients(t *testing.T) {
	t.Setenv("ETOS_CLUSTER", "")
	m := NewSSEMetrics()
	m.ConnectionOpened()
	m.ConnectionOpened()
	m.ConnectionClosedWithReason("client_disconnect")
	recorder := httptest.NewRecorder()
	m.Render(recorder)
	require.Contains(t, recorder.Body.String(), `etos_sse_connections_active{etos_cluster="unassigned"} 1`)
	require.Contains(t, recorder.Body.String(), `etos_sse_disconnects_total{etos_cluster="unassigned",reason="client_disconnect"} 1`)
	m.ConnectionClosedWithReason("server_shutdown")
	recorder = httptest.NewRecorder()
	m.Render(recorder)
	require.Contains(t, recorder.Body.String(), `etos_sse_connections_active{etos_cluster="unassigned"} 0`)
}

// TestHTTPHistogramAggregatesStatusClassesWithoutDuplicateSeries verifies shared latency labels aggregate.
func TestHTTPHistogramAggregatesStatusClassesWithoutDuplicateSeries(t *testing.T) {
	t.Setenv("ETOS_CLUSTER", "")
	m := NewHTTPMetrics("test")
	m.recordRequest("/items/:id", "GET", "4xx", "client_error", 0.01)
	m.recordRequest("/items/:id", "GET", "5xx", "client_error", 0.02)
	recorder := httptest.NewRecorder()
	m.Render(recorder)
	require.Contains(t, recorder.Body.String(), `test_service_request_duration_seconds_count{etos_cluster="unassigned",method="GET",outcome="client_error",route="/items/:id"} 2`)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "\ntest_service_request_duration_seconds_count{"))
}

// TestSSECollectorsUseExplicitCluster verifies all SSE collectors use the configured namespace-qualified cluster identity.
func TestSSECollectorsUseExplicitCluster(t *testing.T) {
	for _, cluster := range []string{"first", "second"} {
		t.Run(cluster, func(t *testing.T) {
			t.Setenv("ETOS_NAMESPACE", "namespace")
			t.Setenv("ETOS_CLUSTER", cluster)
			m := NewSSEMetrics()
			m.ConnectionOpened()
			m.EventDelivered("ping")
			m.ReconnectRequested()
			m.StreamError("backend_failure")
			m.ConnectionClosedWithReason("client_disconnect")
			m.recordRequest("/events", "GET", "2xx", "success", 0.01)
			families, err := m.registry.Gather()
			require.NoError(t, err)
			for _, family := range families {
				for _, metric := range family.Metric {
					found := false
					for _, label := range metric.Label {
						if label.GetName() == "etos_cluster" {
							require.Equal(t, "namespace/"+cluster, label.GetValue())
							found = true
						}
					}
					require.True(t, found, family.GetName())
				}
			}
		})
	}
}

// TestResponseRecorderPreservesFirstAndImplicitStatus verifies implicit success cannot be overwritten.
func TestResponseRecorderPreservesFirstAndImplicitStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := &responseRecorder{ResponseWriter: recorder, status: http.StatusOK}
	_, err := writer.Write([]byte("body"))
	require.NoError(t, err)
	writer.WriteHeader(http.StatusInternalServerError)
	require.Equal(t, http.StatusOK, writer.status)
	require.Equal(t, http.StatusOK, recorder.Code)
}
