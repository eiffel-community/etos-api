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
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// SSEMetrics observes the v2alpha service's connections and server-side writes.
type SSEMetrics struct {
	*HTTPMetrics
	connections metric.Int64UpDownCounter
	total       metric.Int64Counter
	events      metric.Int64Counter
	disconnects metric.Int64Counter
	errors      metric.Int64Counter
	reconnects  metric.Int64Counter
}

// NewSSEMetrics creates the v2alpha instruments on the global meter provider.
func NewSSEMetrics() *SSEMetrics {
	return NewSSEMetricsWithMeter(otel.Meter(meterName))
}

// NewSSEMetricsWithMeter creates the v2alpha instruments on a specific meter.
func NewSSEMetricsWithMeter(meter metric.Meter) *SSEMetrics {
	connections, err := meter.Int64UpDownCounter(
		"etos_sse.connections.active",
		metric.WithUnit("{connection}"),
		metric.WithDescription("Active v2alpha SSE connections."),
	)
	otel.Handle(err)
	total, err := meter.Int64Counter(
		"etos_sse.connections",
		metric.WithUnit("{connection}"),
		metric.WithDescription("Total v2alpha SSE connections opened."),
	)
	otel.Handle(err)
	events, err := meter.Int64Counter(
		"etos_sse.events.delivered",
		metric.WithUnit("{event}"),
		metric.WithDescription("SSE events written by bounded event class; not client receipt."),
	)
	otel.Handle(err)
	disconnects, err := meter.Int64Counter(
		"etos_sse.disconnects",
		metric.WithUnit("{connection}"),
		metric.WithDescription("SSE connections closed by bounded reason."),
	)
	otel.Handle(err)
	errors, err := meter.Int64Counter(
		"etos_sse.errors",
		metric.WithUnit("{error}"),
		metric.WithDescription("SSE stream errors by bounded category."),
	)
	otel.Handle(err)
	reconnects, err := meter.Int64Counter(
		"etos_sse.reconnects",
		metric.WithUnit("{request}"),
		metric.WithDescription("SSE requests carrying Last-Event-ID."),
	)
	otel.Handle(err)
	return &SSEMetrics{
		HTTPMetrics: NewHTTPMetricsWithMeter("etos_sse", meter),
		connections: connections,
		total:       total,
		events:      events,
		disconnects: disconnects,
		errors:      errors,
		reconnects:  reconnects,
	}
}

// ConnectionOpened increments current and cumulative connection counts.
func (m *SSEMetrics) ConnectionOpened() {
	ctx := context.Background()
	m.connections.Add(ctx, 1, metric.WithAttributes(m.cluster))
	m.total.Add(ctx, 1, metric.WithAttributes(m.cluster))
}

// ConnectionClosedWithReason closes one opened connection exactly once.
func (m *SSEMetrics) ConnectionClosedWithReason(reason string) {
	switch reason {
	case "client_disconnect", "server_shutdown", "subscriber_shutdown", "stream_write_error":
	default:
		reason = "unknown"
	}
	ctx := context.Background()
	m.connections.Add(ctx, -1, metric.WithAttributes(m.cluster))
	m.disconnects.Add(ctx, 1, metric.WithAttributes(m.cluster, attribute.String("reason", reason)))
}

// ReconnectRequested counts resume intent without asserting successful replay.
func (m *SSEMetrics) ReconnectRequested() {
	m.reconnects.Add(context.Background(), 1, metric.WithAttributes(m.cluster))
}

// StreamError records an error category without exposing backend error text.
func (m *SSEMetrics) StreamError(category string) {
	switch category {
	case "backend_failure", "protocol_unsupported", "stream_write_error":
	default:
		category = "unknown"
	}
	m.errors.Add(context.Background(), 1, metric.WithAttributes(m.cluster, attribute.String("category", category)))
}

// EventDelivered records a server-side event write using a bounded event class.
func (m *SSEMetrics) EventDelivered(event string) {
	class := "data"
	switch event {
	case "ping", "shutdown", "error":
		class = event
	}
	m.events.Add(context.Background(), 1, metric.WithAttributes(m.cluster, attribute.String("event_class", class)))
}
