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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/eiffel-community/etos-api/internal/stream"
	"github.com/eiffel-community/etos-api/pkg/application"
	"github.com/eiffel-community/etos-api/pkg/events"
	schema "github.com/eiffel-community/etos/schemas/messaging/v2alpha"
	"github.com/julienschmidt/httprouter"

	"github.com/sirupsen/logrus"
)

// pingInterval is a variable so that tests can shorten it.
var pingInterval = 15 * time.Second

type Application struct {
	logger   *logrus.Entry
	cfg      config.SSEConfig
	ctx      context.Context
	cancel   context.CancelFunc
	streamer stream.Streamer
}

type Handler struct {
	logger   *logrus.Entry
	cfg      config.SSEConfig
	ctx      context.Context
	streamer stream.Streamer
}

// Close cancels the application context.
func (a *Application) Close() {
	a.cancel()
	a.streamer.Close()
}

// New returns a new Application object/struct.
func New(ctx context.Context, cfg config.SSEConfig, log *logrus.Entry, streamer stream.Streamer) application.Application {
	ctx, cancel := context.WithCancel(ctx)
	return &Application{
		logger:   log,
		cfg:      cfg,
		ctx:      ctx,
		cancel:   cancel,
		streamer: streamer,
	}
}

// LoadRoutes loads all the v2alpha routes.
func (a Application) LoadRoutes(router *httprouter.Router) {
	handler := &Handler{a.logger, a.cfg, a.ctx, a.streamer}
	router.GET("/sse/v2alpha/selftest/ping", handler.Selftest)
	router.GET("/sse/v2alpha/events/:identifier", handler.GetEvents)
}

// Selftest is a handler to just return 204.
func (h Handler) Selftest(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusNoContent)
}

// cleanFilter will clean up the filters received from clients.
func (h Handler) cleanFilter(identifier string, filters []string) {
	for i, filter := range filters {
		if len(strings.Split(filter, ".")) != 3 {
			filters[i] = fmt.Sprintf("%s.%s", identifier, filter)
		}
	}
}

type ErrorEvent struct {
	Retry  bool   `json:"retry"`
	Reason string `json:"reason"`
}

// Reasons for non-retryable errors when resuming a stream.
const (
	reasonExpired = "events after the requested Last-Event-ID have expired from the event stream"
	reasonUnknown = "the requested Last-Event-ID is unknown to the event stream"
)

// sendError sends an error event, with a retry hint, to the client.
func sendError(ch chan<- events.Event, retry bool, reason string) {
	b, _ := json.Marshal(ErrorEvent{Retry: retry, Reason: reason})
	ch <- events.Event{Event: "error", Data: string(b)}
}

// resumeOffset returns the stream offset to start consuming from for a client that last received
// the event with ID lastID, where 0 means that no event has been received. Event IDs are stream
// offsets plus one, so the last event ID is also the offset of the next message to consume.
// A non-empty reason is returned if the stream cannot be resumed.
func resumeOffset(streamer stream.Stream, lastID int64) (offset int64, retry bool, reason string, err error) {
	if lastID <= 0 {
		return stream.OffsetFirst, false, "", nil
	}
	first, err := streamer.FirstOffset()
	if errors.Is(err, stream.ErrEmptyStream) {
		return 0, false, reasonUnknown, err
	}
	if err != nil {
		return 0, true, "failed to query the event stream", err
	}
	if first > lastID {
		return 0, false, reasonExpired, fmt.Errorf("first offset in stream is %d, want %d", first, lastID)
	}
	return lastID, false, "", nil
}

// subscribe subscribes to stream and gets logs and events from it and writes them to a channel.
//
// The ID of each event is the offset of its message in the stream plus one, which makes the ID
// the offset to resume from. The ID 0 cannot be used because it is not written on the wire and
// clients use it to mean that no event has been received, so a client that has received the
// message at offset 0 could not be told apart from a new client. Pings carry the ID of the
// stream position when it is greater than the last ID sent, which lets clients resume after
// messages that were filtered out.
func (h Handler) subscribe(ctx context.Context, logger *logrus.Entry, streamer stream.Stream, ch chan<- events.Event, lastID int64, filter []string) {
	defer close(ch)
	var err error

	offset, retry, reason, err := resumeOffset(streamer, lastID)
	if reason != "" {
		logger.WithError(err).WithField("lastEventID", lastID).Error("Could not resume the event stream")
		sendError(ch, retry, reason)
		return
	}

	// The channel must be unbuffered for the Position of the stream to be correct.
	consumeCh := make(chan stream.Message)

	closed, err := streamer.WithChannel(consumeCh).WithOffset(offset).WithFilter(filter).Consume(ctx)
	if err != nil {
		logger.WithError(err).Error("failed to start consuming stream")
		sendError(ch, true, "failed to consume the event stream")
		return
	}
	defer streamer.Close()

	// Retention may have removed the requested offset after it was checked, in which case the
	// consumer silently starts at the new first offset.
	if offset != stream.OffsetFirst {
		if _, _, reason, err := resumeOffset(streamer, lastID); reason == reasonExpired {
			logger.WithError(err).WithField("lastEventID", lastID).Error("Could not resume the event stream")
			sendError(ch, false, reason)
			return
		}
	}

	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	var event events.Event
	for {
		select {
		case <-ctx.Done():
			logger.Info("Client lost, closing subscriber")
			return
		case <-ping.C:
			event = events.Event{Event: "ping"}
			// Every message below the position has already been sent on ch, or filtered out,
			// so a client resuming from here will not miss any events.
			if position := streamer.Position(); position > lastID {
				event.ID = int(position)
				lastID = position
			}
			ch <- event
		case <-closed:
			logger.Info("Stream closed, closing down")
			sendError(ch, true, "Streamer closed the connection")
			return
		case msg := <-consumeCh:
			event, err = events.New(msg.Data)
			if err != nil {
				logger.WithError(err).Error("failed to parse SSE event")
				continue
			}
			if err := schema.Validate(msg.Data); err != nil {
				logger.WithError(err).Warning("dropping SSE event that does not match the protocol")
				continue
			}
			event.ID = int(msg.Offset + 1)
			lastID = msg.Offset + 1
			ch <- event
		}
	}
}

// GetEvents is an endpoint for streaming events and logs from ETOS.
func (h Handler) GetEvents(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	identifier := ps.ByName("identifier")
	// Filters may be passed multiple times (?filter=log.info&filter=log.debug)
	// in order to parse multiple values into a slice r.ParseForm() is used.
	// The filters are accessible in r.Form["filter"] after r.ParseForm() has been
	// called.
	r.ParseForm()

	// Making it possible for us to correlate logs to a specific connection
	logger := h.logger.WithField("identifier", identifier)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	var lastID int64
	lastEventID := r.Header.Get("Last-Event-ID")
	if lastEventID != "" {
		var err error
		lastID, err = strconv.ParseInt(lastEventID, 10, 64)
		if err != nil || lastID < 0 {
			logger.Error("Last-Event-ID header is not parsable, streaming from the start")
			lastID = 0
		}
	}

	filter := r.Form["filter"]
	h.cleanFilter(identifier, filter)

	streamer, err := h.streamer.NewStream(r.Context(), logger, identifier)
	if err != nil {
		// Failing to open a stream is a server-side (broker) failure and not caused by the
		// client request, so report it as retryable and keep the details out of the response.
		logger.WithError(err).Error("Could not start a new stream")
		http.Error(w, "event stream is temporarily unavailable", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.NotFound(w, r)
		return
	}
	logger.Info("Client connected to SSE")

	ctx, cancel := context.WithCancel(r.Context())
	receiver := make(chan events.Event) // Channel is closed in Subscriber
	go h.subscribe(ctx, logger, streamer, receiver, lastID, filter)
	defer func() {
		// Stop the subscriber and drain the channel so that it never blocks on a send, and has
		// closed its stream, when this handler returns.
		cancel()
		for range receiver {
		}
	}()

	for {
		select {
		case <-r.Context().Done():
			logger.Info("Client gone from SSE")
			return
		case <-h.ctx.Done():
			logger.Info("Shutting down")
			return
		case event, ok := <-receiver:
			if !ok {
				return
			}
			if err := event.Write(w); err != nil {
				logger.Error(err)
				continue
			}
			flusher.Flush()
		}
	}
}
