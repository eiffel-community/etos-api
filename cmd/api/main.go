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
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/eiffel-community/etos-api/internal/logging"
	"github.com/eiffel-community/etos-api/internal/server"
	"github.com/eiffel-community/etos-api/pkg/application"
	"github.com/eiffel-community/etos-api/pkg/v1alpha1"
	"github.com/sirupsen/logrus"
)

// GitSummary contains "git describe" output and is automatically
// populated via linker options when building with govvv.
var GitSummary = "(unknown)"

// main sets up logging and starts up the webserver.
func main() {
	cfg := config.Get()
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
	if err := validateInput(cfg); err != nil {
		log.Panic(err)
	}

	log.Info("Loading v1alpha1 routes")
	v1alpha1App := v1alpha1.New(cfg, log, ctx)
	defer v1alpha1App.Close()
	handler := application.New(v1alpha1App)

	srv := server.NewWebserver(cfg, log, handler)

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Errorf("Webserver shutdown: %+v", err)
		}
	}()

	<-done

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout())
	defer cancel()

	if err := srv.Close(ctx); err != nil {
		log.Errorf("Webserver shutdown failed: %+v", err)
	}
}

// validateInput checks that all required input parameters that do not have sensible
// defaults are actually set.
func validateInput(cfg config.Config) error {
	if cfg.EventRepositoryHost() == "" {
		return errors.New("-eventrepository input or 'ETOS_GRAPHQL_SERVER' environment variable must be set")
	}
	return nil
}
