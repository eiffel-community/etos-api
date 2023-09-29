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
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/eiffel-community/etos-api/internal/kubernetes"
	"github.com/eiffel-community/etos-api/internal/responses"
	"github.com/eiffel-community/etos-api/pkg/application"
	"github.com/eiffel-community/etos-api/pkg/events"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
)

type SSEApplication struct {
	logger *logrus.Entry
	cfg    config.Config
	ctx    context.Context
	cancel context.CancelFunc
}

type SSEHandler struct {
	logger *logrus.Entry
	cfg    config.Config
	ctx    context.Context
	kube   *kubernetes.Kubernetes
}

// Close cancels the application context
func (a *SSEApplication) Close() {
	a.cancel()
}

// New returns a new SSEApplication object/struct
func New(cfg config.Config, log *logrus.Entry, ctx context.Context) application.Application {
	ctx, cancel := context.WithCancel(ctx)
	return &SSEApplication{
		logger: log,
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

// LoadRoutes loads all the v1 routes.
func (a SSEApplication) LoadRoutes(router *httprouter.Router) {
	kube := kubernetes.New(a.cfg, a.logger)
	handler := &SSEHandler{a.logger, a.cfg, a.ctx, kube}
	router.GET("/v1/selftest/ping", handler.Selftest)
	router.GET("/v1/events/:identifier", handler.GetEvents)
	router.GET("/v1/event/:identifier/:id", handler.GetEvent)
}

// Selftest is a handler to just return 204.
func (h SSEHandler) Selftest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	responses.RespondWithError(w, http.StatusNoContent, "")
}

// Subscribe subscribes to an ETOS suite runner instance and gets logs and events from it and
// writes them to a channel.
func (h SSEHandler) Subscribe(ch chan<- events.Event, ctx context.Context, counter int, identifier string, url string) {
	defer close(ch)

	// TODO: Test a streaming approach.
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Client lost, closing subscriber")
			return
		case <-ping.C:
			ch <- events.Event{Event: "ping"}
		case <-tick.C:
			if h.kube.IsFinished(ctx, identifier) {
				h.logger.Info("ESR finished, shutting down")
				// If this is not sent to the client, then the client will reconnect and
				// the message will be received next time.
				ch <- events.Event{Event: "shutdown", Data: "ESR finished, shutting down"}
				// We expect the client to close the connection, as such we continue here
				// instead of ending the subscriber.
				continue
			}
			events, err := GetFrom(ctx, url, fmt.Sprint(counter))
			if err != nil {
				h.logger.Warning(err.Error())
				continue
			}
			for _, event := range events {
				ch <- event
				counter++
			}
		}
	}
}

// GetFrom gets all events from an ESR instance starting from id.
func GetFrom(ctx context.Context, url string, id string) ([]events.Event, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	query := request.URL.Query()
	query.Add("start", id)
	request.URL.RawQuery = query.Encode()

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}

	etosEvents := []events.Event{}
	scanner := bufio.NewScanner(response.Body)
	defer response.Body.Close()
	for scanner.Scan() {
		event, err := events.New(scanner.Bytes())
		if err != nil {
			// TODO: Log it?
			continue
		}
		etosEvents = append(etosEvents, event)
	}
	return etosEvents, nil
}

// GetOne gets a single event from an ESR instance.
func GetOne(ctx context.Context, url string, id string) (events.Event, error) {
	event := events.Event{}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return event, err
	}
	query := request.URL.Query()
	query.Add("start", id)
	query.Add("end", id)
	request.URL.RawQuery = query.Encode()

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return event, err
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return event, err
	}
	defer response.Body.Close()
	return events.New(body)
}

// url find the url of an ESR instance.
func (h SSEHandler) url(ctx context.Context, identifier string) (string, error) {
	url, err := h.kube.LogListenerIP(ctx, identifier)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://%s:8000/log", url), nil
}

// GetEvent is an endpoint for getting a single event from an ESR instance.
func (h SSEHandler) GetEvent(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	identifier := ps.ByName("identifier")
	counter := ps.ByName("id")
	if h.kube.IsFinished(r.Context(), identifier) {
		responses.RespondWithError(w, http.StatusNotFound, http.StatusText(http.StatusNotFound))
		return
	}
	url, err := h.url(r.Context(), identifier)
	if err != nil {
		h.logger.Error(err)
		responses.RespondWithError(w, http.StatusNotFound, http.StatusText(http.StatusNotFound))
		return
	}

	event, err := GetOne(r.Context(), url, counter)
	if err != nil {
		h.logger.Error(err)
		// TODO: Message client
		return
	}
	if err := event.Write(w); err != nil {
		h.logger.Error(err)
	}
}

// GetEvents is an endpoint for streaming events and logs from an ESR instance.
func (h SSEHandler) GetEvents(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	identifier := ps.ByName("identifier")
	if h.kube.IsFinished(r.Context(), identifier) {
		responses.RespondWithError(w, http.StatusNotFound, http.StatusText(http.StatusNotFound))
		return
	}
	url, err := h.url(r.Context(), identifier)
	if err != nil {
		responses.RespondWithError(w, http.StatusNotFound, http.StatusText(http.StatusNotFound))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	last_id := 1
	lastEventID := r.Header.Get("Last-Event-ID")
	if lastEventID != "" {
		var err error
		last_id, err = strconv.Atoi(lastEventID)
		if err != nil {
			h.logger.Error("Last-Event-ID header is not parsable")
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		responses.RespondWithError(w, 404, "Not Found")
		return
	}

	h.logger.Info("Client connected to SSE")

	receiver := make(chan events.Event) // Channel is closed in Subscriber
	go h.Subscribe(receiver, r.Context(), last_id, identifier, url)

	for {
		select {
		case <-r.Context().Done():
			h.logger.Info("Client gone from SSE")
			return
		case <-h.ctx.Done():
			h.logger.Info("Shutting down")
			return
		case event := <-receiver:
			if err := event.Write(w); err != nil {
				h.logger.Error(err)
				continue
			}
			flusher.Flush()
		}
	}
}
