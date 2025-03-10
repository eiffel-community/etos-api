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
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	auth "github.com/eiffel-community/etos-api/internal/authorization"
	"github.com/eiffel-community/etos-api/internal/authorization/scope"
	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/eiffel-community/etos-api/internal/stream"
	"github.com/eiffel-community/etos-api/pkg/application"
	"github.com/eiffel-community/etos-api/pkg/events"
	"github.com/julienschmidt/httprouter"

	"github.com/sirupsen/logrus"
)

const pingInterval = 15 * time.Second

type Application struct {
	logger     *logrus.Entry
	cfg        config.SSEConfig
	ctx        context.Context
	cancel     context.CancelFunc
	streamer   stream.Streamer
	authorizer *auth.Authorizer
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
func New(ctx context.Context, cfg config.SSEConfig, log *logrus.Entry, streamer stream.Streamer, authorizer *auth.Authorizer) application.Application {
	ctx, cancel := context.WithCancel(ctx)
	return &Application{
		logger:     log,
		cfg:        cfg,
		ctx:        ctx,
		cancel:     cancel,
		streamer:   streamer,
		authorizer: authorizer,
	}
}

// LoadRoutes loads all the v2alpha routes.
func (a Application) LoadRoutes(router *httprouter.Router) {
	handler := &Handler{a.logger, a.cfg, a.ctx, a.streamer}
	router.GET("/sse/v2alpha/selftest/ping", handler.Selftest)
	router.GET("/sse/v2alpha/events/:identifier", a.authorizer.Middleware(scope.StreamSSE, handler.GetEvents))
	router.POST("/sse/v2alpha/stream/:identifier", a.authorizer.Middleware(scope.DefineSSE, handler.CreateStream))
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

// subscribe subscribes to stream and gets logs and events from it and writes them to a channel.
func (h Handler) subscribe(ctx context.Context, logger *logrus.Entry, streamer stream.Stream, ch chan<- events.Event, lastID int, filter []string) {
	defer close(ch)
	var err error

	consumeCh := make(chan []byte, 0)

	offset := -1
	counter := 1 // lastID will default to 1 and the first event will be 1

	closed, err := streamer.WithChannel(consumeCh).WithOffset(offset).WithFilter(filter).Consume(ctx)
	if err != nil {
		logger.WithError(err).Error("failed to start consuming stream")
		b, _ := json.Marshal(ErrorEvent{Retry: false, Reason: err.Error()})
		ch <- events.Event{Event: "error", Data: string(b)}
		return
	}
	defer streamer.Close()

	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	var event events.Event
	for {
		select {
		case <-ctx.Done():
			logger.Info("Client lost, closing subscriber")
			return
		case <-ping.C:
			ch <- events.Event{Event: "ping"}
		case <-closed:
			logger.Info("Stream closed, closing down")
			b, _ := json.Marshal(ErrorEvent{Retry: true, Reason: "Streamer closed the connection"})
			ch <- events.Event{Event: "error", Data: string(b)}
			return
		case msg := <-consumeCh:
			// We have no reliable way of getting a specific offset on the SSE stream so
			// we will need to iterate all events until we reach the last known ID.
			if counter < lastID {
				counter++
				continue
			}

			event, err = events.New(msg)
			if err != nil {
				logger.WithError(err).Error("failed to parse SSE event")
				continue
			}
			// TODO: https://github.com/eiffel-community/etos/issues/299
			if event.JSONData == nil {
				event = events.Event{
					Data:  string(msg),
					Event: "message",
				}
			}
			event.ID = counter
			ch <- event
			counter++
		}
	}
}

// CreateStream creates a new RabbitMQ stream for use with the sse events stream.
func (h Handler) CreateStream(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	identifier := ps.ByName("identifier")
	// Making it possible for us to correlate logs to a specific connection
	logger := h.logger.WithField("identifier", identifier)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	err := h.streamer.CreateStream(r.Context(), logger, identifier)
	if err != nil {
		logger.WithError(err).Error("failed to create a rabbitmq stream")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
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

	lastID := 1
	lastEventID := r.Header.Get("Last-Event-ID")
	if lastEventID != "" {
		var err error
		lastID, err = strconv.Atoi(lastEventID)
		if err != nil {
			logger.Error("Last-Event-ID header is not parsable")
		}
	}

	filter := r.Form["filter"]
	h.cleanFilter(identifier, filter)

	streamer, err := h.streamer.NewStream(r.Context(), logger, identifier)
	if err != nil {
		logger.WithError(err).Error("Could not start a new stream")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.NotFound(w, r)
		return
	}
	logger.Info("Client connected to SSE")

	receiver := make(chan events.Event) // Channel is closed in Subscriber
	go h.subscribe(r.Context(), logger, streamer, receiver, lastID, filter)

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
