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

// package sse for v1alpha uses the old SSE implemenation from the ESR log listener
// and the old version of the ETOS client.
package sse

import (
	"bufio"
	"context"
	"errors"
	"fmt"
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

// LoadRoutes loads all the v1alpha routes.
func (a SSEApplication) LoadRoutes(router *httprouter.Router) {
	kube := kubernetes.New(a.cfg, a.logger)
	handler := &SSEHandler{a.logger, a.cfg, a.ctx, kube}
	router.GET("/v1alpha/selftest/ping", handler.Selftest)
	router.GET("/v1alpha/logs/:identifier", handler.GetEvents)
}

// Selftest is a handler to just return 204.
func (h SSEHandler) Selftest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	responses.RespondWithError(w, http.StatusNoContent, "")
}

// Subscribe subscribes to an ETOS suite runner instance and gets logs and events from it and
// writes them to a channel.
func (h SSEHandler) Subscribe(ch chan<- events.Event, ctx context.Context, counter int, identifier string, url string) {
	defer close(ch)

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
				ch <- events.Event{Event: "shutdown"}
				return
			}
			messages, err := GetFrom(ctx, url)
			if err != nil {
				h.logger.Warning(err.Error())
				continue
			}
			// SSE starts ad ID 1, but slices start with index 0, so we do -1 here.
			if len(messages) >= counter-1 {
				for _, message := range messages[counter-1:] {
					event := events.Event{
						Event: "message",
						ID:    counter,
						Data:  message,
					}
					ch <- event
					counter++
				}
			}
		}
	}
}

// GetFrom gets all events from an ESR instance
func GetFrom(ctx context.Context, url string) ([]string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}

	var messages []string
	scanner := bufio.NewScanner(response.Body)
	defer response.Body.Close()
	for scanner.Scan() {
		messages = append(messages, scanner.Text())
	}
	return messages, nil
}

// url finds the url of an ESR instance.
func (h SSEHandler) url(ctx context.Context, identifier string) (string, error) {
	ip, err := h.kube.LogListenerIP(ctx, identifier)
	if err != nil {
		return "", err
	}
	if ip == "" {
		return "", errors.New("No IP from ESR yet")
	}
	return fmt.Sprintf("http://%s:8000/log", ip), nil
}

// forceKillConnection hijacks the underlying TCP connection between the client and server
// and stops it forcefully. This will cause a panic in the goroutine running the connection
// but this is the only way for us to be compatible with the old ETOS SSE implementation.
func forceKillConnection(w http.ResponseWriter) {
	hijack, ok := w.(http.Hijacker)
	if !ok {
		responses.RespondWithError(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}
	connection, _, err := hijack.Hijack()
	if err != nil {
		responses.RespondWithError(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
		return
	}
	connection.Close()
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

	last_id := 1
	lastEventID := r.Header.Get("Last-Event-ID")
	if lastEventID != "" {
		var err error
		last_id, err = strconv.Atoi(lastEventID)
		if err != nil {
			h.logger.Error("Last-Event-ID header is not parsable")
			responses.RespondWithError(w, http.StatusBadRequest, http.StatusText(http.StatusBadRequest))
			return
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		responses.RespondWithError(w, http.StatusNotFound, http.StatusText(http.StatusNotFound))
		return
	}
	h.logger.Info("Client connected to SSE")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Transfer-Encoding", "chunked")

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
			if event.Event == "shutdown" {
				forceKillConnection(w)
				break
			}
			if err := event.Write(w); err != nil {
				h.logger.Error(err)
				continue
			}
			flusher.Flush()
		}
	}
}
