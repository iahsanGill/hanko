// Copyright 2026 The Hanko Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package runner

import (
	"context"
	"strings"
	"testing"

	hkctx "github.com/iahsanGill/hanko/pkg/context"
)

type fakeRunner struct{ name string }

func (f *fakeRunner) Name() string { return f.name }
func (f *fakeRunner) Run(_ context.Context, _ *hkctx.RunContext, _ RunOptions) error {
	return nil
}

func TestRegisterAndGet(t *testing.T) {
	Register(&fakeRunner{name: "test-harness-A"})
	got, err := Get("test-harness-A")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name() != "test-harness-A" {
		t.Errorf("Name: got %q", got.Name())
	}
}

func TestGet_Unknown(t *testing.T) {
	_, err := Get("nope-not-here")
	if err == nil {
		t.Fatal("expected error for unknown harness")
	}
	if !strings.Contains(err.Error(), "unknown harness") {
		t.Errorf("error should mention 'unknown harness': %v", err)
	}
}

func TestRegister_OverwriteAllowed(t *testing.T) {
	Register(&fakeRunner{name: "overwrite-me"})
	Register(&fakeRunner{name: "overwrite-me"})
	// Just verify no panic and lookup still works.
	if _, err := Get("overwrite-me"); err != nil {
		t.Errorf("second Register should not break Get: %v", err)
	}
}
