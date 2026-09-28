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
	"slices"
	"time"

	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/amqp"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/stream"
	"github.com/sethvargo/go-retry"
	"github.com/sirupsen/logrus"
)

const IgnoreUnfiltered = false
const maxRetries = 10
const retryDelay = 5 * time.Second

// RabbitMQStreamer is a struct representing a single RabbitMQ connection. From a new connection new streams may be created.
// Normal case is to have a single connection with multiple streams. If multiple connections are needed then multiple
// instances of the program should be run.
type RabbitMQStreamer struct {
	environment *stream.Environment
	logger      *logrus.Entry
	streamName  string
}

// NewRabbitMQStreamer creates a new RabbitMQ streamer. Only a single connection should be created.
func NewRabbitMQStreamer(ctx context.Context, options stream.EnvironmentOptions, logger *logrus.Entry, streamName string) (Streamer, error) {
	var err error
	var env *stream.Environment
	if err = retry.Do(ctx, retry.WithMaxRetries(maxRetries, retry.NewConstant(retryDelay)), func(ctx context.Context) error {
		env, err = stream.NewEnvironment(&options)
		if err != nil {
			logger.WithError(err).Warning("failed to connect to RabbitMQ, retrying...")
			return retry.RetryableError(err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ after %d retries: %w", maxRetries, err)
	}
	return &RabbitMQStreamer{environment: env, logger: logger, streamName: streamName}, err
}

// CreateStream creates a new RabbitMQ stream.
func (s *RabbitMQStreamer) CreateStream(ctx context.Context, logger *logrus.Entry, name string) error {
	logger.Info("Defining a new stream")
	// This will create the stream if not already created.
	return s.environment.DeclareStream(name,
		&stream.StreamOptions{
			MaxLengthBytes: stream.ByteCapacity{}.GB(2),
			MaxAge:         time.Hour * 48,
		},
	)
}

// NewStream creates a new stream struct to consume from.
func (s *RabbitMQStreamer) NewStream(ctx context.Context, logger *logrus.Entry, name string) (Stream, error) {
	exists, err := s.environment.StreamExists(s.streamName)
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
	return &RabbitMQStream{
		logger:      logger,
		streamName:  s.streamName,
		identifier:  name,
		environment: s.environment,
		options:     options,
	}, nil
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
	logger      *logrus.Entry
	streamName  string
	identifier  string
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
		s.handleMessage(ctx, message)
	}
	consumer, err := s.environment.NewConsumer(s.streamName, handler, s.options)
	if err != nil {
		return nil, err
	}
	s.consumer = consumer
	closed := make(chan error, 1)
	go notifyClose(ctx, consumer.NotifyClose(), closed)
	return closed, nil
}

// handleMessage forwards message data from the consumer callback. RabbitMQ applies postFilter
// before this callback only when optional filters are configured, so unfiltered streams apply
// the identifier check here.
func (s *RabbitMQStream) handleMessage(ctx context.Context, message *amqp.Message) {
	if len(s.filter) == 0 && !s.postFilter(message) {
		return
	}
	for _, d := range message.Data {
		if s.channel != nil {
			select {
			case s.channel <- d:
			case <-ctx.Done():
				return
			}
		} else {
			s.logger.Debug(d)
		}
	}
}

// notifyClose forwards a broker close unless the subscription has ended.
func notifyClose(ctx context.Context, closed stream.ChannelClose, ch chan<- error) {
	select {
	case <-ctx.Done():
		return
	case event := <-closed:
		select {
		case ch <- event.Err:
		case <-ctx.Done():
		}
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
	identifier, ok := message.ApplicationProperties["identifier"].(string)
	if !ok || identifier != s.identifier {
		return false
	}
	if len(s.filter) == 0 {
		return true
	}
	eventType := message.ApplicationProperties["type"]
	eventMeta := message.ApplicationProperties["meta"]
	name := fmt.Sprintf("%s.%s.%s", identifier, eventType, eventMeta)
	return slices.Contains(s.filter, name)
}
