package sse

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

// Connection is a struct representing a single RabbitMQ connection. From a new connection new streams may be created.
// Normal case is to have a single connection with multiple streams. If multiple connections are needed then multiple
// instances of the program should be run.
type Connection struct {
	environment *stream.Environment
}

// NewConnection creates a new RabbitMQ connection. Only a single connection should be created.
func NewConnection(options stream.EnvironmentOptions) (*Connection, error) {
	env, err := stream.NewEnvironment(&options)
	if err != nil {
		log.Fatal(err)
	}
	return &Connection{environment: env}, err
}

// CreateStream creates a new RabbitMQ stream.
func (c *Connection) CreateStream(ctx context.Context, logger *logrus.Entry, name string) error {
	logger.Info("Defining a new stream")
	// This will create the stream if not already created.
	return c.environment.DeclareStream(name,
		&stream.StreamOptions{
			// TODO: More sane numbers
			MaxLengthBytes: stream.ByteCapacity{}.GB(2),
			MaxAge:         time.Second * 10,
		},
	)
}

// Close the RabbitMQ connection.
func (c *Connection) Close() error {
	return c.environment.Close()
}

// Stream is a struct used for consuming a stream queue in RabbitMQ.
type Stream struct {
	ctx         context.Context
	logger      *logrus.Entry
	streamName  string
	environment *stream.Environment
	filter      *stream.ConsumerFilter
	consumer    *stream.Consumer
}

type Consumer struct {
	*stream.Consumer
	filter []string
}

// NewStream creates a new stream struct to consume from.
func (c *Connection) NewStream(ctx context.Context, logger *logrus.Entry, name string) (*Stream, error) {
	exists, err := c.environment.StreamExists(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("no stream exists, cannot stream events")
	}
	return &Stream{ctx: ctx, logger: logger, streamName: name, environment: c.environment}, nil
}

// Consume the stream and pass all messages to a MessagesHandler function. A channel is returned where RabbitMQ will report
// if the channel has been closed. It is up to the client to understand what to do in that case.
func (s *Stream) Consume(handler stream.MessagesHandler, offset stream.OffsetSpecification, filter []string) (*Consumer, error) {
	c := &Consumer{filter: filter}
	options := stream.NewConsumerOptions().
		SetClientProvidedName(s.streamName).
		SetConsumerName(s.streamName).
		SetOffset(offset).
		SetCRCCheck(false)
	if len(filter) > 0 {
		options = options.SetFilter(stream.NewConsumerFilter(filter, IgnoreUnfiltered, c.postFilter))
	}
	consumer, err := s.environment.NewConsumer(s.streamName, handler, options)
	if err != nil {
		return nil, err
	}
	c.Consumer = consumer
	return c, nil
}

// postFilter applies client side filtering on all messages received from the RabbitMQ stream.
// The RabbitMQ server-side filtering is not perfect and will let through a few messages that don't
// match the filter, this is expected as the RabbitMQ unit of delivery is the chunk and there may
// be multiple messages in a chunk and those messages are not filtered.
func (c *Consumer) postFilter(message *amqp.Message) bool {
	if c.filter == nil {
		return true // Unfiltered
	}
	identifier := message.ApplicationProperties["identifier"]
	eventType := message.ApplicationProperties["type"]
	eventMeta := message.ApplicationProperties["meta"]
	name := fmt.Sprintf("%s.%s.%s", identifier, eventType, eventMeta)
	for _, filter := range c.filter {
		if name == filter {
			return true
		}
	}
	return false
}
