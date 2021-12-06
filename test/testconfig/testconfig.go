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

	"github.com/eiffel-community/etos-api/internal/config"
)

type cfg struct {
	apiHost     string
	apiPort     string
	logLevel    string
	logFilePath string
}

func Get(apiHost, apiPort, logLevel, logFilePath string) config.Config {
	return &cfg{
		apiHost:     apiHost,
		apiPort:     apiPort,
		logLevel:    logLevel,
		logFilePath: logFilePath,
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
