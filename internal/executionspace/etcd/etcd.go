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
package etcd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	config "github.com/eiffel-community/etos-api/internal/configs/executionspace"
	"github.com/eiffel-community/etos-api/internal/executionspace/database"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const etcdTreePrefix string = "/execution-space"

// TODO: refactor the client so that it does not store data it fetched.
// However, without it implementing the database.Opener interface would be more complex (methods readByte, read).
type Etcd struct {
	cfg     config.Config
	client  *clientv3.Client
	ID      uuid.UUID
	ctx     context.Context
	data    []byte
	hasRead bool
}

// New returns a new Etcd Object/Struct.
func New(cfg config.Config, logger *logrus.Logger) database.Opener {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{cfg.DatabaseURI()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		panic(err)
	}

	return Etcd{
		client: client,
		cfg:    cfg,
	}
}

// Open returns a copy of an Etcd client with ID and context added
func (etcd Etcd) Open(ctx context.Context, id uuid.UUID) io.ReadWriter {
	return &Etcd{
		client: etcd.client,
		cfg:    etcd.cfg,
		ID:     id,
		ctx:    ctx,
	}
}

// Write writes data to etcd
func (etcd Etcd) Write(p []byte) (int, error) {
	if etcd.ID == uuid.Nil {
		return 0, errors.New("please create a new etcd client using Open")
	}
	key := fmt.Sprintf("%s/%s", etcdTreePrefix, etcd.ID.String())
	_, err := etcd.client.Put(etcd.ctx, key, string(p))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// readByte reads a single byte from etcd.data and reduces the slice afterwards
func (etcd *Etcd) readByte() byte {
	b := etcd.data[0]
	etcd.data = etcd.data[1:]
	return b
}

// Read reads data from etcd and returns p bytes to user
func (etcd *Etcd) Read(p []byte) (n int, err error) {
	if etcd.ID == uuid.Nil {
		err = errors.New("please create a new etcd client using NewWithID")
		return n, err
	}

	key := fmt.Sprintf("%s/%s", etcdTreePrefix, etcd.ID.String())

	if !etcd.hasRead {
		resp, err := etcd.client.Get(etcd.ctx, key)
		if err != nil {
			return n, err
		}
		if len(resp.Kvs) == 0 {
			return n, io.EOF
		}
		etcd.data = resp.Kvs[0].Value
		etcd.hasRead = true
	}

	if len(etcd.data) == 0 {
		return n, io.EOF
	}
	if c := cap(p); c > 0 {
		for n < c {
			p[n] = etcd.readByte()
			n++
			if len(etcd.data) == 0 {
				return n, io.EOF
			}
		}
	}
	return n, nil
}
