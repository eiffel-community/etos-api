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
	"errors"
	"net/http"
	"time"

	"github.com/eiffel-community/etos-api/pkg/v1alpha1/suite"
	"github.com/sethvargo/go-retry"
	"github.com/sirupsen/logrus"
)

// waitForSuiteValidation waits for a test suite to download and validate and then
// reports the result to a channel.
func waitForSuiteValidation(ctx context.Context, logger *logrus.Entry, validator *suite.SuiteValidator, testSuiteURL string, channel chan error) {
	defer close(channel)
	// Make sure we don't write to a closed channel.
	select {
	case <-ctx.Done():
		return
	case channel <- validateTestSuite(ctx, logger, validator, testSuiteURL):
	}
}

// validateSuite will try to download the test suite until it's downloaded properly or the context times out.
// After the test suite is downloaded it will make sure that the test suite is a valid ETOS test suite.
func validateTestSuite(ctx context.Context, logger *logrus.Entry, validator *suite.SuiteValidator, testSuiteURL string) error {
	var testSuite suite.Suite
	logger.Infof("Downloading test suite (%s)", testSuiteURL)
	err := retry.Fibonacci(ctx, 1*time.Second, func(ctx context.Context) error {
		response, err := validator.Download(ctx, logger, testSuiteURL)
		if err != nil {
			// Cannot recover from client errors making it
			// unnecessary to retry the download.
			if response.Code >= 400 && response.Code < 500 {
				return NewHTTPError(err, response.Code)
			} else {
				return retry.RetryableError(err)
			}
		}
		testSuite = response
		return nil
	})
	if err != nil {
		logger.Errorf("failed validating test suite: %+v", err)
		// Yes, it's correct with double '&' in these two statements.
		// This is because 'As panics if target is not a non-nil pointer to
		// either a type that implements error, or to any interface type'.
		errType := &HTTPError{}
		if errors.As(err, &errType) {
			return err
		}
		return NewHTTPError(err, http.StatusRequestTimeout)
	}
	logger.Info("test suite downloaded. Validating")
	err = testSuite.Validate(ctx, logger)
	if err != nil {
		logger.Info("test suite validation failed")
		return NewHTTPError(err, http.StatusBadRequest)
	}
	return nil
}
