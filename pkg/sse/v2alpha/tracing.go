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
	"net/http"
	"time"

	"github.com/eiffel-community/etos-api/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

type testRunResolver interface {
	Resolve(context.Context, string) (tracing.TestRunContext, error)
}

// WithTracing enables optional connection tracing. A nil resolver leaves spans
// unlinked, and a nil tracer disables both spans and lookups.
func (a *Application) WithTracing(tracer trace.Tracer, resolver testRunResolver) *Application {
	a.tracer, a.resolver = tracer, resolver
	return a
}

// startSpan resolves an optional TestRun link before starting the connection span,
// preserving the HTTP parent and including lookup time in the span duration.
func (h Handler) startSpan(r *http.Request, identifier string) (*http.Request, trace.Span) {
	if h.tracer == nil {
		return r, noop.Span{}
	}
	start := time.Now()
	result := tracing.TestRunContext{Outcome: "unavailable"}
	if h.resolver != nil && h.ctx.Err() == nil && r.Context().Err() == nil {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		stop := context.AfterFunc(h.ctx, cancel)
		var err error
		result, err = h.resolver.Resolve(ctx, identifier)
		stop()
		cancel()
		if err != nil {
			h.logger.WithError(err).WithField("identifier", identifier).
				WithField("trace_link_outcome", result.Outcome).Warn("Could not resolve TestRun trace link")
		}
	}
	opts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithTimestamp(start),
		trace.WithAttributes(
			attribute.String("http.request.method", r.Method),
			attribute.String("http.route", "/sse/v2alpha/events/:identifier"),
			attribute.String("etos.id", identifier),
			attribute.Bool("etos.sse.reconnect", r.Header.Get("Last-Event-ID") != ""),
			attribute.String("etos.sse.trace_link.outcome", result.Outcome),
		),
	}
	if result.SpanContext.IsValid() {
		opts = append(opts, trace.WithLinks(trace.Link{SpanContext: result.SpanContext}))
	}
	if result.Name != "" {
		opts = append(opts, trace.WithAttributes(
			attribute.String("etos.testrun.name", result.Name),
			attribute.String("k8s.namespace.name", result.Namespace),
		))
	}
	ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	ctx, span := h.tracer.Start(ctx, "GET /sse/v2alpha/events/:identifier", opts...)
	return r.WithContext(ctx), span
}

// traceFailure marks an active connection span with a bounded error category,
// ignoring failures after cancellation and when tracing is disabled.
func (h Handler) traceFailure(ctx context.Context, category string) {
	if h.tracer == nil || ctx.Err() != nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("error.type", category))
	span.SetStatus(codes.Error, category)
}
