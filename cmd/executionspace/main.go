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
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	config "github.com/eiffel-community/etos-api/internal/configs/executionspace"
	"github.com/eiffel-community/etos-api/internal/database/etcd"
	"github.com/eiffel-community/etos-api/internal/executionspace/provider"
	"github.com/eiffel-community/etos-api/internal/logging"
	"github.com/eiffel-community/etos-api/internal/logging/rabbitmqhook"
	"github.com/eiffel-community/etos-api/internal/rabbitmq"
	"github.com/eiffel-community/etos-api/internal/server"
	"github.com/eiffel-community/etos-api/pkg/application"
	providerservice "github.com/eiffel-community/etos-api/pkg/executionspace/v1alpha"
	"github.com/sirupsen/logrus"
	"github.com/snowzach/rotatefilehook"
	"go.elastic.co/ecslogrus"
)

// main sets up logging and starts up the webservice.
func main() {
	cfg := config.Get()
	ctx := context.Background()

	var hooks []logrus.Hook
	if publisher := remoteLogging(cfg); publisher != nil {
		defer publisher.Close()
		hooks = append(hooks, rabbitmqhook.NewRabbitMQHook(publisher))
	}
	if fileHook := fileLogging(cfg); fileHook != nil {
		hooks = append(hooks, fileHook)
	}

	logger, err := logging.Setup(cfg.LogLevel(), hooks)
	if err != nil {
		logrus.Fatal(err.Error())
	}

	hostname, err := os.Hostname()
	if err != nil {
		logrus.Fatal(err.Error())
	}
	log := logger.WithFields(logrus.Fields{
		"hostname":    hostname,
		"application": "ETOS Execution Space Provider Kubernetes",
		"version":     vcsRevision(),
		"name":        "ETOS Execution Space Provider",
		"user_log":    false,
	})

	log.Info("Loading v1alpha routes")
	provider := provider.Kubernetes{}.New(etcd.New(cfg, logger), cfg)
	providerServiceApp := providerservice.New(cfg, log, provider, ctx)
	defer providerServiceApp.Close()
	handler := application.New(providerServiceApp)

	srv := server.NewWebService(cfg, log, handler)

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Errorf("WebService shutdown: %+v", err)
		}
	}()

	sig := <-done
	log.Infof("%s received", sig.String())

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout())
	defer cancel()

	if err := srv.Close(ctx); err != nil {
		log.Errorf("WebService shutdown failed: %+v", err)
	}
	log.Info("Wait for checkout and checkin jobs to complete")
}

// fileLogging adds a hook into a slice of hooks, if the filepath configuration is set
func fileLogging(cfg config.Config) logrus.Hook {
	if filePath := cfg.LogFilePath(); filePath != "" {
		// TODO: Make these parameters configurable.
		// NewRotateFileHook cannot return an error which is why it's set to '_'.
		rotateFileHook, _ := rotatefilehook.NewRotateFileHook(rotatefilehook.RotateFileConfig{
			Filename:   filePath,
			MaxSize:    10, // megabytes
			MaxBackups: 3,
			MaxAge:     0, // days
			Level:      logrus.DebugLevel,
			Formatter: &ecslogrus.Formatter{
				DataKey: "labels",
			},
		})
		return rotateFileHook
	}
	return nil
}

// remoteLogging starts a new rabbitmq publisher if the rabbitmq parameters are set
// Warning: Must call publisher.Close() on the publisher returned from this function
func remoteLogging(cfg config.Config) *rabbitmq.Publisher {
	if cfg.RabbitMQHookURL() != "" {
		if cfg.RabbitMQHookExchangeName() == "" {
			panic("-rabbitmq_hook_exchange (env:ETOS_RABBITMQ_EXCHANGE) must be set when using -rabbitmq_hook_url (env:ETOS_RABBITMQ_URL)")
		}
		publisher := rabbitmq.NewPublisher(rabbitmq.PublisherConfig{
			URL:          cfg.RabbitMQHookURL(),
			ExchangeName: cfg.RabbitMQHookExchangeName(),
		})
		return publisher
	}
	return nil
}

// vcsRevision returns the current source code revision
func vcsRevision() string {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return "(unknown)"
	}
	for _, val := range buildInfo.Settings {
		if val.Key == "vcs.revision" {
			return val.Value
		}
	}
	return "(unknown)"
}
