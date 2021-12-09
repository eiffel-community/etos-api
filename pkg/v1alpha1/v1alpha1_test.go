// Copyright 2021 Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package v1alpha1

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/eiffel-community/etos-api/pkg/application"
	"github.com/eiffel-community/etos-api/test/testconfig"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TestNew verifies that New creates an Application interface.
func TestNew(t *testing.T) {
	ctx := context.Background()
	log := &logrus.Entry{}
	cfg := testconfig.Get("", "", "", "", "", "", "1m")
	v1alpha1 := New(cfg, log, ctx)
	assert.Implements(t, (*application.Application)(nil), v1alpha1)
}

// TestLoadRoutes verifies that it is possible to load the v1alpha1 routes.
func TestLoadRoutes(t *testing.T) {
	router := httprouter.New()
	v1alpha1 := &V1Alpha1Application{}
	v1alpha1.LoadRoutes(router)

	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/v1alpha1/selftest/ping", nil)
	router.ServeHTTP(responseRecorder, request)
	assert.Equal(t, 204, responseRecorder.Code)
}

// TestSelftest verifies that the selftest endpoint returns 204 No Content.
func TestSelftest(t *testing.T) {
	router := application.New(&V1Alpha1Application{})

	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/v1alpha1/selftest/ping", nil)
	router.ServeHTTP(responseRecorder, request)
	assert.Equal(t, 204, responseRecorder.Code)
}
