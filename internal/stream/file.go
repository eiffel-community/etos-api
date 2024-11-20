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
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

// FileStreamer will create a stream that reads from a file and publishes them
// to a consumer.
type FileStreamer struct {
	interval time.Duration
	logger   *logrus.Entry
}

func NewFileStreamer(interval time.Duration, logger *logrus.Entry) (Streamer, error) {
	return &FileStreamer{interval: interval, logger: logger}, nil
}

// CreateStream does nothing.
func (s *FileStreamer) CreateStream(ctx context.Context, logger *logrus.Entry, name string) error {
	return nil
}

// NewStream creates a new stream struct to consume from.
func (s *FileStreamer) NewStream(ctx context.Context, logger *logrus.Entry, name string) (Stream, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return &FileStream{ctx: ctx, file: file, interval: s.interval, logger: logger}, nil
}

// Close does nothing.
func (s *FileStreamer) Close() {}

// FileStream is a structure implementing the Stream interface. Used to consume events
// from a file.
type FileStream struct {
	ctx      context.Context
	logger   *logrus.Entry
	file     io.ReadCloser
	interval time.Duration
	offset   int
	channel  chan<- []byte
	filter   []string
}

// WithChannel adds a channel for receiving events from the stream. If no
// channel is added, then events will be logged.
func (s *FileStream) WithChannel(ch chan<- []byte) Stream {
	s.channel = ch
	return s
}

// WithOffset adds an offset to the file stream. -1 means start from the beginning.
func (s *FileStream) WithOffset(offset int) Stream {
	s.logger.Warning("offset is not yet supported by file stream")
	return s
}

// WithFilter adds a filter to the file stream.
func (s *FileStream) WithFilter(filter []string) Stream {
	s.logger.Warning("filter is not yet supported by file stream")
	return s
}

// Consume will start consuming the file, non blocking. A channel is returned where
// an error is sent when the consumer closes down.
func (s *FileStream) Consume(ctx context.Context) (<-chan error, error) {
	closed := make(chan error)
	go func() {
		scanner := bufio.NewReader(s.file)
		interval := time.NewTicker(s.interval)
		for {
			select {
			case <-ctx.Done():
				closed <- ctx.Err()
				return
			case <-interval.C:
				var isPrefix bool = true
				var err error
				var line []byte
				var event []byte

				for isPrefix && err == nil {
					line, isPrefix, err = scanner.ReadLine()
					event = append(event, line...)
				}
				if err != nil {
					// Don't close the stream just because the file is empty.
					if errors.Is(err, io.EOF) {
						continue
					}
					closed <- err
					return
				}
				if s.channel != nil {
					s.channel <- event
				} else {
					s.logger.Info(event)
				}
			}
		}
	}()
	return closed, nil
}

// Close the file.
func (s *FileStream) Close() {
	if s.file != nil {
		if err := s.file.Close(); err != nil {
			s.logger.WithError(err).Error("failed to close the file")
		}
	}
}
