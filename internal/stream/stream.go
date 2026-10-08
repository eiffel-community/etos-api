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

	"github.com/sirupsen/logrus"
)

// OffsetFirst is the offset to use in order to consume a stream from its first retained message.
const OffsetFirst int64 = -1

// ErrEmptyStream is returned by Stream.FirstOffset when the stream holds no messages.
var ErrEmptyStream = errors.New("stream is empty")

// Message is a single message consumed from a stream together with its offset. The offset is
// assigned by the stream, is immutable and strictly increasing, but not necessarily contiguous
// for a consumer since messages may be filtered out.
type Message struct {
	Offset int64
	Data   []byte
}

type Streamer interface {
	NewStream(context.Context, *logrus.Entry, string) (Stream, error)
	CreateStream(context.Context, *logrus.Entry, string) error
	Close()
}

type Stream interface {
	WithChannel(chan<- Message) Stream
	// WithOffset sets the offset of the first message to consume, or OffsetFirst.
	WithOffset(int64) Stream
	WithFilter([]string) Stream
	Consume(context.Context) (<-chan error, error)
	// FirstOffset returns the offset of the first message retained in the stream, or
	// ErrEmptyStream if there are no messages.
	FirstOffset() (int64, error)
	// Position returns an offset such that every message below it has either been sent on the
	// channel, and received from it, or been filtered out. It never moves backwards.
	Position() int64
	Close()
}
