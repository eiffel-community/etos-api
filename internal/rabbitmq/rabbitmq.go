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
package rabbitmq

import (
	"github.com/eiffel-community/etos-api/internal/config"
	log "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
)

type RabbitMQPublisher struct {
	logger   *log.Entry
	host     string
	exchange string
	conn     *amqp.Connection
	channel  *amqp.Channel
}

// New creates a new RabbitMQPublisher and connects and creates a channel.
func New(logger *log.Entry, cfg config.Config) *RabbitMQPublisher {
	publisher := &RabbitMQPublisher{
		logger:   logger,
		host:     cfg.RabbitMQHost(),
		exchange: cfg.RabbitMQExchange(),
	}
	conn, err := publisher.Connect()
	if err != nil {
		logger.Panic(err)
	}
	publisher.conn = conn
	if err := publisher.Channel(); err != nil {
		logger.Panic(err)
	}
	return publisher
}

// Connect connects to RabbitMQ.
func (r *RabbitMQPublisher) Connect() (*amqp.Connection, error) {
	r.logger.Infof("connecting to '%s'", r.host)
	conn, err := amqp.Dial(r.host)
	if err != nil {
		return nil, err
	}
	r.conn = conn
	return conn, err
}

// Channel opens a channel to the RabbitMQ connection.
func (r *RabbitMQPublisher) Channel() error {
	r.logger.Info("open channel to rabbitmq connection")
	channel, err := r.conn.Channel()
	if err != nil {
		return err
	}
	r.channel = channel
	return nil
}

// Publish publishes a single event (as []byte) to RabbitMQ.
func (r *RabbitMQPublisher) Publish(event []byte, routingKey string) error {
	r.logger.Infof("publishing event '%s' to routing key '%s'", string(event), routingKey)
	return r.channel.Publish(r.exchange, routingKey, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        event,
	})
}

// Close closes the RabbitMQ connection and channel.
func (r *RabbitMQPublisher) Close() {
	if r.conn != nil {
		r.conn.Close()
	}
	if r.channel != nil {
		r.channel.Close()
	}
}
