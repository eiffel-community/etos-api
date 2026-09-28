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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// TestFileStreamCancelUnblocksSend checks that a file reader stops even when
// nobody receives the event it is forwarding.
func TestFileStreamCancelUnblocksSend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events")
	require.NoError(t, os.WriteFile(path, []byte("payload\n"), 0600))
	file, err := os.Open(path)
	require.NoError(t, err)
	s := &FileStream{file: file, logger: logrus.NewEntry(logrus.New()), interval: time.Millisecond}
	defer s.Close()
	s.WithChannel(make(chan []byte))
	ctx, cancel := context.WithCancel(context.Background())
	done, err := s.Consume(ctx)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case _, ok := <-done:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("file reader blocked after cancellation")
	}
}
