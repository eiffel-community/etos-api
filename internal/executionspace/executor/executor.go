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
package executor

import (
	"context"

	"github.com/eiffel-community/etos-api/pkg/executionspace/executionspace"
	"github.com/sirupsen/logrus"
)

type Executor interface {
	Name() string
	Start(context.Context, *logrus.Entry, *executionspace.ExecutorSpec) (string, error)
	Wait(context.Context, *logrus.Entry, string, *executionspace.ExecutorSpec) (string, string, error)
	Cancel(context.Context, *logrus.Entry, string) error
	Stop(context.Context, *logrus.Entry, string) error
	Alive(context.Context, *logrus.Entry, string) (bool, error)
}
