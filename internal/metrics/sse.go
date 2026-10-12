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

import "github.com/prometheus/client_golang/prometheus"

// SSEMetrics observes the v2alpha service's connections and server-side writes.
type SSEMetrics struct {
	*HTTPMetrics
	connections prometheus.Gauge
	total       prometheus.Counter
	events      *prometheus.CounterVec
	disconnects *prometheus.CounterVec
	errors      *prometheus.CounterVec
	reconnects  prometheus.Counter
}

// NewSSEMetrics registers v2alpha collectors in an isolated service registry.
func NewSSEMetrics() *SSEMetrics {
	m := &SSEMetrics{
		HTTPMetrics: NewHTTPMetrics("etos_sse"),
		connections: prometheus.NewGauge(prometheus.GaugeOpts{
			ConstLabels: clusterLabels(),
			Name:        "etos_sse_connections_active", Help: "Active v2alpha SSE connections.",
		}),
		total: prometheus.NewCounter(prometheus.CounterOpts{
			ConstLabels: clusterLabels(),
			Name:        "etos_sse_connections_total", Help: "Total v2alpha SSE connections opened.",
		}),
		events: prometheus.NewCounterVec(prometheus.CounterOpts{
			ConstLabels: clusterLabels(),
			Name:        "etos_sse_events_delivered_total", Help: "SSE events written by bounded event class; not client receipt.",
		}, []string{"event_class"}),
		disconnects: prometheus.NewCounterVec(prometheus.CounterOpts{
			ConstLabels: clusterLabels(),
			Name:        "etos_sse_disconnects_total", Help: "SSE connections closed by bounded reason.",
		}, []string{"reason"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			ConstLabels: clusterLabels(),
			Name:        "etos_sse_errors_total", Help: "SSE stream errors by bounded category.",
		}, []string{"category"}),
		reconnects: prometheus.NewCounter(prometheus.CounterOpts{
			ConstLabels: clusterLabels(),
			Name:        "etos_sse_reconnects_total", Help: "SSE requests carrying Last-Event-ID.",
		}),
	}
	m.registry.MustRegister(m.connections, m.total, m.events, m.disconnects, m.errors, m.reconnects)
	return m
}

// ConnectionOpened increments current and cumulative connection counts.
func (m *SSEMetrics) ConnectionOpened() {
	m.connections.Inc()
	m.total.Inc()
}

// ConnectionClosedWithReason closes one opened connection exactly once.
func (m *SSEMetrics) ConnectionClosedWithReason(reason string) {
	switch reason {
	case "client_disconnect", "server_shutdown", "subscriber_shutdown", "stream_write_error":
	default:
		reason = "unknown"
	}
	m.connections.Dec()
	m.disconnects.WithLabelValues(reason).Inc()
}

// ReconnectRequested counts resume intent without asserting successful replay.
func (m *SSEMetrics) ReconnectRequested() {
	m.reconnects.Inc()
}

// StreamError records an error category without exposing backend error text.
func (m *SSEMetrics) StreamError(category string) {
	switch category {
	case "backend_failure", "protocol_unsupported", "stream_write_error":
	default:
		category = "unknown"
	}
	m.errors.WithLabelValues(category).Inc()
}

// EventDelivered records a server-side event write using a bounded event class.
func (m *SSEMetrics) EventDelivered(event string) {
	class := "data"
	switch event {
	case "ping", "shutdown", "error":
		class = event
	}
	m.events.WithLabelValues(class).Inc()
}
