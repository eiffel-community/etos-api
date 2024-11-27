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
package config

import (
	"flag"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

type ExecutionSpaceConfig interface {
	Config
	Hostname() string
	Timeout() time.Duration
	ExecutionSpaceWaitTimeout() time.Duration
	RabbitMQHookURL() string
	RabbitMQHookExchangeName() string
	EiffelGoerURL() string
}

// executionSpaceCfg implements the ExecutionSpaceConfig interface.
type executionSpaceCfg struct {
	Config
	stripPrefix               string
	hostname                  string
	timeout                   time.Duration
	executionSpaceWaitTimeout time.Duration
	rabbitmqHookURL           string
	rabbitmqHookExchange      string
	eiffelGoerURL             string
}

// NewExecutionSpaceConfig creates an executio nspace config interface based on input parameters or environment variables.
func NewExecutionSpaceConfig() ExecutionSpaceConfig {
	var conf executionSpaceCfg

	defaultTimeout, err := time.ParseDuration(EnvOrDefault("REQUEST_TIMEOUT", "1m"))
	if err != nil {
		logrus.Panic(err)
	}

	executionSpaceWaitTimeout, err := time.ParseDuration(EnvOrDefault("EXECUTION_SPACE_WAIT_TIMEOUT", "1h"))
	if err != nil {
		logrus.Panic(err)
	}

	flag.StringVar(&conf.hostname, "hostname", EnvOrDefault("PROVIDER_HOSTNAME", "http://localhost"), "Host to supply to ESR for starting executors")
	flag.DurationVar(&conf.timeout, "timeout", defaultTimeout, "Maximum timeout for requests to Execution space provider Service.")
	flag.DurationVar(&conf.executionSpaceWaitTimeout, "executionSpaceWaitTimeout", executionSpaceWaitTimeout, "Timeout duration to wait when trying to checkout execution space(s)")
	flag.StringVar(&conf.rabbitmqHookURL, "rabbitmq_hook_url", os.Getenv("ETOS_RABBITMQ_URL"), "URL to the ETOS rabbitmq for logs")
	flag.StringVar(&conf.rabbitmqHookExchange, "rabbitmq_hook_exchange", os.Getenv("ETOS_RABBITMQ_EXCHANGE"), "Exchange to use for the ETOS rabbitmq for logs")
	flag.StringVar(&conf.eiffelGoerURL, "event_repository_host", os.Getenv("EIFFEL_GOER_URL"), "Event repository URL used for Eiffel event lookup")
	base := load()
	flag.Parse()
	conf.Config = base

	return &conf
}

// Hostname returns the hostname to use for executors.
func (c *executionSpaceCfg) Hostname() string {
	return c.hostname
}

// Timeout returns the request timeout for Execution space provider Service API.
func (c *executionSpaceCfg) Timeout() time.Duration {
	return c.timeout
}

// ExecutionSpaceWaitTimeout returns the timeout for checking out execution spaces.
func (c *executionSpaceCfg) ExecutionSpaceWaitTimeout() time.Duration {
	return c.executionSpaceWaitTimeout
}

// RabbitMQHookURL returns the rabbitmq url for ETOS logs
func (c *executionSpaceCfg) RabbitMQHookURL() string {
	return c.rabbitmqHookURL
}

// EventRepositoryURL returns the Eiffel event repository used for event lookups
func (c *executionSpaceCfg) EiffelGoerURL() string {
	return c.eiffelGoerURL
}

// RabbitMQHookExchangeName returns the rabbitmq exchange name used for ETOS logs
func (c *executionSpaceCfg) RabbitMQHookExchangeName() string {
	return c.rabbitmqHookExchange
}
