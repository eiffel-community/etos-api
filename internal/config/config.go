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
	"time"

	"github.com/sirupsen/logrus"
)

// Config interface for retreiving configuration options.
type Config interface {
	APIHost() string
	LogLevel() string
	LogFilePath() string
	RoutingKey(string) string
	EventRepositoryHost() string
	EnvironmentProviderHost() string
	RabbitMQHost() string
	RabbitMQExchange() string
	Timeout() time.Duration
}

// cfg implements the Config interface.
type cfg struct {
	apiHost                 string
	apiPort                 string
	logLevel                string
	logFilePath             string
	eventRepositoryHost     string
	environmentProviderHost string
	routingKeyTag           string
	routingKeyFamily        string
	routingKeyDomain        string
	rabbitMQHost            string
	rabbitMQExchange        string
	timeout                 time.Duration
}

// Get creates a config interface based on input parameters or environment variables.
func Get() Config {
	var conf cfg

	defaultTimeout, err := time.ParseDuration(EnvOrDefault("REQUEST_TIMEOUT", "1m"))
	if err != nil {
		logrus.Panic(err)
	}

	flag.StringVar(&conf.apiHost, "address", EnvOrDefault("API_HOST", "127.0.0.1"), "Address to serve API on")
	flag.StringVar(&conf.apiPort, "port", EnvOrDefault("API_PORT", "8080"), "Port to serve API on")
	flag.StringVar(&conf.logLevel, "loglevel", EnvOrDefault("LOGLEVEL", "INFO"), "Log level (TRACE, DEBUG, INFO, WARNING, ERROR, FATAL, PANIC).")
	flag.StringVar(&conf.logFilePath, "logfilepath", os.Getenv("LOG_FILE_PATH"), "Path, including filename, for the log files to create.")
	flag.StringVar(&conf.eventRepositoryHost, "eventrepository", os.Getenv("ETOS_GRAPHQL_SERVER"), "Host to the GraphQL server to use for event lookup.")
	flag.StringVar(&conf.routingKeyTag, "routingkeytag", EnvOrDefault("RABBITMQ_ROUTING_KEY_TAG", "_"), "Tag to use for routing key. Defaults to '_'.")
	flag.StringVar(&conf.routingKeyFamily, "routingkeyfamily", EnvOrDefault("RABBITMQ_ROUTING_KEY_FAMILY", "_"), "Family to use for routing key. Defaults to '_'.")
	flag.StringVar(&conf.routingKeyDomain, "routingkeydomain", EnvOrDefault("RABBITMQ_ROUTING_KEY_DOMAIN_ID", "_"), "Domain ID to use for routing key. Defaults to '_'.")
	flag.StringVar(&conf.rabbitMQHost, "rabbitmqhost", os.Getenv("RABBITMQ_HOST"), "Host to the RabbitMQ server.")
	flag.StringVar(&conf.rabbitMQExchange, "rabbitmqexchange", os.Getenv("RABBITMQ_EXCHANGE"), "Exchange to the RabbitMQ server.")
	flag.StringVar(&conf.environmentProviderHost, "environmentprovider", os.Getenv("ETOS_ENVIRONMENT_PROVIDER"), "Host to the ETOS environment provider.")
	flag.DurationVar(&conf.timeout, "timeout", defaultTimeout, "Maximum timeout for requests to ETOS API.")

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

// EventRepositoryHost returns the host to use for event lookups.
func (c *cfg) EventRepositoryHost() string {
	return c.eventRepositoryHost
}

// EnvironmentProvider returns the host to use for configuring the environment provider.
func (c *cfg) EnvironmentProviderHost() string {
	return c.environmentProviderHost
}

// Timeout returns the request timeout for starting ETOS.
func (c *cfg) Timeout() time.Duration {
	return c.timeout
}

// RoutingKey is the routing key to use when sending events. The eventType input is the meta.type field in eiffel events.
func (c *cfg) RoutingKey(eventType string) string {
	return fmt.Sprintf("eiffel.%s.%s.%s.%s", c.routingKeyFamily, eventType, c.routingKeyTag, c.routingKeyDomain)
}

// RabbitMQExchange is the exchange to publish events to.
func (c *cfg) RabbitMQExchange() string {
	return c.rabbitMQExchange
}

// RabbitMQHost is the host to RabbitMQ for event publishing
func (c *cfg) RabbitMQHost() string {
	return c.rabbitMQHost
}

// EnvOrDefault will look up key in environment variables and return if it exists, else return the fallback value.
func EnvOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
