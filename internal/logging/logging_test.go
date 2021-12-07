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
package logging

import (
	"testing"

	"github.com/eiffel-community/etos-api/test/testconfig"
	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"
)

// TestLoggingSetup tests that it is possible to setup logging without a filehook.
func TestLoggingSetup(t *testing.T) {
	cfg := testconfig.Get("127.0.0.1", "8080", "INFO", "", "", "1m")
	log, err := Setup(cfg)
	assert.Nil(t, err)
	assert.Nil(t, log.Hooks[logrus.InfoLevel])
}

// TestLoggingSetupFileHook tests that it is possible to setup logging with a filehook.
func TestLoggingSetupFileHook(t *testing.T) {
	cfg := testconfig.Get("127.0.0.1", "8080", "INFO", "testdata/logs.json", "", "1m")
	log, err := Setup(cfg)
	assert.Nil(t, err)
	assert.NotNil(t, log.Hooks[logrus.InfoLevel])
}

// TestLoggingSetupBadLogLevel shall return an error if loglevel is not parseable.
func TestLoggingSetupBadLogLevel(t *testing.T) {
	cfg := testconfig.Get("127.0.0.1", "8080", "NOTALOGLEVEL", "", "", "1m")
	_, err := Setup(cfg)
	assert.Error(t, err)
}
