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

// Package metrics instruments service operations with bounded Prometheus labels.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var histogramBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// HTTPMetrics owns HTTP collectors and a registry isolated to one service.
type HTTPMetrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewHTTPMetrics registers request counts and latency histograms for a service.
func NewHTTPMetrics(namespace string) *HTTPMetrics {
	m := &HTTPMetrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "service_requests_total",
			ConstLabels: clusterLabels(),
			Help:        "HTTP requests handled by the service.",
		}, []string{"method", "route", "status_class", "outcome"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Name: "service_request_duration_seconds",
			ConstLabels: clusterLabels(),
			Help:        "HTTP request duration in seconds.", Buckets: histogramBuckets,
		}, []string{"method", "route", "outcome"}),
	}
	m.registry.MustRegister(m.requests, m.duration)
	return m
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
		m.recordRequest(route, method, statusClass, outcome, time.Since(start).Seconds())
	}
}

// recordRequest observes a request once in the counter and latency histogram.
func (m *HTTPMetrics) recordRequest(route, method, statusClass, outcome string, seconds float64) {
	m.requests.WithLabelValues(method, route, statusClass, outcome).Inc()
	m.duration.WithLabelValues(method, route, outcome).Observe(seconds)
}

// Handler serves this service's registry using standard Prometheus exposition.
func (m *HTTPMetrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Render exposes metrics in text format for direct callers.
func (m *HTTPMetrics) Render(w http.ResponseWriter) {
	m.Handler().ServeHTTP(w, &http.Request{Method: http.MethodGet, Header: make(http.Header)})
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

// Endpoint registers the metrics handler with the application's router.
type Endpoint struct {
	handler http.Handler
}

// NewEndpoint creates an application exposing a supplied metrics handler.
func NewEndpoint(handler http.Handler) *Endpoint {
	return &Endpoint{handler: handler}
}

// LoadRoutes registers the scrape endpoint without instrumenting its requests.
func (e *Endpoint) LoadRoutes(router *httprouter.Router) {
	router.GET("/metrics/", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		e.handler.ServeHTTP(w, r)
	})
}

// Close satisfies the application interface; this endpoint owns no resources.
func (e *Endpoint) Close() {}
