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
package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var testRuns = schema.GroupVersionResource{
	Group: "etos.eiffel-community.github.io", Version: "v1beta1", Resource: "testruns",
}

const annotationPrefix = "etos.eiffel-community.github.io/"

// TestRunContext contains only the metadata needed to link an SSE span.
type TestRunContext struct {
	SpanContext trace.SpanContext
	Name        string
	Namespace   string
	Outcome     string
}

// TestRunResolver reads v1beta1 TestRun metadata in one namespace.
type TestRunResolver struct {
	resources dynamic.ResourceInterface
}

// NewTestRunResolver constructs a reusable in-cluster client without querying Kubernetes.
func NewTestRunResolver(namespace string) (*TestRunResolver, error) {
	if namespace == "" {
		return nil, fmt.Errorf("testrun namespace must not be empty")
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("load TestRun client configuration: %w", err)
	}
	cfg.Timeout = time.Second
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create TestRun client: %w", err)
	}
	return &TestRunResolver{resources: client.Resource(testRuns).Namespace(namespace)}, nil
}

// Resolve performs one selector lookup. Missing metadata is expected; errors are
// returned for diagnostics only and must not prevent event delivery.
func (r *TestRunResolver) Resolve(ctx context.Context, identifier string) (TestRunContext, error) {
	result := TestRunContext{Outcome: "not_found"}
	if identifier == "" || len(validation.IsValidLabelValue(identifier)) != 0 {
		result.Outcome = "invalid_identifier"
		return result, fmt.Errorf("invalid TestRun identifier")
	}
	list, err := r.resources.List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{annotationPrefix + "id": identifier}.AsSelector().String(),
		Limit:         2,
	})
	if err != nil {
		result.Outcome = "lookup_error"
		return result, fmt.Errorf("list TestRuns: %w", err)
	}
	if len(list.Items) > 1 || list.GetContinue() != "" {
		result.Outcome = "ambiguous"
		return result, fmt.Errorf("TestRun identifier did not resolve uniquely")
	}
	if len(list.Items) == 0 {
		return result, nil
	}
	run := &list.Items[0]
	result.Name, result.Namespace = run.GetName(), run.GetNamespace()
	annotations := run.GetAnnotations()
	if annotations[annotationPrefix+"traceparent"] == "" {
		result.Outcome = "missing_context"
		return result, nil
	}
	// Extract separately from the request context: this is a link, never a parent.
	linkCtx := propagation.TraceContext{}.Extract(context.Background(), propagation.MapCarrier{
		"traceparent": annotations[annotationPrefix+"traceparent"],
		"tracestate":  annotations[annotationPrefix+"tracestate"],
	})
	result.SpanContext = trace.SpanContextFromContext(linkCtx)
	if !result.SpanContext.IsValid() {
		result.Outcome = "invalid_context"
		return result, fmt.Errorf("invalid TestRun trace context")
	}
	result.Outcome = "linked"
	return result, nil
}
