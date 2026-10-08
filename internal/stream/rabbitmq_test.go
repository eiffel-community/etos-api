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
	"testing"
	"time"

	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/amqp"
	"github.com/rabbitmq/rabbitmq-stream-go-client/pkg/stream"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TestRabbitMQStreamHandleMessageScopesToIdentifier tests that a RabbitMQ stream only forwards
// events for its requested identifier, with or without optional filters, and that fully qualified
// filters for another identifier cannot broaden access.
func TestRabbitMQStreamHandleMessageScopesToIdentifier(t *testing.T) {
	tests := []struct {
		name      string
		filter    []string
		eventID   string
		eventType string
		eventMeta string
		want      bool
	}{
		{name: "unfiltered matching run", eventID: "run-a", want: true},
		{name: "unfiltered other run", eventID: "run-b"},
		{
			name:      "matching optional filter",
			filter:    []string{"run-a.log.info"},
			eventID:   "run-a",
			eventType: "log",
			eventMeta: "info",
			want:      true,
		},
		{
			name:      "fully qualified cross-run filter cannot broaden access",
			filter:    []string{"run-b.log.info"},
			eventID:   "run-b",
			eventType: "log",
			eventMeta: "info",
		},
		{
			name:      "fully qualified filter for another run excludes requested run event",
			filter:    []string{"run-b.log.info"},
			eventID:   "run-a",
			eventType: "log",
			eventMeta: "info",
		},
		{
			name:      "optional filter excludes event",
			filter:    []string{"run-a.log.info"},
			eventID:   "run-a",
			eventType: "log",
			eventMeta: "debug",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			received := make(chan []byte, 1)
			s := &RabbitMQStream{
				logger:     logrus.NewEntry(logrus.New()),
				identifier: "run-a",
				options:    stream.NewConsumerOptions(),
			}

			s.WithChannel(received)
			s.WithFilter(tt.filter)
			message := &amqp.Message{
				ApplicationProperties: map[string]any{
					"identifier": tt.eventID,
					"type":       tt.eventType,
					"meta":       tt.eventMeta,
				},
				Data: [][]byte{[]byte("event payload")},
			}
			// The RabbitMQ client runs the configured post-filter before invoking the callback.
			if s.options.Filter == nil || s.options.Filter.PostFilter(message) {
				s.handleMessage(context.Background(), message)
			}

			if tt.want {
				assert.Equal(t, []byte("event payload"), <-received)
			} else {
				assert.Empty(t, received)
			}
		})
	}
}

// TestRabbitMQStreamHandleMessageCancelUnblocksSend checks that a blocked broker
// callback exits when its subscription is canceled.
func TestRabbitMQStreamHandleMessageCancelUnblocksSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &RabbitMQStream{
		identifier: "run-a",
		channel:    make(chan []byte),
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.handleMessage(ctx, &amqp.Message{
			ApplicationProperties: map[string]any{"identifier": "run-a"},
			Data:                  [][]byte{[]byte("payload")},
		})
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("broker callback blocked after cancellation")
	}
}

// TestNotifyCloseCancelUnblocksSend checks that a close notification does not
// strand its goroutine if the subscriber no longer reads from its channel.
func TestNotifyCloseCancelUnblocksSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	notifications := make(chan stream.Event)
	result := make(chan error)
	done := make(chan struct{})
	go func() {
		defer close(done)
		notifyClose(ctx, notifications, result)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("close notifier blocked after cancellation")
	}
}

// TestNotifyCloseForwardsBrokerError checks the registered notification is
// delivered without requiring a receiver to be ready at the same instant.
func TestNotifyCloseForwardsBrokerError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	notifications := make(chan stream.Event, 1)
	result := make(chan error, 1)
	brokerErr := errors.New("broker closed")
	notifications <- stream.Event{Err: brokerErr}
	done := make(chan struct{})
	go func() {
		defer close(done)
		notifyClose(ctx, notifications, result)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("close notifier blocked without a waiting subscriber")
	}
	assert.ErrorIs(t, <-result, brokerErr)
}
