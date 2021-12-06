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
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/eiffel-community/etos-api/internal/logging"
	"github.com/eiffel-community/etos-api/internal/server"
	"github.com/sirupsen/logrus"

	"github.com/julienschmidt/httprouter"
)

// GitSummary contains "git describe" output and is automatically
// populated via linker options when building with govvv.
var GitSummary = "(unknown)"

// main sets up logging and starts up the webserver.
func main() {
	cfg := config.Get()
	handler := httprouter.New()
	ctx := context.Background()

	logger, err := logging.Setup(cfg)
	if err != nil {
		logrus.Fatal(err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		logrus.Fatal(err)
	}
	log := logger.WithFields(logrus.Fields{
		"hostname":    hostname,
		"application": "etos-api",
		"version":     GitSummary,
	})

	srv := server.NewWebserver(cfg, log, handler)

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Errorf("Webserver shutdown: %+v", err)
		}
	}()

	<-done
	// TODO: This timeout shall be the same as the request timeout when that
	// gets implemented.
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	if err := srv.Close(ctx); err != nil {
		log.Errorf("Webserver shutdown failed: %+v", err)
	}
}
