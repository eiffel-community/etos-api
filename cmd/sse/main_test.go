// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"os"
	"runtime/debug"
	"testing"
)

// TestVCSRevisionExplicit verifies an injected revision takes precedence over Go metadata.
func TestVCSRevisionExplicit(t *testing.T) {
	original := buildRevision
	t.Cleanup(func() { buildRevision = original })
	buildRevision = "0123456789abcdef"
	if got := vcsRevision(); got != buildRevision {
		t.Fatalf("vcsRevision() = %q, want %q", got, buildRevision)
	}
}

// TestVCSRevisionFallback verifies ordinary builds retain their Go VCS metadata behavior.
func TestVCSRevisionFallback(t *testing.T) {
	original := buildRevision
	t.Cleanup(func() { buildRevision = original })
	buildRevision = ""
	want := "(unknown)"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				want = setting.Value
			}
		}
	}
	if got := vcsRevision(); got != want {
		t.Fatalf("vcsRevision() = %q, want %q", got, want)
	}
}

// TestVCSRevisionLinked verifies the actual linker-injected revision when requested by a build check.
func TestVCSRevisionLinked(t *testing.T) {
	want := os.Getenv("EXPECTED_BUILD_REVISION")
	if want == "" {
		t.Skip("linker injection check requires EXPECTED_BUILD_REVISION")
	}
	if got := vcsRevision(); got != want {
		t.Fatalf("vcsRevision() = %q, want linker revision %q", got, want)
	}
}
