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
package logarea

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/eiffel-community/etos-api/internal/config"
	log "github.com/sirupsen/logrus"
)

const interval = time.Second * 60

type RetentionHandler struct {
	Logger *log.Entry
	config config.LogAreaConfig
	quit   chan bool
}

// NewRetentionHandler creates a new retention handler.
func NewRetentionHandler(ctx context.Context, cfg config.LogAreaConfig, logger *log.Entry) *RetentionHandler {
	return &RetentionHandler{
		Logger: logger,
		config: cfg,
		quit:   make(chan bool),
	}
}

// cleanup all files that are older than the configured time.
// note that this function does not traverse all files in a directory
// but instead looks at the top files and directories and deletes them.
func (r *RetentionHandler) cleanup() {
	r.Logger.Debug("Starting cleanup job")
	tmpfiles, err := os.ReadDir(r.config.UploadPath())
	if err != nil {
		r.Logger.Fatal(err)
	}
	var directories []os.FileInfo
	for _, directory := range tmpfiles {
		info, err := directory.Info()
		if err != nil {
			r.Logger.Error(err)
			continue
		}
		if time.Since(info.ModTime()) > r.config.Retention() {
			directories = append(directories, info)
		}
	}
	for _, directory := range directories {
		r.Logger.Debugf("Removing directory %q", directory.Name())
		path := strings.Join([]string{r.config.UploadPath(), directory.Name()}, "/")
		if err := os.RemoveAll(path); err != nil {
			r.Logger.Error(err)
		}
	}
	if len(directories) > 0 {
		r.Logger.Debug("Cleanup job finished")
	} else {
		r.Logger.Debug("Nothing to clean up")
	}
}

// run a select loop with a timer attached which will call the cleanup
// function every X seconds until a quit signal is detected.
func (r *RetentionHandler) run() {
	// Do a cleanup just after the application has started.
	t := time.NewTimer(time.Second * 1)
	for {
		select {
		case <-r.quit:
			r.Logger.Debug("Shutting down file retention handler.")
			return
		case <-t.C:
			r.cleanup()
			r.Logger.Debugf("Next cleanup in %s", interval)
			t = time.NewTimer(interval)
		}
	}
}

// Start up the retention handler. Non-blocking.
func (r *RetentionHandler) Start() {
	r.Logger.Debug("Starting up retention handler.")
	r.Logger.Debugf("Retention set to %s", r.config.Retention())
	if r.config.Retention() < time.Minute*1 {
		r.Logger.Warning("Retention set to below 1m with default cleanup interval at 1m")
	}
	go r.run()
}

// Shutdown the retention handler.
func (r *RetentionHandler) Shutdown() {
	r.quit <- true
}
