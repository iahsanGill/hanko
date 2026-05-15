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

package inspect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	hkctx "github.com/iahsanGill/hanko/pkg/context"
	"github.com/iahsanGill/hanko/pkg/runner"
)

// fakeExecer simulates `inspect eval` by copying a fixture EvalLog into
// the --log-dir the adapter passed. Lets us exercise the full Run() flow
// without Python, an API key, or network.
type fakeExecer struct {
	fixture     string
	returnError error
	gotName     string
	gotArgs     []string
}

func (f *fakeExecer) Run(_ context.Context, name string, args []string) error {
	f.gotName = name
	f.gotArgs = args
	if f.returnError != nil {
		return f.returnError
	}
	logDir := logDirFromArgs(args)
	if logDir == "" {
		return errors.New("fakeExecer: --log-dir missing from args")
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	src, err := os.ReadFile(f.fixture)
	if err != nil {
		return err
	}
	// Mirror the default Inspect filename pattern; the parser glob is
	// `*.json` so the exact name doesn't matter, but we pick a realistic
	// one so a future filename-aware test stays honest.
	return os.WriteFile(filepath.Join(logDir, "2026-05-16T10-14-33_mmlu_abc123.json"), src, 0o644)
}

func logDirFromArgs(args []string) string {
	for i, a := range args {
		if a == "--log-dir" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func newTestRC() *hkctx.RunContext {
	return &hkctx.RunContext{
		Model: hkctx.ModelRef{
			Ref:    "gpt-4o-mini",
			Source: "openai",
		},
		Benchmark: hkctx.BenchmarkRef{
			Harness: HarnessName,
			Task:    "inspect_evals/mmlu",
		},
		Runtime: hkctx.RuntimeConfig{
			Seed:        42,
			BatchSize:   32,
			Temperature: 0.0,
			TopP:        1.0,
			Backend:     "openai",
		},
		StartedAt: time.Now().UTC().Add(-30 * time.Second),
	}
}

func TestAdapter_Run_PopulatesContext(t *testing.T) {
	rc := newTestRC()
	outDir := t.TempDir()

	fe := &fakeExecer{fixture: "testdata/sample_eval.json"}
	a := &Adapter{command: "inspect", exec: fe}

	if err := a.Run(context.Background(), rc, runner.RunOptions{OutputDir: outDir}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got, want := rc.Results.PrimaryMetric, "accuracy"; got != want {
		t.Errorf("PrimaryMetric: got %q want %q", got, want)
	}
	if got, want := rc.Results.PrimaryValue, 0.72; got != want {
		t.Errorf("PrimaryValue: got %v want %v", got, want)
	}
	if got, want := rc.Results.PrimaryStderr, 0.045; got != want {
		t.Errorf("PrimaryStderr: got %v want %v", got, want)
	}
	if got, want := rc.Benchmark.HarnessCommit, "5f3a9c1d2e8a7b6c4d3e2f1a0b9c8d7e6f5a4b3c"; got != want {
		t.Errorf("HarnessCommit: got %q want %q", got, want)
	}
	if got, want := rc.Benchmark.TaskVersion, "0"; got != want {
		t.Errorf("TaskVersion: got %q want %q", got, want)
	}
	if got, want := rc.Runtime.BackendVersion, "inspect_ai/0.3.135"; got != want {
		t.Errorf("BackendVersion: got %q want %q", got, want)
	}
	if rc.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set after Run")
	}
}

func TestAdapter_Run_DoesntOverwritePinnedBackendVersion(t *testing.T) {
	rc := newTestRC()
	rc.Runtime.BackendVersion = "operator-supplied/1.2.3"
	outDir := t.TempDir()

	a := &Adapter{command: "inspect", exec: &fakeExecer{fixture: "testdata/sample_eval.json"}}
	if err := a.Run(context.Background(), rc, runner.RunOptions{OutputDir: outDir}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rc.Runtime.BackendVersion != "operator-supplied/1.2.3" {
		t.Errorf("Run() overwrote operator-pinned BackendVersion: %q", rc.Runtime.BackendVersion)
	}
}

func TestAdapter_buildArgs_JoinsProviderWhenMissing(t *testing.T) {
	rc := newTestRC()
	rc.Model.Ref = "gpt-4o-mini" // no slash → adapter must prepend backend
	a := &Adapter{}
	args := a.buildArgs(rc, runner.RunOptions{
		OutputDir: "/tmp/x",
		ExtraArgs: []string{"--limit", "10"},
	})

	want := []string{
		"eval", "inspect_evals/mmlu",
		"--model", "openai/gpt-4o-mini",
		"--log-format", "json",
		"--log-dir", "/tmp/x",
		"--seed", "42",
		"--temperature", "0",
		"--top-p", "1",
		"--limit", "10",
	}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("buildArgs mismatch.\n got: %v\nwant: %v", args, want)
	}
}

func TestAdapter_buildArgs_PreservesAlreadyQualifiedModel(t *testing.T) {
	rc := newTestRC()
	rc.Model.Ref = "anthropic/claude-3-5-sonnet" // already slashed → adapter must NOT double-prefix
	rc.Runtime.Backend = "openai"                // and must NOT swap the provider
	a := &Adapter{}
	args := a.buildArgs(rc, runner.RunOptions{OutputDir: "/tmp/x"})

	for i, a := range args {
		if a == "--model" {
			if got, want := args[i+1], "anthropic/claude-3-5-sonnet"; got != want {
				t.Errorf("--model: got %q want %q", got, want)
			}
			return
		}
	}
	t.Fatal("--model flag absent from buildArgs output")
}

func TestAdapter_Run_RequiresOutputDir(t *testing.T) {
	rc := newTestRC()
	a := &Adapter{command: "inspect", exec: &fakeExecer{fixture: "testdata/sample_eval.json"}}
	err := a.Run(context.Background(), rc, runner.RunOptions{})
	if err == nil {
		t.Fatal("expected error when OutputDir is empty")
	}
}

func TestAdapter_Run_PropagatesExecError(t *testing.T) {
	rc := newTestRC()
	want := errors.New("simulated inspect crash")
	a := &Adapter{command: "inspect", exec: &fakeExecer{returnError: want}}

	err := a.Run(context.Background(), rc, runner.RunOptions{OutputDir: t.TempDir()})
	if err == nil || !errors.Is(err, want) {
		t.Errorf("expected wrapped exec error; got %v", err)
	}
}

func TestAdapter_RegisteredOnInit(t *testing.T) {
	r, err := runner.Get(HarnessName)
	if err != nil {
		t.Fatalf("HarnessName %q not registered: %v", HarnessName, err)
	}
	if r.Name() != HarnessName {
		t.Errorf("Name(): got %q want %q", r.Name(), HarnessName)
	}
}
