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
package suite

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/qri-io/jsonschema"
	"github.com/sirupsen/logrus"
)

type SuiteValidator struct {
	schema *jsonschema.Schema
}

type Suite struct {
	TERCC  []byte
	schema *jsonschema.Schema
	Code   int
}

// schema is the JSONSchema that is being embedded when we are building the binary
//go:embed TERCCSchema.json
var schema embed.FS

func New() (*SuiteValidator, error) {
	var suiteValidator *SuiteValidator
	schemaFile, err := schema.Open("TERCCSchema.json")
	if err != nil {
		return suiteValidator, fmt.Errorf("cannot open the TERCCSchema.json file %+v", err)
	}
	defer schemaFile.Close()
	schemaData, err := ioutil.ReadAll(schemaFile)
	if err != nil {
		return suiteValidator, fmt.Errorf("cannot read the TERCCSchema.json file %+v", err)
	}
	schema := &jsonschema.Schema{}
	if err = json.Unmarshal(schemaData, schema); err != nil {
		return suiteValidator, fmt.Errorf("TERCCSchema.json file is not a valid JSON %+v", err)
	}
	suiteValidator = &SuiteValidator{
		schema: schema,
	}
	return suiteValidator, nil
}

// Download a test suite.
func (s *SuiteValidator) Download(ctx context.Context, logger *logrus.Entry, url string) (Suite, error) {
	client := http.Client{}
	logger.Infof("starting a GET request to %s", url)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		logger.Errorf("building http request failed: %v", err)
		return Suite{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		logger.Errorf("http request failed: %v", err)
		return Suite{}, err
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	switch status := response.StatusCode; {
	case status >= 400 && status < 500:
		return Suite{Code: status}, fmt.Errorf("failed to download test suite due to client error")
	case status >= 500:
		return Suite{Code: status}, fmt.Errorf("failed to download test suite due to server error")
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		logger.Errorf("failed to read http response: %v", err)
		return Suite{Code: response.StatusCode}, err
	}
	return Suite{
		TERCC:  body,
		schema: s.schema,
		Code:   response.StatusCode,
	}, nil
}

// Validate a downloaded suite against the JSON schema.
func (s *Suite) Validate(ctx context.Context, logger *logrus.Entry) error {
	logger.Info("validating test suite")
	errs, err := s.schema.ValidateBytes(ctx, s.TERCC)
	if err != nil {
		logger.Errorf("test suite invalid: %v", err)
		return err
	}
	var validateErrors []string
	for _, e := range errs {
		logger.Error(e.Error())
		validateErrors = append(validateErrors, e.Error())
	}
	if len(validateErrors) > 0 {
		logger.Error("test suite invalid")
		return errors.New(strings.Join(validateErrors, "\n"))
	}
	logger.Info("test suite is valid")
	return nil
}
