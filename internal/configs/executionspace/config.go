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
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

// Config interface for retreiving configuration options.
type Config interface {
	ServiceHost() string
	ServicePort() string
	StripPrefix() string
	Hostname() string
	LogLevel() string
	LogFilePath() string
	Timeout() time.Duration
	KubernetesNamespace() string
	ExecutionSpaceWaitTimeout() time.Duration
	RabbitMQHookUrl() string
	RabbitMQHookExchangeName() string
	DatabaseURI() string
	ETOSNamespace() string
}

// cfg implements the Config interface.
type cfg struct {
	serviceHost               string
	servicePort               string
	stripPrefix               string
	hostname                  string
	logLevel                  string
	logFilePath               string
	timeout                   time.Duration
	databaseHost              string
	databasePort              string
	kubernetesNamespace       string
	executionSpaceWaitTimeout time.Duration
	rabbitmqHookURL           string
	rabbitmqHookExchange      string
	etosNamespace             string
}

// Get creates a config interface based on input parameters or environment variables.
func Get() Config {
	var conf cfg

	defaultTimeout, err := time.ParseDuration(EnvOrDefault("REQUEST_TIMEOUT", "1m"))
	if err != nil {
		logrus.Panic(err)
	}

	executionSpaceWaitTimeout, err := time.ParseDuration(EnvOrDefault("EXECUTION_SPACE_WAIT_TIMEOUT", "1h"))
	if err != nil {
		logrus.Panic(err)
	}

	flag.StringVar(&conf.serviceHost, "address", EnvOrDefault("SERVICE_HOST", "127.0.0.1"), "Address to serve API on")
	flag.StringVar(&conf.servicePort, "port", EnvOrDefault("SERVICE_PORT", "8080"), "Port to serve API on")
	flag.StringVar(&conf.stripPrefix, "stripprefix", EnvOrDefault("STRIP_PREFIX", ""), "Strip prefix")
	flag.StringVar(&conf.hostname, "hostname", EnvOrDefault("PROVIDER_HOSTNAME", "http://localhost"), "Host to supply to ESR for starting executors")
	flag.StringVar(&conf.logLevel, "loglevel", EnvOrDefault("LOGLEVEL", "INFO"), "Log level (TRACE, DEBUG, INFO, WARNING, ERROR, FATAL, PANIC).")
	flag.StringVar(&conf.logFilePath, "logfilepath", os.Getenv("LOG_FILE_PATH"), "Path, including filename, for the log files to create.")
	flag.DurationVar(&conf.timeout, "timeout", defaultTimeout, "Maximum timeout for requests to Execution space provider Service.")
	flag.StringVar(&conf.databaseHost, "database_host", EnvOrDefault("ETOS_ETCD_HOST", "etcd-client"), "Host to ETOS database")
	flag.StringVar(&conf.databasePort, "database_port", EnvOrDefault("ETOS_ETCD_PORT", "2379"), "Port to ETOS database")
	flag.StringVar(&conf.kubernetesNamespace, "kubernetes_namespace", os.Getenv("KUBERNETES_NAMESPACE"), "Namespace to start k8s jobs")
	flag.DurationVar(&conf.executionSpaceWaitTimeout, "execution space wait timeout", executionSpaceWaitTimeout, "Timeout duration to wait when trying to checkout execution space(s)")
	flag.StringVar(&conf.rabbitmqHookURL, "rabbitmq_hook_url", os.Getenv("ETOS_RABBITMQ_URL"), "URL to the ETOS rabbitmq for logs")
	flag.StringVar(&conf.rabbitmqHookExchange, "rabbitmq_hook_exchange", os.Getenv("ETOS_RABBITMQ_EXCHANGE"), "Exchange to use for the ETOS rabbitmq for logs")
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

// StripPrefix returns a prefix that is supposed to be stripped from URL.
func (c *cfg) StripPrefix() string {
	return c.stripPrefix
}

// Hostname returns the hostname to use for executors
func (c *cfg) Hostname() string {
	return c.hostname
}

// LogLevel returns the log level.
func (c *cfg) LogLevel() string {
	return c.logLevel
}

// LogFilePath returns the path to where log files should be stored, including filename.
func (c *cfg) LogFilePath() string {
	return c.logFilePath
}

// Timeout returns the request timeout for Execution space provider Service API.
func (c *cfg) Timeout() time.Duration {
	return c.timeout
}

// KubernetesNamespace returns the namespace where k8s jobs shall be deployed.
func (c *cfg) KubernetesNamespace() string {
	return c.kubernetesNamespace
}

// ExecutionSpaceWaitTimeout returns the timeout for checking out execution spaces.
func (c *cfg) ExecutionSpaceWaitTimeout() time.Duration {
	return c.executionSpaceWaitTimeout
}

// RabbitMQHookURL returns the rabbitmq url for ETOS logs
func (c *cfg) RabbitMQHookUrl() string {
	return c.rabbitmqHookURL
}

// RabbitMQHookExchangeName returns the rabbitmq exchange name used for ETOS logs
func (c *cfg) RabbitMQHookExchangeName() string {
	return c.rabbitmqHookExchange
}

// DatabaseURI returns the URI to the ETOS database.
func (c *cfg) DatabaseURI() string {
	return fmt.Sprintf("%s:%s", c.databaseHost, c.databasePort)
}

// ETOSNamespace returns the ETOS namespace.
func (c *cfg) ETOSNamespace() string {
	return c.etosNamespace
}

// EnvOrDefault will look up key in environment variables and return if it exists, else return the fallback value.
func EnvOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
