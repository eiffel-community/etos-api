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
package provider

import (
	"fmt"
	"sync"

	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/eiffel-community/etos-api/internal/database"
	"github.com/eiffel-community/etos-api/internal/executionspace/executor"
)

type Kubernetes struct {
	providerCore
}

// New creates a copy of a Kubernetes provider
func (k Kubernetes) New(db database.Opener, cfg config.ExecutionSpaceConfig) Provider {
	return &Kubernetes{
		providerCore{
			db:  db,
			cfg: cfg,
			url: fmt.Sprintf("%s/executionspace/v1alpha/executor/kubernetes", cfg.Hostname()),
			executor: executor.Kubernetes(
				cfg.ETOSNamespace(),
				cfg.EncryptionKeySecretName(),
			),
			active: &sync.WaitGroup{},
		},
	}
}
