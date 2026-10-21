// Copyright Axis Communications AB.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package sse

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eiffel-community/etos-api/internal/metrics"
	"github.com/eiffel-community/etos-api/internal/stream"
	"github.com/eiffel-community/etos-api/internal/tracing"
	"github.com/eiffel-community/etos-api/pkg/application"
	v1 "github.com/eiffel-community/etos-api/pkg/sse/v1"
	v1alpha "github.com/eiffel-community/etos-api/pkg/sse/v1alpha"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

const runTraceparent = "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"
const requestTraceparent = "00-abcdef0123456789abcdef0123456789-abcd0123456789ef-01"

type resolverFunc func(context.Context, string) (tracing.TestRunContext, error)

// Resolve delegates lookup to the test's configured resolver function.
func (f resolverFunc) Resolve(ctx context.Context, id string) (tracing.TestRunContext, error) {
	return f(ctx, id)
}

// spanContext extracts a W3C traceparent fixture into a remote span context.
func spanContext(parent string) trace.SpanContext {
	ctx := propagation.TraceContext{}.Extract(context.Background(), propagation.MapCarrier{"traceparent": parent})
	return trace.SpanContextFromContext(ctx)
}

// testTracer creates a synchronous in-memory exporter and registers provider cleanup.
func testTracer(t *testing.T) (trace.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	return provider.Tracer("test"), exporter
}

// spanAttributes indexes exported attributes by key for test assertions.
func spanAttributes(span tracetest.SpanStub) map[attribute.Key]attribute.Value {
	attrs := make(map[attribute.Key]attribute.Value)
	for _, attr := range span.Attributes {
		attrs[attr.Key] = attr.Value
	}
	return attrs
}

// contextStreamer captures the context passed into the broker setup.
type contextStreamer struct {
	stream.Streamer
	stream stream.Stream
	ctx    context.Context
}

type closedBrokerStream struct{ fakeStream }

// WithChannel ignores message delivery for the disconnected-broker fixture.
func (s *closedBrokerStream) WithChannel(chan<- stream.Message) stream.Stream { return s }

// WithOffset leaves the disconnected-broker fixture unchanged.
func (s *closedBrokerStream) WithOffset(int64) stream.Stream { return s }

// WithFilter leaves the disconnected-broker fixture unchanged.
func (s *closedBrokerStream) WithFilter([]string) stream.Stream { return s }

// Consume immediately reports a broker disconnect through the closure channel.
func (s *closedBrokerStream) Consume(context.Context) (<-chan error, error) {
	closed := make(chan error, 1)
	closed <- errors.New("broker disconnected")
	return closed, nil
}

// NewStream captures the setup context and returns the configured test stream.
func (s *contextStreamer) NewStream(ctx context.Context, _ *logrus.Entry, _ string) (stream.Stream, error) {
	s.ctx = ctx
	return s.stream, nil
}

type cancelOnFlush struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

// Flush records delivery before triggering the configured disconnect or shutdown.
func (w cancelOnFlush) Flush() {
	w.ResponseRecorder.Flush()
	w.cancel()
}

// deliveryStream delivers one protocol-valid event, then waits for cancellation.
type deliveryStream struct {
	controlledStream
	ctx context.Context
}

// WithChannel records the destination for the fixture's protocol event.
func (s *deliveryStream) WithChannel(ch chan<- stream.Message) stream.Stream {
	s.channel = ch
	return s
}

// WithOffset leaves the single-event fixture unchanged.
func (s *deliveryStream) WithOffset(int64) stream.Stream { return s }

// WithFilter leaves the single-event fixture unchanged.
func (s *deliveryStream) WithFilter([]string) stream.Stream { return s }

// Consume captures the subscription context and sends one event unless canceled.
func (s *deliveryStream) Consume(ctx context.Context) (<-chan error, error) {
	s.ctx = ctx
	go func() {
		select {
		case s.channel <- stream.Message{Data: []byte(shutdownA)}:
		case <-ctx.Done():
		}
	}()
	return make(chan error), nil
}

// TestConnectionTraceLinksAndParents verifies that each connection links to its
// TestRun without reparenting and propagates its own span context to the stream.
func TestConnectionTraceLinksAndParents(t *testing.T) {
	for _, parent := range []string{"", requestTraceparent} {
		t.Run(parent, func(t *testing.T) {
			tracer, exporter := testTracer(t)
			calls := 0
			resolver := resolverFunc(func(ctx context.Context, id string) (tracing.TestRunContext, error) {
				calls++
				assert.Equal(t, "run-id", id)
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				assert.LessOrEqual(t, time.Until(deadline), time.Second)
				return tracing.TestRunContext{
					SpanContext: spanContext(runTraceparent), Outcome: "linked",
					Name: "generated-run", Namespace: "runs",
				}, nil
			})
			for _, lastID := range []string{"", "1"} {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				s := &deliveryStream{controlledStream: controlledStream{closed: make(chan struct{})}}
				backend := &contextStreamer{stream: s}
				h := Handler{
					logger: logrus.NewEntry(logrus.New()), ctx: context.Background(),
					streamer: backend, metrics: metrics.NewSSEMetrics(), tracer: tracer, resolver: resolver,
				}
				r := httptest.NewRequest(http.MethodGet, "/sse/v2alpha/events/run-id", nil).WithContext(ctx)
				r.Header.Set("traceparent", parent)
				r.Header.Set("Last-Event-ID", lastID)
				w := cancelOnFlush{httptest.NewRecorder(), cancel}
				h.GetEvents(w, r, httprouter.Params{{Key: "identifier", Value: "run-id"}})
				cancel()
				require.Contains(t, w.Body.String(), "event: shutdown")
				ended := exporter.GetSpans()
				require.NotEmpty(t, ended)
				span := ended[len(ended)-1]
				require.Len(t, span.Links, 1)
				assert.Equal(t, spanContext(runTraceparent), span.Links[0].SpanContext)
				assert.NotEqual(t, spanContext(runTraceparent).TraceID(), span.SpanContext.TraceID())
				assert.Equal(t, trace.SpanKindServer, span.SpanKind)
				if parent == "" {
					assert.False(t, span.Parent.IsValid())
				} else {
					assert.Equal(t, spanContext(parent), span.Parent)
					assert.Equal(t, spanContext(parent).TraceID(), span.SpanContext.TraceID())
				}
				assert.Equal(t, span.SpanContext, trace.SpanContextFromContext(backend.ctx))
				assert.Equal(t, span.SpanContext, trace.SpanContextFromContext(s.ctx))
				assert.Equal(t, codes.Unset, span.Status.Code)
				attrs := spanAttributes(span)
				assert.Equal(t, "run-id", attrs["etos.id"].AsString())
				assert.Equal(t, "linked", attrs["etos.sse.trace_link.outcome"].AsString())
				assert.Equal(t, "generated-run", attrs["etos.testrun.name"].AsString())
				assert.Equal(t, "runs", attrs["k8s.namespace.name"].AsString())
				assert.Equal(t, lastID != "", attrs["etos.sse.reconnect"].AsBool())
				assert.Equal(t, "client_disconnect", attrs["etos.sse.close_reason"].AsString())
				assert.EqualValues(t, 200, attrs["http.response.status_code"].AsInt64())
			}
			assert.Equal(t, 2, calls)
			spans := exporter.GetSpans()
			require.Len(t, spans, 2)
			assert.NotEqual(t, spans[0].SpanContext.SpanID(), spans[1].SpanContext.SpanID())
		})
	}
}

// TestCorrelationFailuresDoNotBlockStreaming verifies that missing or unusable
// trace metadata and bounded lookup failures still allow healthy event delivery.
func TestCorrelationFailuresDoNotBlockStreaming(t *testing.T) {
	for _, outcome := range []string{"not_found", "missing_context", "invalid_context", "ambiguous", "lookup_error", "timeout", "unavailable"} {
		t.Run(outcome, func(t *testing.T) {
			tracer, exporter := testTracer(t)
			var resolver testRunResolver
			if outcome != "unavailable" {
				resolver = resolverFunc(func(ctx context.Context, _ string) (tracing.TestRunContext, error) {
					result := tracing.TestRunContext{Outcome: outcome}
					switch outcome {
					case "not_found", "missing_context":
						return result, nil
					case "timeout":
						<-ctx.Done()
						return result, ctx.Err()
					default:
						return result, errors.New("correlation unavailable")
					}
				})
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			s := &deliveryStream{controlledStream: controlledStream{closed: make(chan struct{})}}
			h := Handler{
				logger: logrus.NewEntry(logrus.New()), ctx: context.Background(),
				streamer: &contextStreamer{stream: s}, metrics: metrics.NewSSEMetrics(), tracer: tracer, resolver: resolver,
			}
			w := cancelOnFlush{httptest.NewRecorder(), cancel}
			r := httptest.NewRequest(http.MethodGet, "/sse/v2alpha/events/run-id", nil).WithContext(ctx)
			start := time.Now()
			h.GetEvents(w, r, httprouter.Params{{Key: "identifier", Value: "run-id"}})
			assert.Less(t, time.Since(start), 2*time.Second)
			assert.Contains(t, w.Body.String(), "event: shutdown")
			spans := exporter.GetSpans()
			require.Len(t, spans, 1)
			assert.Empty(t, spans[0].Links)
			assert.Equal(t, codes.Unset, spans[0].Status.Code)
			assert.Equal(t, outcome, spanAttributes(spans[0])["etos.sse.trace_link.outcome"].AsString())
		})
	}
}

// TestConnectionTraceFailures verifies that backend failures produce bounded span
// errors while expired resume positions are not classified as server failures.
func TestConnectionTraceFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		backend stream.Streamer
		status  int
		errType string
	}{
		{"setup failure", unavailableStreamer{err: errors.New("broker credentials must not be exported")}, 503, "backend_failure"},
		{"consume failure", fakeStreamer{&fakeStream{consumeErr: errors.New("broker failure")}}, 200, "backend_failure"},
		{"resume backend failure", fakeStreamer{&fakeStream{firstErr: errors.New("broker failure")}}, 200, "backend_failure"},
		{"broker closure", &contextStreamer{stream: &closedBrokerStream{}}, 200, "backend_failure"},
		{"expired resume", fakeStreamer{&fakeStream{first: 10}}, 200, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracer, exporter := testTracer(t)
			h := Handler{
				logger: logrus.NewEntry(logrus.New()), ctx: context.Background(),
				streamer: tc.backend, metrics: metrics.NewSSEMetrics(), tracer: tracer,
			}
			r := httptest.NewRequest(http.MethodGet, "/sse/v2alpha/events/run-id", nil)
			r.Header.Set("Last-Event-ID", "1")
			h.GetEvents(httptest.NewRecorder(), r, httprouter.Params{{Key: "identifier", Value: "run-id"}})
			spans := exporter.GetSpans()
			require.Len(t, spans, 1)
			attrs := spanAttributes(spans[0])
			assert.EqualValues(t, tc.status, attrs["http.response.status_code"].AsInt64())
			assert.Equal(t, tc.errType, attrs["error.type"].AsString())
			if tc.errType != "" {
				assert.Equal(t, codes.Error, spans[0].Status.Code)
			} else {
				assert.Equal(t, codes.Unset, spans[0].Status.Code)
			}
			assert.Empty(t, spans[0].Events)
		})
	}
}

// TestConnectionTraceShutdownAndWriteFailure verifies that shutdown aborts and
// write failures end one span each, with only write failures marked as errors.
func TestConnectionTraceShutdownAndWriteFailure(t *testing.T) {
	for _, mode := range []string{"server_shutdown", "stream_write_error"} {
		t.Run(mode, func(t *testing.T) {
			tracer, exporter := testTracer(t)
			appCtx, stop := context.WithCancel(context.Background())
			defer stop()
			s := &deliveryStream{controlledStream: controlledStream{closed: make(chan struct{})}}
			h := Handler{
				logger: logrus.NewEntry(logrus.New()), ctx: appCtx,
				streamer: &contextStreamer{stream: s}, metrics: metrics.NewSSEMetrics(), tracer: tracer,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			r := httptest.NewRequest(http.MethodGet, "/sse/v2alpha/events/run-id", nil).WithContext(ctx)
			params := httprouter.Params{{Key: "identifier", Value: "run-id"}}
			if mode == "server_shutdown" {
				w := cancelOnFlush{httptest.NewRecorder(), stop}
				require.PanicsWithValue(t, http.ErrAbortHandler, func() { h.GetEvents(w, r, params) })
			} else {
				h.GetEvents(&failingWriter{written: make(chan struct{})}, r, params)
			}
			spans := exporter.GetSpans()
			require.Len(t, spans, 1)
			assert.Equal(t, mode, spanAttributes(spans[0])["etos.sse.close_reason"].AsString())
			expected := codes.Unset
			if mode == "stream_write_error" {
				expected = codes.Error
			}
			assert.Equal(t, expected, spans[0].Status.Code)
		})
	}
}

// TestLookupCancellation verifies that disconnects and shutdown cancel or skip
// trace lookups promptly without marking the connection span as failed.
func TestLookupCancellation(t *testing.T) {
	for _, mode := range []string{"client_disconnect", "server_shutdown", "already_shutting_down"} {
		t.Run(mode, func(t *testing.T) {
			tracer, exporter := testTracer(t)
			appCtx, stop := context.WithCancel(context.Background())
			defer stop()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			calls := 0
			resolver := resolverFunc(func(lookupCtx context.Context, _ string) (tracing.TestRunContext, error) {
				calls++
				if mode == "client_disconnect" {
					cancel()
				} else {
					stop()
				}
				<-lookupCtx.Done()
				return tracing.TestRunContext{Outcome: "lookup_error"}, lookupCtx.Err()
			})
			if mode == "already_shutting_down" {
				stop()
			}
			h := Handler{
				logger: logrus.NewEntry(logrus.New()), ctx: appCtx,
				streamer: unavailableStreamer{err: context.Canceled},
				metrics:  metrics.NewSSEMetrics(), tracer: tracer, resolver: resolver,
			}
			r := httptest.NewRequest(http.MethodGet, "/sse/v2alpha/events/run-id", nil).WithContext(ctx)
			start := time.Now()
			h.GetEvents(httptest.NewRecorder(), r, httprouter.Params{{Key: "identifier", Value: "run-id"}})
			assert.Less(t, time.Since(start), 500*time.Millisecond)
			expectedCalls, expectedReason := 1, mode
			if mode == "already_shutting_down" {
				expectedCalls, expectedReason = 0, "server_shutdown"
			}
			assert.Equal(t, expectedCalls, calls)
			spans := exporter.GetSpans()
			require.Len(t, spans, 1)
			assert.Empty(t, spans[0].Links)
			assert.Equal(t, codes.Unset, spans[0].Status.Code)
			assert.Equal(t, expectedReason, spanAttributes(spans[0])["etos.sse.close_reason"].AsString())
		})
	}
}

type traceConfig struct{ cfg }

// ETOSNamespace supplies a namespace when constructing legacy routes in tests.
func (traceConfig) ETOSNamespace() string { return "runs" }

// TestTracingRouteScopeAndDisabledLookups verifies that self-test routes emit no
// spans and disabled v2alpha tracing performs no TestRun lookup.
func TestTracingRouteScopeAndDisabledLookups(t *testing.T) {
	tracer, exporter := testTracer(t)
	logger := logrus.NewEntry(logrus.New())
	resolver := resolverFunc(func(context.Context, string) (tracing.TestRunContext, error) {
		t.Error("unexpected TestRun lookup")
		return tracing.TestRunContext{}, nil
	})
	backend := unavailableStreamer{err: errors.New("broker unavailable")}
	app := New(context.Background(), traceConfig{}, logger, backend, nil).WithTracing(tracer, resolver)
	defer app.Close()
	oldV1 := v1.New(traceConfig{}, logger, context.Background())
	defer oldV1.Close()
	oldAlpha := v1alpha.New(traceConfig{}, logger, context.Background())
	defer oldAlpha.Close()
	router := application.New(app, oldV1, oldAlpha)
	for _, path := range []string{"/sse/v2alpha/selftest/ping", "/sse/v1/selftest/ping", "/sse/v1alpha/selftest/ping"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusNoContent, w.Code)
	}
	disabled := New(context.Background(), traceConfig{}, logger, backend, nil).WithTracing(nil, resolver)
	defer disabled.Close()
	router = application.New(disabled)
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/sse/v2alpha/events/run-id", nil))
	assert.Empty(t, exporter.GetSpans())
}
