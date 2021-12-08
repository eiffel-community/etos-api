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
package v1alpha1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/eiffel-community/eiffelevents-sdk-go"
	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/eiffel-community/etos-api/internal/responses"
	"github.com/eiffel-community/etos-api/pkg/application"
	"github.com/eiffel-community/etos-api/pkg/v1alpha1/suite"

	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
)

type V1Alpha1Application struct {
	logger    *logrus.Entry
	cfg       config.Config
	validator *suite.SuiteValidator
}

type V1Alpha1Handler struct {
	logger    *logrus.Entry
	cfg       config.Config
	validator *suite.SuiteValidator
}

func New(cfg config.Config, log *logrus.Entry, ctx context.Context) application.Application {
	validator, err := suite.New()
	if err != nil {
		log.Panic(err)
	}
	return &V1Alpha1Application{
		logger:    log,
		cfg:       cfg,
		validator: validator,
	}
}

// LoadRoutes loads all the v1alpha1 routes.
func (a *V1Alpha1Application) LoadRoutes(router *httprouter.Router) {
	handler := &V1Alpha1Handler{a.logger, a.cfg, a.validator}
	router.GET("/v1alpha1/selftest/ping", handler.Selftest)
	router.POST("/v1alpha1/etos", handler.timeoutHandler(handler.identifierHandler(handler.requestTimeHandler(handler.StartETOS))))
}

// Selftest is a handler to just return 204.
func (h *V1Alpha1Handler) Selftest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	responses.RespondWithError(w, http.StatusNoContent, "")
}

// StartRequest is the request to Start ETOS.
type StartRequest struct {
	ArtifactIdentity       string                 `json:"artifact_identity,omitempty"`
	ArtifactID             string                 `json:"artifact_id,omitempty"`
	TestSuiteURL           string                 `json:"test_suite_url"`
	Dataset                map[string]interface{} `json:"dataset"`
	ExecutionSpaceProvider string                 `json:"execution_space_provider,omitempty"`
	LogAreaProvider        string                 `json:"log_area_provider,omitempty"`
	IUTProvider            string                 `json:"iut_provider,omitempty"`
	Artifact               eiffelevents.ArtifactCreatedV3
}

type StartResponse struct {
	EventRepository  string `json:"event_repository"`
	TERCC            string `json:"tercc"`
	ArtifactID       string `json:"artifact_id"`
	ArtifactIdentity string `json:"artifact_identity"`
}

// StartETOS checks if the artifact IUT exists, the test suite validates, configures the
// environment provider and sends a TERCC event, triggering ETOS.
func (h *V1Alpha1Handler) StartETOS(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	identifier := ps.ByName("identifier")
	logger := h.logger.WithField("identifier", identifier)
	var response StartResponse
	ctx := r.Context()

	request, err := h.verifyInput(ctx, logger, r)
	if err != nil {
		logger.Error(err.Error())
		sendError(w, err)
		return
	}

	response = StartResponse{
		EventRepository:  h.cfg.EventRepositoryHost(),
		TERCC:            identifier,
		ArtifactID:       request.Artifact.Meta.ID,
		ArtifactIdentity: request.Artifact.Data.Identity,
	}

	responses.RespondWithJSON(w, http.StatusOK, response)
}

// verifyInput verifies that the required request parameters exist. It verifies that the
// IUT artifact exists and validates that the test suite works with ETOS.
func (h V1Alpha1Handler) verifyInput(ctx context.Context, logger *logrus.Entry, r *http.Request) (StartRequest, error) {
	request := StartRequest{
		ExecutionSpaceProvider: config.EnvOrDefault("DEFAULT_EXECUTION_SPACE_PROVIDER", "default"),
		LogAreaProvider:        config.EnvOrDefault("DEFAULT_LOG_AREA_PROVIDER", "default"),
		IUTProvider:            config.EnvOrDefault("DEFAULT_IUT_PROVIDER", "default"),
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return request, NewHTTPError(fmt.Errorf("unable to decode post body %+v", err), http.StatusBadRequest)
	}
	if request.ArtifactID == "" && request.ArtifactIdentity == "" {
		return request, NewHTTPError(errors.New("at least one of 'artifact_identity' or 'artifact_id' is required"), http.StatusBadRequest)
	}
	if request.ArtifactID != "" && request.ArtifactIdentity != "" {
		return request, NewHTTPError(errors.New("only one of 'artifact_identity' or 'artifact_id' is required"), http.StatusBadRequest)
	}

	artifactCh := make(chan ArtifactResult, 1)
	suiteCh := make(chan error, 1)
	go waitForArtifact(ctx, h.cfg, logger, request, artifactCh)
	go waitForSuiteValidation(ctx, logger, h.validator, request.TestSuiteURL, suiteCh)

	result := <-artifactCh
	if result.err != nil {
		return request, result.err
	}
	request.Artifact = result.Artifact
	err := <-suiteCh
	if err != nil {
		return request, err
	}
	return request, nil
}

// sendError sends an error HTTP response depending on which error has been returned.
func sendError(w http.ResponseWriter, err error) {
	httpError, ok := err.(*HTTPError)
	if !ok {
		responses.RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("unknown error %+v", err))
	} else {
		responses.RespondWithError(w, httpError.Code, httpError.Message)
	}
}

// requestTimeHandler will log the time each request took.
func (h *V1Alpha1Handler) requestTimeHandler(fn func(http.ResponseWriter, *http.Request, httprouter.Params)) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		start := time.Now()
		h.logger.WithField("identifier", ps.ByName("identifier")).Infof("%s: %s", r.Method, r.URL.Path)
		fn(w, r, ps)
		h.logger.WithField("identifier", ps.ByName("identifier")).Infof("Request completed in %v (%s %s)", time.Since(start), r.Method, r.URL.Path)
	}
}

// identifierHandler will generate an identifier and attach to each request.
func (h *V1Alpha1Handler) identifierHandler(fn func(http.ResponseWriter, *http.Request, httprouter.Params)) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ps = append(ps, httprouter.Param{Key: "identifier", Value: uuid.NewString()})
		fn(w, r, ps)
	}
}

// timeoutHandler will change the request context to a timeout context.
func (h *V1Alpha1Handler) timeoutHandler(fn func(http.ResponseWriter, *http.Request, httprouter.Params)) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ctx, cancel := context.WithTimeout(r.Context(), h.cfg.Timeout())
		defer cancel()
		newRequest := r.WithContext(ctx)
		fn(w, newRequest, ps)
	}
}
