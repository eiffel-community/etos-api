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
package rabbitmqhook

import (
	"errors"
	"fmt"

	"github.com/eiffel-community/etos-api/internal/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sirupsen/logrus"
)

var fieldMap = logrus.FieldMap{
	logrus.FieldKeyTime:  "@timestamp",
	logrus.FieldKeyMsg:   "message",
	logrus.FieldKeyLevel: "levelname",
}

type RabbitMQHook struct {
	formatter logrus.Formatter
	publisher *rabbitmq.Publisher
}

// NewRabbitMQHook creates a new RabbitMQ hook for use in logrus.
func NewRabbitMQHook(publisher *rabbitmq.Publisher) *RabbitMQHook {
	return &RabbitMQHook{
		formatter: &logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z",
			FieldMap:        fieldMap,
		},
		publisher: publisher,
	}
}

// Fire formats a logrus entry to json and publishes it to a RabbitMQ.
// Will only fire messages if the 'publish' field and 'identifier' fields are set.
// A context must also be set on the entry else publish won't run.
func (h RabbitMQHook) Fire(entry *logrus.Entry) error {
	// Ignore publish to RabbitMQ if user_log or identifier are not set
	if entry.Data["user_log"] == false {
		return nil
	}
	if entry.Data["identifier"] == nil {
		return errors.New("no identifier set to user log entry")
	}
	if entry.Context == nil {
		return errors.New("no context set to user log entry")
	}

	message, err := h.format(entry)
	if err != nil {
		return err
	}
	return h.publish(entry, message)
}

// Levels returns a list of levels that this hook reacts to
func (h RabbitMQHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		logrus.DebugLevel,
		logrus.TraceLevel,
	}
}

// format serializes an entry to a format better suited for ETOS logging.
func (h RabbitMQHook) format(entry *logrus.Entry) ([]byte, error) {
	// Changing the timezone to UTC from whatever the host system uses
	// but we don't want to change the output from the string formatter that
	// prints to terminal, so we store the old Time, change the entry.Time
	// and then change it back to what it was before we started screwing
	// with it.
	originalTime := entry.Time
	entry.Time = entry.Time.UTC()
	formatted, err := h.formatter.Format(entry)
	entry.Time = originalTime
	return formatted, err
}

// publish publishes a log message to RabbitMQ.
func (h RabbitMQHook) publish(entry *logrus.Entry, message []byte) error {
	routingKey := fmt.Sprintf("%s.log.%s", entry.Data["identifier"], entry.Logger.Level.String())
	return h.publisher.Publish(
		entry.Context,
		entry.WithField("user_log", false),
		routingKey,
		amqp.Publishing{Body: message},
	)
}
