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

// Package runner defines the seam between hanko and an evaluation harness.
// Each supported harness (lm-evaluation-harness, HELM, Inspect AI, ...) has
// an adapter that implements Runner and registers itself via Register.
package runner

import (
	"context"
	"fmt"
	"sort"
	"sync"

	hkctx "github.com/iahsanGill/hanko/pkg/context"
)

// Runner invokes an evaluation harness and populates the run-time-only
// fields of a RunContext (results, harness version/commit, task version,
// backend version, CompletedAt, DurationSeconds).
//
// Adapters MUST treat rc's pre-populated fields (Model, Benchmark.Harness,
// Benchmark.Task, Runtime.*, Hardware.*) as inputs; modifying them is a
// contract violation.
type Runner interface {
	// Name returns the canonical harness identifier this Runner adapts —
	// matches Benchmark.Harness in the EvalRun predicate.
	Name() string

	// Run invokes the harness. On success the Runner has written every
	// field of rc that comes from the run (see package doc above).
	Run(ctx context.Context, rc *hkctx.RunContext, opts RunOptions) error
}

// RunOptions are the per-invocation knobs that aren't part of the EvalRun
// predicate — temporary paths, pass-through flags, and similar plumbing.
type RunOptions struct {
	// OutputDir is a writable directory where the harness drops its
	// results files. The caller is responsible for cleanup.
	OutputDir string

	// ExtraArgs is a pass-through list of additional flags appended to
	// the harness invocation verbatim. Use sparingly; anything that
	// affects the result should be captured in RunContext instead.
	ExtraArgs []string
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Runner)
)

// Register makes a Runner available under its Name(). Intended to be
// called from package init() of each adapter. Subsequent registrations
// under the same name overwrite the previous one; this is by design so
// tests can inject fakes.
func Register(r Runner) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[r.Name()] = r
}

// Get returns the registered Runner with the given name.
func Get(name string) (Runner, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	r, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown harness %q; registered: %v", name, registeredNames())
	}
	return r, nil
}

// registeredNames returns the sorted list of registered harness names.
// Caller must hold registryMu (read or write).
func registeredNames() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
