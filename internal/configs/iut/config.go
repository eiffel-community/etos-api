// Copyright 2022 Axis Communications AB.
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
package config

import (
	"flag"
	"fmt"
	"os"
)

// Config interface for retrieving configuration options.
type Config interface {
	ServiceHost() string
	ServicePort() string
	StripPrefix() string
	LogLevel() string
	LogFilePath() string
	ETOSNamespace() string
	DatabaseURI() string
}

// cfg implements the Config interface.
type cfg struct {
	serviceHost   string
	servicePort   string
	stripPrefix   string
	logLevel      string
	logFilePath   string
	etosNamespace string
	databaseHost  string
	databasePort  string
}

// Get creates a config interface based on input parameters or environment variables.
func Get() Config {
	var conf cfg

	flag.StringVar(&conf.serviceHost, "address", EnvOrDefault("SERVICE_HOST", "127.0.0.1"), "Address to serve API on")
	flag.StringVar(&conf.servicePort, "port", EnvOrDefault("SERVICE_PORT", "8080"), "Port to serve API on")
	flag.StringVar(&conf.stripPrefix, "stripprefix", EnvOrDefault("STRIP_PREFIX", ""), "Strip a URL prefix. Useful when a reverse proxy sets a subpath. I.e. reverse proxy sets /stream as prefix, making the etos API available at /stream/v1/events. In that case we want to set stripprefix to /stream")
	flag.StringVar(&conf.logLevel, "loglevel", EnvOrDefault("LOGLEVEL", "INFO"), "Log level (TRACE, DEBUG, INFO, WARNING, ERROR, FATAL, PANIC).")
	flag.StringVar(&conf.logFilePath, "logfilepath", os.Getenv("LOG_FILE_PATH"), "Path, including filename, for the log files to create.")
	flag.StringVar(&conf.databaseHost, "database_host", EnvOrDefault("ETOS_ETCD_HOST", "etcd-client"), "Host to ETOS database")
	flag.StringVar(&conf.databasePort, "database_port", EnvOrDefault("ETOS_ETCD_PORT", "2379"), "Port to ETOS database")
	flag.Parse()

	return &conf
}

// ServiceHost returns the host of the service.
func (c *cfg) ServiceHost() string {
	return c.serviceHost
}

// ServicePort returns the port of the service.
func (c *cfg) ServicePort() string {
	return c.servicePort
}

// StripPrefix returns the prefix to strip. Empty string if no prefix.
func (c *cfg) StripPrefix() string {
	return c.stripPrefix
}

// LogLevel returns the log level.
func (c *cfg) LogLevel() string {
	return c.logLevel
}

// LogFilePath returns the path to where log files should be stored, including filename.
func (c *cfg) LogFilePath() string {
	return c.logFilePath
}

// ETOSNamespace returns the ETOS namespace.
func (c *cfg) ETOSNamespace() string {
	return c.etosNamespace
}

// DatabaseURI returns the URI to the ETOS database.
func (c *cfg) DatabaseURI() string {
	return fmt.Sprintf("%s:%s", c.databaseHost, c.databasePort)
}

// EnvOrDefault will look up key in environment variables and return if it exists, else return the fallback value.
func EnvOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}