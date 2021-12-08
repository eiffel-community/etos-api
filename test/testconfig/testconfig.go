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
package testconfig

import (
	"fmt"
	"time"

	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/sirupsen/logrus"
)

type cfg struct {
	apiHost             string
	apiPort             string
	logLevel            string
	logFilePath         string
	eventRepositoryHost string
	timeout             time.Duration
}

func Get(apiHost, apiPort, logLevel, logFilePath, erHost, timeoutStr string) config.Config {
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		logrus.Panic("timeout could not be parsed")
	}
	return &cfg{
		apiHost:             apiHost,
		apiPort:             apiPort,
		logLevel:            logLevel,
		logFilePath:         logFilePath,
		eventRepositoryHost: erHost,
		timeout:             timeout,
	}
}

func (c *cfg) APIHost() string {
	return fmt.Sprintf("%s:%s", c.apiHost, c.apiPort)
}

func (c *cfg) LogLevel() string {
	return c.logLevel
}

func (c *cfg) LogFilePath() string {
	return c.logFilePath
}

func (c *cfg) EventRepositoryHost() string {
	return c.eventRepositoryHost
}

func (c *cfg) Timeout() time.Duration {
	return c.timeout
}
