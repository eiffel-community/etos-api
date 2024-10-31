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
package scope

import (
	"strings"
)

type (
	Var string
	// Scope is a slice of actions and resources that a token can do.
	// For example the string "post-testrun" means that the action "post" can
	// be done on "testrun", i.e. create a new testrun for ETOS.
	// The actions are HTTP methods and the resource is a name that the service
	// described in the 'aud' claim has knowledge about.
	Scope []Var
)

// Format a scope to be used in a JWT.
func (s Scope) Format() string {
	var str []string
	for _, sc := range s {
		str = append(str, string(sc))
	}
	return strings.Join(str, " ")
}

// Has checks if a scope has a specific action + resource.
func (s Scope) Has(scope Var) bool {
	for _, sc := range s {
		if scope == sc {
			return true
		}
	}
	return false
}

// Parse a scope string and return a Scope.
func Parse(scope string) Scope {
	split := strings.Split(scope, " ")
	var s Scope
	for _, sc := range split {
		s = append(s, Var(sc))
	}
	return s
}

var (
	AnonymousAccess Scope = []Var{CreateTestrun, StopTestrun, StreamSSE}
	AdminAccess     Scope = []Var{CreateTestrun, StopTestrun, GetEnvironment, DefineSSE, StreamSSE}
)
