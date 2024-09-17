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
package executionspace

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type Data struct {
	ID uuid.UUID `json:"id"`
}

type Request struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Data    Data              `json:"json"`
	Headers map[string]string `json:"headers"`
	Timeout int               `json:"timeout"`
}

type Instructions struct {
	Image       string            `json:"image"`
	Environment map[string]string `json:"environment"`
	Parameters  map[string]string `json:"parameters"`
	Identifier  uuid.UUID         `json:"identifier"`
}

type ExecutorSpec struct {
	Request      Request      `json:"request"`
	Instructions Instructions `json:"instructions"`
	ID           uuid.UUID    `json:"id"`
	BuildID      string
}

// NewExecutorSpec creates a new ExecutorSpec
func NewExecutorSpec(url string, etosIdentifier string, testRunner string, environment map[string]string, otelCtx context.Context) ExecutorSpec {
	id := uuid.New()

	headers := make(map[string]string)
	headers["X-Etos-id"] = etosIdentifier

	carrier := propagation.HeaderCarrier(make(map[string][]string))
	propagators := otel.GetTextMapPropagator()
	propagators.Inject(otelCtx, carrier)

	for key, values := range carrier {
		headers[key] = strings.Join(values, ",")
	}

	e := ExecutorSpec{
		Request: Request{
			URL:     url,
			Method:  "POST",
			Timeout: 7200, // 2 hours
			Data: Data{
				ID: id,
			},
			Headers: headers,
		},
		Instructions: Instructions{
			Environment: environment,
			Image:       testRunner,
			Parameters:  map[string]string{},
			Identifier:  uuid.New(),
		},
		ID: id,
	}
	e.Instructions.Environment["ENVIRONMENT_ID"] = id.String()
	if v := os.Getenv("EXECUTOR_HTTPS_PROXY"); v != "" {
		e.Instructions.Environment["HTTPS_PROXY"] = v
		e.Instructions.Environment["https_proxy"] = v
	}
	if v := os.Getenv("EXECUTOR_HTTP_PROXY"); v != "" {
		e.Instructions.Environment["HTTP_PROXY"] = v
		e.Instructions.Environment["http_proxy"] = v
	}
	if v := os.Getenv("EXECUTOR_NO_PROXY"); v != "" {
		e.Instructions.Environment["NO_PROXY"] = v
		e.Instructions.Environment["no_proxy"] = v
	}
	if v := os.Getenv("EXECUTOR_TZ"); v != "" {
		e.Instructions.Environment["TZ"] = v
	}
	return e
}

// LoadExecutorSpec loads an ExecutorSpec from an io Reader
func LoadExecutorSpec(r io.Reader) (*ExecutorSpec, error) {
	executor := &ExecutorSpec{}
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(executor); err != nil {
		return nil, err
	}
	return executor, nil
}

// LoadExecutorSpecs loads multiple executors from a single io Reader
func LoadExecutorSpecs(r io.Reader) ([]ExecutorSpec, error) {
	var executors []ExecutorSpec
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&executors); err != nil {
		return nil, err
	}
	return executors, nil
}

// Save saves an execution space to an io Writer
func (e ExecutorSpec) Save(w io.Writer) error {
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(e); err != nil {
		return errors.Join(errors.New("failed to write executor to database"), err)
	}
	return nil
}

// Delete deletes an executor from an io Writer
func (e ExecutorSpec) Delete(w io.Writer) error {
	_, err := w.Write(nil)
	if err != nil {
		return err
	}
	return nil
}
