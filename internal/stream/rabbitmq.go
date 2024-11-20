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
package stream

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/amqp"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/stream"
	"github.com/sirupsen/logrus"
)

const IgnoreUnfiltered = false

// RabbitMQStreamer is a struct representing a single RabbitMQ connection. From a new connection new streams may be created.
// Normal case is to have a single connection with multiple streams. If multiple connections are needed then multiple
// instances of the program should be run.
type RabbitMQStreamer struct {
	environment *stream.Environment
	logger      *logrus.Entry
}

// NewRabbitMQStreamer creates a new RabbitMQ streamer. Only a single connection should be created.
func NewRabbitMQStreamer(options stream.EnvironmentOptions, logger *logrus.Entry) (Streamer, error) {
	env, err := stream.NewEnvironment(&options)
	if err != nil {
		log.Fatal(err)
	}
	return &RabbitMQStreamer{environment: env, logger: logger}, err
}

// CreateStream creates a new RabbitMQ stream.
func (s *RabbitMQStreamer) CreateStream(ctx context.Context, logger *logrus.Entry, name string) error {
	logger.Info("Defining a new stream")
	// This will create the stream if not already created.
	return s.environment.DeclareStream(name,
		&stream.StreamOptions{
			// TODO: More sane numbers
			MaxLengthBytes: stream.ByteCapacity{}.GB(2),
			MaxAge:         time.Second * 10,
		},
	)
}

// NewStream creates a new stream struct to consume from.
func (s *RabbitMQStreamer) NewStream(ctx context.Context, logger *logrus.Entry, name string) (Stream, error) {
	exists, err := s.environment.StreamExists(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("no stream exists, cannot stream events")
	}
	options := stream.NewConsumerOptions().
		SetClientProvidedName(name).
		SetConsumerName(name).
		SetCRCCheck(false).
		SetOffset(stream.OffsetSpecification{}.First())
	return &RabbitMQStream{ctx: ctx, logger: logger, streamName: name, environment: s.environment, options: options}, nil
}

// Close the RabbitMQ connection.
func (s *RabbitMQStreamer) Close() {
	err := s.environment.Close()
	if err != nil {
		s.logger.WithError(err).Error("failed to close RabbitMQStreamer")
	}
}

// RabbitMQStream is a structure implementing the Stream interface. Used to consume events
// from a RabbitMQ stream.
type RabbitMQStream struct {
	ctx         context.Context
	logger      *logrus.Entry
	streamName  string
	environment *stream.Environment
	options     *stream.ConsumerOptions
	consumer    *stream.Consumer
	channel     chan<- []byte
	filter      []string
}

// WithChannel adds a channel for receiving events from the stream. If no
// channel is added, then events will be logged.
func (s *RabbitMQStream) WithChannel(ch chan<- []byte) Stream {
	s.channel = ch
	return s
}

// WithOffset adds an offset to the RabbitMQ stream. -1 means start from the beginning.
func (s *RabbitMQStream) WithOffset(offset int) Stream {
	if offset == -1 {
		s.options = s.options.SetOffset(stream.OffsetSpecification{}.First())
	} else {
		s.options = s.options.SetOffset(stream.OffsetSpecification{}.Offset(int64(offset)))
	}
	return s
}

// WithFilter adds a filter to the RabbitMQ stream.
func (s *RabbitMQStream) WithFilter(filter []string) Stream {
	s.filter = filter
	if len(filter) > 0 {
		s.options = s.options.SetFilter(stream.NewConsumerFilter(filter, IgnoreUnfiltered, s.postFilter))
	}
	return s
}

// Consume will start consuming the RabbitMQ stream, non blocking. A channel is returned where
// an error is sent when the consumer closes down.
func (s *RabbitMQStream) Consume(ctx context.Context) (<-chan error, error) {
	handler := func(_ stream.ConsumerContext, message *amqp.Message) {
		for _, d := range message.Data {
			if s.channel != nil {
				s.channel <- d
			} else {
				s.logger.Debug(d)
			}
		}
	}
	consumer, err := s.environment.NewConsumer(s.streamName, handler, s.options)
	if err != nil {
		return nil, err
	}
	s.consumer = consumer
	closed := make(chan error)
	go s.notifyClose(ctx, closed)
	return closed, nil
}

// notifyClose will keep track of context and the notify close channel from RabbitMQ and send
// error on a channel.
func (s *RabbitMQStream) notifyClose(ctx context.Context, ch chan<- error) {
	closed := s.consumer.NotifyClose()
	select {
	case <-ctx.Done():
		ch <- ctx.Err()
	case event := <-closed:
		ch <- event.Err
	}
}

// Close the RabbitMQ stream consumer.
func (s *RabbitMQStream) Close() {
	if s.consumer != nil {
		if err := s.consumer.Close(); err != nil {
			s.logger.WithError(err).Error("failed to close rabbitmq consumer")
		}
	}
}

// postFilter applies client side filtering on all messages received from the RabbitMQ stream.
// The RabbitMQ server-side filtering is not perfect and will let through a few messages that don't
// match the filter, this is expected as the RabbitMQ unit of delivery is the chunk and there may
// be multiple messages in a chunk and those messages are not filtered.
func (s *RabbitMQStream) postFilter(message *amqp.Message) bool {
	if s.filter == nil {
		return true // Unfiltered
	}
	identifier := message.ApplicationProperties["identifier"]
	eventType := message.ApplicationProperties["type"]
	eventMeta := message.ApplicationProperties["meta"]
	name := fmt.Sprintf("%s.%s.%s", identifier, eventType, eventMeta)
	for _, filter := range s.filter {
		if name == filter {
			return true
		}
	}
	return false
}
