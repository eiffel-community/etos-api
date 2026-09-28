// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
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

package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/eiffel-community/etos-api/test/testconfig"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TestNewWebService tests that a new webservice can be created and that it
// implements the Server interface
func TestNewWebService(t *testing.T) {
	log := &logrus.Entry{}
	cfg := testconfig.Get("", "", "", "", "")
	webserver := NewWebService(cfg, log, http.Handler(nil))
	assert.Implements(t, (*Server)(nil), webserver)
}

// TestWebServiceOnShutdown tests that functions registered with OnShutdown are called when
// the webservice is closed.
func TestWebServiceOnShutdown(t *testing.T) {
	log := logrus.NewEntry(logrus.New())
	cfg := testconfig.Get("", "", "", "", "")
	webserver := NewWebService(cfg, log, http.Handler(nil))
	called := make(chan struct{})
	webserver.OnShutdown(func() { close(called) })

	assert.NoError(t, webserver.Close(context.Background()))
	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("OnShutdown function was not called on Close")
	}
}
