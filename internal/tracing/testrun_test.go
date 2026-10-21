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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
	ktesting "k8s.io/client-go/testing"
)

const testParent = "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"

// TestResolveTestRunContext verifies namespace-scoped v1beta1 lookup and extraction
// of unique, valid trace context, including missing, ambiguous and failed lookups.
func TestResolveTestRunContext(t *testing.T) {
	for _, tc := range []struct {
		name       string
		parent     string
		count      int
		continueID string
		err        error
		outcome    string
		wantError  bool
	}{
		{name: "linked", parent: testParent, count: 1, outcome: "linked"},
		{name: "absent", outcome: "not_found"},
		{name: "missing annotation", count: 1, outcome: "missing_context"},
		{name: "invalid annotation", parent: "not-a-parent", count: 1, outcome: "invalid_context", wantError: true},
		{name: "zero IDs", parent: "00-00000000000000000000000000000000-0000000000000000-01", count: 1, outcome: "invalid_context", wantError: true},
		{name: "ambiguous", parent: testParent, count: 2, outcome: "ambiguous", wantError: true},
		{name: "truncated list", parent: testParent, count: 1, continueID: "next", outcome: "ambiguous", wantError: true},
		{name: "API error", err: errors.New("API unavailable"), outcome: "lookup_error", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
				testRuns: "TestRunList",
			})
			client.PrependReactor("list", "testruns", func(action ktesting.Action) (bool, runtime.Object, error) {
				assert.Equal(t, testRuns, action.GetResource())
				assert.Equal(t, "runs", action.GetNamespace())
				listAction, ok := action.(ktesting.ListAction)
				require.True(t, ok)
				assert.Equal(t, annotationPrefix+"id=run-id", listAction.GetListRestrictions().Labels.String())
				if tc.err != nil {
					return true, nil, tc.err
				}
				list := &unstructured.UnstructuredList{}
				list.SetContinue(tc.continueID)
				for range tc.count {
					run := unstructured.Unstructured{}
					run.SetName("generated-run")
					run.SetNamespace("runs")
					run.SetLabels(map[string]string{annotationPrefix + "id": "run-id"})
					run.SetAnnotations(map[string]string{
						annotationPrefix + "traceparent": tc.parent,
						annotationPrefix + "tracestate":  "vendor=value",
						annotationPrefix + "baggage":     "secret=not-propagated",
					})
					list.Items = append(list.Items, run)
				}
				return true, list, tc.err
			})
			resolver := &TestRunResolver{resources: client.Resource(testRuns).Namespace("runs")}
			result, err := resolver.Resolve(context.Background(), "run-id")
			assert.Equal(t, tc.wantError, err != nil)
			assert.Equal(t, tc.outcome, result.Outcome)
			require.Len(t, client.Actions(), 1)
			if tc.outcome == "linked" {
				assert.Equal(t, "0123456789abcdef0123456789abcdef", result.SpanContext.TraceID().String())
				assert.Equal(t, "0123456789abcdef", result.SpanContext.SpanID().String())
				assert.Equal(t, "vendor=value", result.SpanContext.TraceState().String())
				assert.True(t, result.SpanContext.IsRemote())
				assert.Equal(t, "generated-run", result.Name)
				assert.Equal(t, "runs", result.Namespace)
			} else {
				assert.False(t, result.SpanContext.IsValid())
			}
		})
	}
}

// TestResolveRejectsInvalidSelectorValues verifies that invalid identifiers never
// reach Kubernetes and that an empty namespace cannot create a resolver.
func TestResolveRejectsInvalidSelectorValues(t *testing.T) {
	for _, identifier := range []string{"", "run,other=value", "../run", "run/id"} {
		t.Run(identifier, func(t *testing.T) {
			// A nil client also verifies that invalid identifiers never trigger a query.
			result, err := (&TestRunResolver{}).Resolve(context.Background(), identifier)
			require.Error(t, err)
			assert.Equal(t, "invalid_identifier", result.Outcome)
		})
	}
	_, err := NewTestRunResolver("")
	require.Error(t, err)
}

// TestResolveHTTPFailures verifies the Kubernetes request path, selector and limit,
// and that forbidden or timed-out requests return errors without usable trace context.
func TestResolveHTTPFailures(t *testing.T) {
	for _, name := range []string{"forbidden", "timeout"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/apis/etos.eiffel-community.github.io/v1beta1/namespaces/runs/testruns", r.URL.Path)
				assert.Equal(t, annotationPrefix+"id=run-id", r.URL.Query().Get("labelSelector"))
				assert.Equal(t, "2", r.URL.Query().Get("limit"))
				if name == "timeout" {
					<-r.Context().Done()
					return
				}
				http.Error(w, "forbidden", http.StatusForbidden)
			}))
			defer server.Close()
			client, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
			require.NoError(t, err)
			resolver := &TestRunResolver{resources: client.Resource(testRuns).Namespace("runs")}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			result, err := resolver.Resolve(ctx, "run-id")
			require.Error(t, err)
			assert.Equal(t, "lookup_error", result.Outcome)
			assert.False(t, result.SpanContext.IsValid())
		})
	}
}
