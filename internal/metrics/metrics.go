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

// Package metrics instruments service operations with bounded OpenTelemetry attributes.
package metrics

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/julienschmidt/httprouter"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// meterName is the instrumentation scope of the ETOS API Go services.
const meterName = "github.com/eiffel-community/etos-api"

// durationBuckets are in seconds; the OpenTelemetry defaults are meant for milliseconds.
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// HTTPMetrics owns the HTTP instruments of one service.
type HTTPMetrics struct {
	meter    metric.Meter
	cluster  attribute.KeyValue
	requests metric.Int64Counter
	duration metric.Float64Histogram
}

// NewHTTPMetrics creates request count and latency instruments on the global meter provider.
func NewHTTPMetrics(namespace string) *HTTPMetrics {
	return NewHTTPMetricsWithMeter(namespace, otel.Meter(meterName))
}

// NewHTTPMetricsWithMeter creates request count and latency instruments on a specific meter.
func NewHTTPMetricsWithMeter(namespace string, meter metric.Meter) *HTTPMetrics {
	requests, err := meter.Int64Counter(
		namespace+".service.requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("HTTP requests handled by the service."),
	)
	otel.Handle(err)
	duration, err := meter.Float64Histogram(
		namespace+".service.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("HTTP request duration in seconds."),
		metric.WithExplicitBucketBoundaries(durationBuckets...),
	)
	otel.Handle(err)
	return &HTTPMetrics{meter: meter, cluster: clusterAttribute(), requests: requests, duration: duration}
}

// Wrap records a completed request using the route template, not the request URL.
func (m *HTTPMetrics) Wrap(route, method string, handler httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		start := time.Now()
		recorder := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		handler(recorder, r, ps)
		statusClass := strconv.Itoa(recorder.status/100) + "xx"
		outcome := "success"
		if recorder.status >= 500 {
			outcome = "server_error"
		} else if recorder.status >= 400 {
			outcome = "client_error"
		}
		m.recordRequest(r.Context(), route, method, statusClass, outcome, time.Since(start).Seconds())
	}
}

// recordRequest observes a request once in the counter and latency histogram.
func (m *HTTPMetrics) recordRequest(ctx context.Context, route, method, statusClass, outcome string, seconds float64) {
	m.requests.Add(ctx, 1, metric.WithAttributes(
		m.cluster,
		attribute.String("method", method),
		attribute.String("route", route),
		attribute.String("status_class", statusClass),
		attribute.String("outcome", outcome),
	))
	m.duration.Record(ctx, seconds, metric.WithAttributes(
		m.cluster,
		attribute.String("method", method),
		attribute.String("route", route),
		attribute.String("outcome", outcome),
	))
}

// responseRecorder tracks the first response status while preserving streaming.
type responseRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// Unwrap exposes the underlying writer to net/http response controllers.
func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// WriteHeader records the first status sent to the client.
func (r *responseRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

// Write records the implicit success status before forwarding response data.
func (r *responseRecorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(p)
}

// Flush forwards streaming flushes and records the implicit response status.
func (r *responseRecorder) Flush() {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
