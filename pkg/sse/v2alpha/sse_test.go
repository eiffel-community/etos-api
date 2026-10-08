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
	"bufio"
	"context"
	"errors"
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
	"github.com/stretchr/testify/require"
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

// TestSSEGetEvents tests that a client can subscribe to an SSE stream and get events.
func TestSSEGetEvents(t *testing.T) {
	data := []byte(`{"event":"message","data":{"message":"hello world","name":"etos","@timestamp":"2026-08-31T10:00:00Z"}}`)
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
data: {"@timestamp":"2026-08-31T10:00:00Z","message":"hello world","name":"etos"}

`))
}

// TestSSEGetEventsDropsInvalid tests that events which do not match the protocol
// are dropped and never forwarded to the client.
func TestSSEGetEventsDropsInvalid(t *testing.T) {
	// A "message" event whose data is a plain string does not match the Log
	// protocol and must be dropped.
	data := []byte(`{"event":"message","data":"hello world"}`)
	testrunID := "test_sse_drops_invalid"
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
	// The invalid event is dropped, so no data event is written to the client.
	assert.NotContains(t, string(body), "hello world")
}

// unavailableStreamer is a stream.Streamer whose NewStream always fails, simulating a
// broker that cannot be reached when a client subscribes.
type unavailableStreamer struct {
	err error
}

// NewStream returns the configured error.
func (s unavailableStreamer) NewStream(context.Context, *logrus.Entry, string) (stream.Stream, error) {
	return nil, s.err
}

// CreateStream does nothing.
func (s unavailableStreamer) CreateStream(context.Context, *logrus.Entry, string) error {
	return nil
}

// Close does nothing.
func (s unavailableStreamer) Close() {}

// TestSSEGetEventsStreamUnavailable tests that a failure to open a stream is reported as
// a retryable 503 Service Unavailable, without leaking the backend error to the client.
func TestSSEGetEventsStreamUnavailable(t *testing.T) {
	backendErr := errors.New("dial tcp rabbitmq.internal:5552: connection refused")
	log := logrus.WithFields(logrus.Fields{})
	handler := Handler{log, &cfg{}, context.Background(), unavailableStreamer{err: backendErr}}
	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/v2alpha/events/test_sse_stream_unavailable", nil)
	ps := httprouter.Params{httprouter.Param{Key: "identifier", Value: "test_sse_stream_unavailable"}}
	handler.GetEvents(responseRecorder, request, ps)

	assert.Equal(t, http.StatusServiceUnavailable, responseRecorder.Code)
	assert.Equal(t, "text/plain; charset=utf-8", responseRecorder.Header().Get("Content-Type"))
	body := responseRecorder.Body.String()
	assert.Equal(t, "event stream is temporarily unavailable\n", body)
	assert.NotContains(t, body, "rabbitmq.internal")
}

// TestSSEGetEventsShuttingDown tests that a stream is not started when the application is
// shutting down, and that the client is told to retry.
func TestSSEGetEventsShuttingDown(t *testing.T) {
	appCtx, stop := context.WithCancel(context.Background())
	stop()

	log := logrus.WithFields(logrus.Fields{})
	streamer, err := stream.NewFileStreamer(100*time.Millisecond, log)
	assert.NoError(t, err)
	handler := Handler{log, &cfg{}, appCtx, streamer}
	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/v2alpha/events/test_sse_shutting_down", nil)
	ps := httprouter.Params{httprouter.Param{Key: "identifier", Value: "test_sse_shutting_down"}}
	handler.GetEvents(responseRecorder, request, ps)

	assert.Equal(t, http.StatusServiceUnavailable, responseRecorder.Code)
}

// TestSSEGetEventsShutdownAbortsStream tests that an active stream is aborted, rather than
// ended cleanly, when the application shuts down, so that clients reconnect.
func TestSSEGetEventsShutdownAbortsStream(t *testing.T) {
	data := []byte(`{"event":"message","data":{"message":"hello world","name":"etos","@timestamp":"2026-08-31T10:00:00Z"}}`)
	testrunID := "test_sse_shutdown_aborts_stream"
	os.WriteFile(testrunID, data, 0644)
	defer func() {
		os.Remove(testrunID)
	}()
	appCtx, stop := context.WithCancel(context.Background())
	defer stop()

	log := logrus.WithFields(logrus.Fields{})
	streamer, err := stream.NewFileStreamer(10*time.Millisecond, log)
	assert.NoError(t, err)
	handler := Handler{log, &cfg{}, appCtx, streamer}
	router := httprouter.New()
	router.GET("/v2alpha/events/:identifier", handler.GetEvents)
	server := httptest.NewServer(router)
	defer server.Close()

	response, err := http.Get(fmt.Sprintf("%s/v2alpha/events/%s", server.URL, testrunID))
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, http.StatusOK, response.StatusCode)

	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	assert.NoError(t, err)
	assert.Equal(t, "id: 1\n", line)

	stop()
	result := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(reader)
		result <- err
	}()
	select {
	case err := <-result:
		// A cleanly ended stream would return a nil error here.
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	case <-time.After(5 * time.Second):
		t.Fatal("stream was not closed after the application shut down")
	}
}
