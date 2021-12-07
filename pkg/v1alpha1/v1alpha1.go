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
	"net/http"

	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/eiffel-community/etos-api/internal/responses"
	"github.com/eiffel-community/etos-api/pkg/application"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
)

type V1Alpha1Application struct {
	logger *logrus.Entry
	cfg    config.Config
}

type V1Alpha1Handler struct {
	logger *logrus.Entry
	cfg    config.Config
}

func New(cfg config.Config, log *logrus.Entry, ctx context.Context) application.Application {
	return &V1Alpha1Application{
		logger: log,
		cfg:    cfg,
	}
}

// LoadRoutes loads all the v1alpha1 routes.
func (a *V1Alpha1Application) LoadRoutes(router *httprouter.Router) {
	handler := &V1Alpha1Handler{a.logger, a.cfg}
	router.GET("/v1alpha1/selftest/ping", handler.Selftest)
}

// Selftest is a handler to just return 204.
func (h *V1Alpha1Handler) Selftest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	responses.RespondWithError(w, http.StatusNoContent, "")
}
