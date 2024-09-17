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
package executionspace

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/google/uuid"
)

type CheckoutStatus string

const (
	Pending CheckoutStatus = "PENDING"
	Failed  CheckoutStatus = "FAILED"
	Done    CheckoutStatus = "DONE"
)

type ExecutionSpace struct {
	ID          uuid.UUID      `json:"id"`
	Executors   []ExecutorSpec `json:"execution_spaces"`
	References  []string       `json:"references"`
	Status      CheckoutStatus `json:"status"`
	Description string         `json:"description"`
}

// New creates a new ExecutionSpace
func New(id uuid.UUID) *ExecutionSpace {
	return &ExecutionSpace{
		ID:          id,
		Status:      Pending,
		Description: "Checking out execution spaces",
	}
}

// Load loads an ExecutionSpace from an io Reader
func Load(r io.Reader) (*ExecutionSpace, error) {
	executionSpace := &ExecutionSpace{}
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(executionSpace); err != nil {
		return nil, err
	}
	return executionSpace, nil
}

// Add adds a new executor to an execution space
func (e *ExecutionSpace) Add(executor ExecutorSpec) {
	e.Executors = append(e.Executors, executor)
	e.References = append(e.References, executor.ID.String())
}

// Save saves an execution space to an io Writer
func (e ExecutionSpace) Save(w io.Writer) error {
	// Executors should be written separately into their own writers.
	e.Executors = nil

	encoder := json.NewEncoder(w)
	if err := encoder.Encode(e); err != nil {
		return errors.Join(errors.New("failed to write execution space to database"), err)
	}
	return nil
}

// Fail writes a failure message to an io Writer
func (e ExecutionSpace) Fail(w io.Writer, err error) error {
	fakeExecutionSpace := ExecutionSpace{
		ID:          e.ID,
		Status:      Failed,
		Description: err.Error(),
	}
	return fakeExecutionSpace.Save(w)
}
