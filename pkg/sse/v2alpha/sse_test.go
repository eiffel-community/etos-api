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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

// getEvents runs GetEvents against a file stream containing lines until the timeout and returns
// the response body.
func getEvents(t *testing.T, testrunID string, lines string, lastEventID string) string {
	t.Helper()
	assert.NoError(t, os.WriteFile(testrunID, []byte(lines), 0644))
	t.Cleanup(func() { _ = os.Remove(testrunID) })
	ctx, done := context.WithTimeout(context.Background(), time.Second*1)
	defer done()

	log := logrus.WithFields(logrus.Fields{})
	streamer, err := stream.NewFileStreamer(10*time.Millisecond, log)
	assert.NoError(t, err)
	handler := Handler{log, &cfg{}, context.Background(), streamer}
	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", fmt.Sprintf("/v2alpha/events/%s", testrunID), nil)
	if lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	request = request.WithContext(ctx)
	ps := httprouter.Params{httprouter.Param{Key: "identifier", Value: testrunID}}
	handler.GetEvents(responseRecorder, request, ps)
	assert.Equal(t, http.StatusOK, responseRecorder.Code)
	return responseRecorder.Body.String()
}

const (
	invalidLine = `{"event":"message","data":"invalid"}`
	shutdownA   = `{"event":"shutdown","data":{"conclusion":"Successful","verdict":"Passed","description":"a"}}`
	shutdownB   = `{"event":"shutdown","data":{"conclusion":"Successful","verdict":"Passed","description":"b"}}`
)

// TestSSEGetEventsOffsetIDs tests that event IDs are the stream offset plus one and that
// dropped events do not shift the IDs of other events.
func TestSSEGetEventsOffsetIDs(t *testing.T) {
	body := getEvents(t, "test_sse_offset_ids", invalidLine+"\n"+shutdownA+"\n"+shutdownB+"\n", "")
	assert.NotContains(t, body, "invalid")
	assert.Contains(t, body, "id: 2\nevent: shutdown\ndata: {\"conclusion\":\"Successful\",\"description\":\"a\"")
	assert.Contains(t, body, "id: 3\nevent: shutdown\ndata: {\"conclusion\":\"Successful\",\"description\":\"b\"")
	assert.NotContains(t, body, "id: 1\n")
}

// TestSSEGetEventsResume tests that a client resuming with Last-Event-ID only receives the
// events after that ID.
func TestSSEGetEventsResume(t *testing.T) {
	body := getEvents(t, "test_sse_resume", invalidLine+"\n"+shutdownA+"\n"+shutdownB+"\n", "2")
	assert.NotContains(t, body, `"description":"a"`)
	assert.Contains(t, body, "id: 3\nevent: shutdown\ndata: {\"conclusion\":\"Successful\",\"description\":\"b\"")
}

// TestSSEGetEventsMalformedLastEventID tests that a Last-Event-ID that cannot be parsed makes
// the server stream from the start.
func TestSSEGetEventsMalformedLastEventID(t *testing.T) {
	for _, lastEventID := range []string{"not-a-number", "-5"} {
		t.Run(lastEventID, func(t *testing.T) {
			body := getEvents(t, "test_sse_malformed_last_event_id", shutdownA+"\n", lastEventID)
			assert.Contains(t, body, "id: 1\nevent: shutdown\n")
		})
	}
}

// fakeStream is a stream.Stream with a fixed first offset and position that never delivers
// any messages.
type fakeStream struct {
	// firstAfterConsume, if set, is the first offset once Consume has been called.
	firstAfterConsume int64
	consumeErr        error
	first             int64
	firstErr          error
	position          int64
	offset            int64
	consumed          bool
}

func (s *fakeStream) WithChannel(chan<- stream.Message) stream.Stream { return s }
func (s *fakeStream) WithOffset(offset int64) stream.Stream           { s.offset = offset; return s }
func (s *fakeStream) WithFilter([]string) stream.Stream               { return s }
func (s *fakeStream) Consume(context.Context) (<-chan error, error) {
	if s.consumeErr != nil {
		return nil, s.consumeErr
	}
	s.consumed = true
	if s.firstAfterConsume != 0 {
		s.first = s.firstAfterConsume
	}
	return make(chan error), nil
}
func (s *fakeStream) FirstOffset() (int64, error) { return s.first, s.firstErr }
func (s *fakeStream) Position() int64             { return s.position }
func (s *fakeStream) Close()                      {}

// fakeStreamer is a stream.Streamer returning a fakeStream.
type fakeStreamer struct {
	stream *fakeStream
}

func (s fakeStreamer) NewStream(context.Context, *logrus.Entry, string) (stream.Stream, error) {
	return s.stream, nil
}
func (s fakeStreamer) CreateStream(context.Context, *logrus.Entry, string) error { return nil }
func (s fakeStreamer) Close()                                                    {}

// getFakeEvents runs GetEvents against a fakeStream until the timeout and returns the body.
func getFakeEvents(t *testing.T, fake *fakeStream, lastEventID string, timeout time.Duration) string {
	t.Helper()
	ctx, done := context.WithTimeout(context.Background(), timeout)
	defer done()
	log := logrus.WithFields(logrus.Fields{})
	handler := Handler{log, &cfg{}, context.Background(), fakeStreamer{fake}}
	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/v2alpha/events/fake", nil)
	if lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	request = request.WithContext(ctx)
	ps := httprouter.Params{httprouter.Param{Key: "identifier", Value: "fake"}}
	handler.GetEvents(responseRecorder, request, ps)
	assert.Equal(t, http.StatusOK, responseRecorder.Code)
	return responseRecorder.Body.String()
}

// TestSSEGetEventsResumeExpired tests that resuming from an event that has been removed from
// the stream by retention is a non-retryable error.
func TestSSEGetEventsResumeExpired(t *testing.T) {
	fake := &fakeStream{first: 10}
	body := getFakeEvents(t, fake, "5", time.Second)
	assert.Equal(t, "event: error\ndata: {\"retry\":false,\"reason\":\""+reasonExpired+"\"}\n\n", body)
	assert.False(t, fake.consumed)
}

// TestSSEGetEventsResumeUnknown tests that resuming an empty stream is a non-retryable error.
func TestSSEGetEventsResumeUnknown(t *testing.T) {
	fake := &fakeStream{firstErr: stream.ErrEmptyStream}
	body := getFakeEvents(t, fake, "5", time.Second)
	assert.Equal(t, "event: error\ndata: {\"retry\":false,\"reason\":\""+reasonUnknown+"\"}\n\n", body)
	assert.False(t, fake.consumed)
}

// TestSSEGetEventsResumeFromFirstOffset tests that a client can resume from the first offset
// retained in the stream and that the stream is consumed from the last event ID.
func TestSSEGetEventsResumeFromFirstOffset(t *testing.T) {
	fake := &fakeStream{first: 10}
	body := getFakeEvents(t, fake, "10", 100*time.Millisecond)
	assert.NotContains(t, body, "error")
	assert.True(t, fake.consumed)
	assert.Equal(t, int64(10), fake.offset)
}

// TestSSEGetEventsPingIDs tests that pings carry the stream position as ID only when it is
// greater than the last event ID sent to the client.
func TestSSEGetEventsPingIDs(t *testing.T) {
	original := pingInterval
	pingInterval = 20 * time.Millisecond
	t.Cleanup(func() { pingInterval = original })

	t.Run("position ahead of client", func(t *testing.T) {
		body := getFakeEvents(t, &fakeStream{position: 50}, "", 70*time.Millisecond)
		assert.True(t, strings.HasPrefix(body, "id: 50\nevent: ping\ndata: \n\nevent: ping\ndata: \n\n"), body)
	})
	t.Run("position equal to client", func(t *testing.T) {
		body := getFakeEvents(t, &fakeStream{position: 50}, "50", 70*time.Millisecond)
		assert.True(t, strings.HasPrefix(body, "event: ping\ndata: \n\n"), body)
		assert.NotContains(t, body, "id:")
	})
}

// TestSSEGetEventsResumeExpiredAfterConsume verifies that events expiring between the resume
// check and the start of consumption result in a non-retryable error.
func TestSSEGetEventsResumeExpiredAfterConsume(t *testing.T) {
	fake := &fakeStream{first: 5, firstAfterConsume: 10}
	body := getFakeEvents(t, fake, "5", time.Second)
	assert.Equal(t, "event: error\ndata: {\"retry\":false,\"reason\":\""+reasonExpired+"\"}\n\n", body)
	assert.True(t, fake.consumed)
}

// TestSSEGetEventsConsumeError verifies that a failure to start consuming the stream results in a
// retryable error that does not expose the underlying error to the client.
func TestSSEGetEventsConsumeError(t *testing.T) {
	fake := &fakeStream{consumeErr: errors.New("broker internals")}
	body := getFakeEvents(t, fake, "", time.Second)
	assert.Equal(t, "event: error\ndata: {\"retry\":true,\"reason\":\"failed to consume the event stream\"}\n\n", body)
}
