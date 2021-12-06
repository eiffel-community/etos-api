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
package config

import (
	"flag"
	"fmt"
	"os"
)

// Config interface for retreiving configuration options.
type Config interface {
	APIHost() string
	LogLevel() string
	LogFilePath() string
}

// cfg implements the Config interface.
type cfg struct {
	apiHost     string
	apiPort     string
	logLevel    string
	logFilePath string
}

// Get creates a config interface based on input parameters or environment variables.
func Get() Config {
	var conf cfg

	flag.StringVar(&conf.apiHost, "address", EnvOrDefault("API_HOST", "127.0.0.1"), "Address to serve API on")
	flag.StringVar(&conf.apiPort, "port", EnvOrDefault("API_PORT", "8080"), "Port to serve API on")
	flag.StringVar(&conf.logLevel, "loglevel", EnvOrDefault("LOGLEVEL", "INFO"), "Log level (TRACE, DEBUG, INFO, WARNING, ERROR, FATAL, PANIC).")
	flag.StringVar(&conf.logFilePath, "logfilepath", os.Getenv("LOG_FILE_PATH"), "Path, including filename, for the log files to create.")

	flag.Parse()
	return &conf
}

// APIHost returns the host and port of a server.
func (c *cfg) APIHost() string {
	return fmt.Sprintf("%s:%s", c.apiHost, c.apiPort)
}

// LogLevel returns the log level.
func (c *cfg) LogLevel() string {
	return c.logLevel
}

// LogFilePath returns the path to where log files should be stored, including filename.
func (c *cfg) LogFilePath() string {
	return c.logFilePath
}

// EnvOrDefault will look up key in environment variables and return if it exists, else return the fallback value.
func EnvOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
