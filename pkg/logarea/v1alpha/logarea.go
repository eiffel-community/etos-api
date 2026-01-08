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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/eiffel-community/etos-api/pkg/application"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
)

type LogAreaApplication struct {
	logger *logrus.Entry
	cfg    config.LogAreaConfig
}

type LogAreaHandler struct {
	logger *logrus.Entry
	cfg    config.LogAreaConfig
}

// Close does nothing
func (a *LogAreaApplication) Close() {
}

// New returns a new LogAreaApplication object/struct
func New(cfg config.LogAreaConfig, log *logrus.Entry) application.Application {
	return &LogAreaApplication{
		logger: log,
		cfg:    cfg,
	}
}

// LoadRoutes loads all the v1alpha routes.
func (a LogAreaApplication) LoadRoutes(router *httprouter.Router) {
	handler := &LogAreaHandler{a.logger, a.cfg}
	if err := os.MkdirAll(a.cfg.UploadPath(), os.ModePerm); err != nil {
		panic(err)
	}

	router.GET("/logarea/v1alpha/selftest/ping", handler.Selftest)
	router.POST("/logarea/upload", handler.Upload)
	router.GET("/logarea/upload", handler.Download)
}

// Selftest is a handler to just return 204.
func (h LogAreaHandler) Selftest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	w.WriteHeader(http.StatusNoContent)
}

// Download is an HTTP Get endpoint for downloading uploaded files.
func (h *LogAreaHandler) Download(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	h.logger.Debug("Download request received")
	q := r.URL.Query()
	path := q.Get("path")
	if path == "" {
		h.logger.Error("Path query is missing in request. Cancel request.")
		respondWithError(w, http.StatusBadRequest, "missing path query parameter")
		return
	}

	h.logger.Debugf("Build path to file %q.", path)
	path = strings.Join([]string{h.cfg.UploadPath(), path}, "/")
	splitPath := strings.Split(path, "/")
	filename := splitPath[len(splitPath)-1]
	h.logger.Debugf("Finalized path: %q.", path)
	h.logger.Debugf("Finalized filename: %q.", filename)

	if _, err := os.Stat(path); err == nil {
		// File exists.
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
		w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
		h.logger.Debugf("Serving file")
		http.ServeFile(w, r, path)
	} else if os.IsNotExist(err) {
		// File does not exist.
		h.logger.Error(err)
		respondWithError(w, http.StatusNotFound, "file not found")
		return
	} else {
		// Unknown error.
		h.logger.Errorf("Unknown error: %r", err)
		respondWithError(w, http.StatusInternalServerError, "unknown error, please contact maintainers")
		return
	}
}

// Upload is an HTTP Post endpoint for uploading files.
func (h *LogAreaHandler) Upload(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	h.logger.Debug("Upload request received")
	q := r.URL.Query()
	path := q.Get("path")
	if path == "" {
		h.logger.Error("Path query is missing in request. Cancel request.")
		respondWithError(w, http.StatusBadRequest, "missing path query parameter")
		return
	}

	h.logger.Debugf("Build path to file %q.", path)
	path = strings.Join([]string{h.cfg.UploadPath(), path}, "/")
	directories := strings.Split(path, "/")
	directory := strings.Join(directories[0:len(directories)-1], "/")
	h.logger.Debugf("Finalized path: %q.", path)
	if err := os.MkdirAll(directory, os.ModePerm); err != nil {
		h.logger.Errorf("could not create the log directory %q", err.Error())
		respondWithError(w, http.StatusInternalServerError, "could not create the log directory")
		return
	}
	file, err := os.Create(path)
	if err != nil {
		h.logger.Errorf("could not create the file %q", err.Error())
		respondWithError(w, http.StatusInternalServerError, "could not create file on server")
		return
	}
	n, err := io.Copy(file, r.Body)
	if err != nil {
		h.logger.Errorf("could not create the file %q", err.Error())
		respondWithError(w, http.StatusInternalServerError, "could not create file on server")
		return
	}
	h.logger.Debug("File successfully uploaded")
	respondWithJSON(w, http.StatusOK, map[string]string{"bytes": fmt.Sprintf("%d", n)})
}

// respondWithJSON writes a JSON response with a status code to the HTTP ResponseWriter.
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(response)
}

// respondWithError writes a JSON response with an error message and status code to the HTTP ResponseWriter.
func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}
