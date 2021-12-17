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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/sethvargo/go-retry"
	"github.com/sirupsen/logrus"
)

type EnvironmentProvider struct {
	host   string
	logger *logrus.Entry
}

// NewEnvironmentProvider creates a new environment provider struct with host an logger.
func NewEnvironmentProvider(logger *logrus.Entry, host string) *EnvironmentProvider {
	return &EnvironmentProvider{
		host:   host,
		logger: logger,
	}
}

type EnvironmentProviderConfiguration struct {
	SuiteID                string                 `json:"suite_id"`
	Dataset                map[string]interface{} `json:"dataset"`
	ExecutionSpaceProvider string                 `json:"execution_space_provider"`
	IUTProvider            string                 `json:"iut_provider"`
	LogAreaProvider        string                 `json:"log_area_provider"`
}

type EnvironmentProviderResponse struct {
	Dataset                map[string]interface{} `json:"dataset"`
	ExecutionSpaceProvider map[string]interface{} `json:"execution_space_provider"`
	IUTProvider            map[string]interface{} `json:"iut_provider"`
	LogAreaProvider        map[string]interface{} `json:"log_area_provider"`
}

// Configure posts a configuration to the ETOS environment provider.
func (e *EnvironmentProvider) Configure(ctx context.Context, configuration EnvironmentProviderConfiguration) error {
	e.logger.Infof("configuring environment provider (%s) with suite ID %s", e.host, configuration.SuiteID)
	url := strings.Join([]string{e.host, "configure"}, "/")
	client := http.Client{}
	body, err := json.Marshal(configuration)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		e.logger.Errorf("building http request failed: %v", err)
		return err
	}
	request.Header.Add("Accept", "application/json")
	request.Header.Add("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		e.logger.Errorf("http request failed: %v", err)
		return err
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	if response.StatusCode != http.StatusOK {
		return NewHTTPError(fmt.Errorf("error when configuring environment (%s)", response.Status), response.StatusCode)
	}
	e.logger.Info(url)
	e.logger.Info("environment provider configured")
	return nil
}

// Verify will request the environment provider to verify that the configuration has been properly added.
func (e *EnvironmentProvider) Verify(ctx context.Context, configuration EnvironmentProviderConfiguration) error {
	e.logger.Infof("verify environment provider configuration with suite ID %s", configuration.SuiteID)
	url := strings.Join([]string{e.host, "configure"}, "/")
	client := http.Client{}
	e.logger.Debugf("starting a GET request to %s", url)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		e.logger.Errorf("building http request failed: %v", err)
		return err
	}
	request.Header.Add("Accept", "application/json")
	request.Header.Add("Content-Type", "application/json")
	q := request.URL.Query()
	q.Add("suite_id", configuration.SuiteID)
	request.URL.RawQuery = q.Encode()
	response, err := client.Do(request)
	if err != nil {
		e.logger.Errorf("http request failed: %v", err)
		return err
	}
	if response.Body != nil {
		defer response.Body.Close()
	}

	if response.StatusCode != http.StatusOK {
		return NewHTTPError(fmt.Errorf("failed to verify the test suite due to HTTP errors (%s)", response.Status), response.StatusCode)
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		e.logger.Errorf("failed to read http response: %v", err)
		return err
	}
	var configuredEnvironment EnvironmentProviderResponse
	err = json.Unmarshal(body, &configuredEnvironment)
	if err != nil {
		e.logger.Error(err)
		return errors.New("failed to parse environment provider configuration as JSON")
	}
	if configuredEnvironment.ExecutionSpaceProvider["id"] != configuration.ExecutionSpaceProvider {
		return errors.New("execution space provider is not configured properly")
	} else if configuredEnvironment.IUTProvider["id"] != configuration.IUTProvider {
		return errors.New("IUT provider is not configured properly")
	} else if configuredEnvironment.LogAreaProvider["id"] != configuration.LogAreaProvider {
		return errors.New("log area provider is not configured properly")
	}
	e.logger.Info("environment provider configuration verified")
	return nil
}

// configureEnvironmentProvider configures the environment provider with the input data from
// the user starting ETOS.
func configureEnvironmentProvider(ctx context.Context, logger *logrus.Entry, environmentProvider *EnvironmentProvider, configuration EnvironmentProviderConfiguration) error {
	err := retry.Fibonacci(ctx, 1*time.Second, func(ctx context.Context) error {
		err := environmentProvider.Configure(ctx, configuration)
		if err == nil {
			return nil
		}
		switch httpError := err.(type) {
		case *HTTPError:
			// Cannot recover from client errors making it unnecessary to retry.
			if httpError.Code >= 400 && httpError.Code < 500 {
				return err
			}
			return retry.RetryableError(err)
		default:
			return retry.RetryableError(err)
		}
	})
	if err != nil {
		logger.Errorf("failed configuring environment provider: %+v", err)
		// Yes, it's correct with double '&' in these two statements.
		// This is because 'As panics if target is not a non-nil pointer to
		// either a type that implements error, or to any interface type'.
		errType := &HTTPError{}
		if errors.As(err, &errType) {
			return err
		}
		return NewHTTPError(err, http.StatusRequestTimeout)
	}
	return nil
}

// verifyEnvironmentProvider verifies that the environment provider configuration is set correctly.
func verifyEnvironmentProvider(ctx context.Context, logger *logrus.Entry, environmentProvider *EnvironmentProvider, configuration EnvironmentProviderConfiguration) error {
	err := retry.Fibonacci(ctx, 1*time.Second, func(ctx context.Context) error {
		err := environmentProvider.Verify(ctx, configuration)
		if err == nil {
			return nil
		}
		switch httpError := err.(type) {
		case *HTTPError:
			// Cannot recover from client errors making it unnecessary to retry.
			if httpError.Code >= 400 && httpError.Code < 500 {
				return err
			}
			return retry.RetryableError(err)
		default:
			return retry.RetryableError(err)
		}
	})
	if err != nil {
		logger.Errorf("failed verifying environment provider configuration: %+v", err)
		// Yes, it's correct with double '&' in these two statements.
		// This is because 'As panics if target is not a non-nil pointer to
		// either a type that implements error, or to any interface type'.
		errType := &HTTPError{}
		if errors.As(err, &errType) {
			return err
		}
		return NewHTTPError(err, http.StatusRequestTimeout)
	}
	return nil
}
