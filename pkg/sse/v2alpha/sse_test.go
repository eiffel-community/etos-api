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
package sse

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/eiffel-community/etos-api/internal/stream"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type cfg struct {
	config.Config
}

func (c cfg) RabbitMQURI() string {
	return ""
}

func (c cfg) RabbitMQStreamName() string {
	return "test"
}

// TestSSECreateStream tests that the CreateStream endpoint works.
// Does not test the authorization middleware!
func TestSSECreateStream(t *testing.T) {
	log := logrus.WithFields(logrus.Fields{})
	streamer, err := stream.NewFileStreamer(100*time.Millisecond, log)
	assert.NoError(t, err)
	handler := Handler{log, &cfg{}, context.Background(), streamer}
	responseRecorder := httptest.NewRecorder()
	testrunID := "test_sse_create_stream"
	request := httptest.NewRequest("GET", fmt.Sprintf("/v2alpha/stream/%s", testrunID), nil)
	ps := httprouter.Params{httprouter.Param{Key: "identifier", Value: testrunID}}
	handler.CreateStream(responseRecorder, request, ps)
	defer func() {
		os.Remove(testrunID)
	}()
	assert.Equal(t, http.StatusCreated, responseRecorder.Code)
	assert.FileExists(t, testrunID)
}

// TestSSEGetEvents tests that a client can subscribe to an SSE stream and get events.
func TestSSEGetEvents(t *testing.T) {
	data := []byte(`{"data":"hello world","event":"message"}`)
	testrunID := "test_sse_get_events"
	os.WriteFile(testrunID, data, 0644)
	defer func() {
		os.Remove(testrunID)
	}()
	ctx, done := context.WithTimeout(context.Background(), time.Second*1)
	defer done()

	log := logrus.WithFields(logrus.Fields{})
	streamer, err := stream.NewFileStreamer(100*time.Millisecond, log)
	assert.NoError(t, err)
	handler := Handler{log, &cfg{}, context.Background(), streamer}
	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", fmt.Sprintf("/v2alpha/events/%s", testrunID), nil)
	request = request.WithContext(ctx)
	ps := httprouter.Params{httprouter.Param{Key: "identifier", Value: testrunID}}
	handler.GetEvents(responseRecorder, request, ps)

	assert.Equal(t, http.StatusOK, responseRecorder.Code)
	body, err := io.ReadAll(responseRecorder.Body)
	assert.NoError(t, err)
	fmt.Println(string(body))
	assert.Equal(t, body, []byte(`id: 1
event: message
data: "hello world"

`))
}
