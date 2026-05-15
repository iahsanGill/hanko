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

package lmeval

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

// fakeExecer simulates an lm_eval invocation by copying a fixture JSON
// into the OutputDir the adapter passed via --output_path. This lets us
// exercise the full Run() flow without Python, GPUs, or network.
type fakeExecer struct {
	fixture     string // path to fixture JSON to drop in OutputDir
	subdir      string // optional subdir under OutputDir (mirrors lm-eval layout)
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
	out := outputPathFromArgs(args)
	if out == "" {
		return errors.New("fakeExecer: --output_path missing from args")
	}
	dest := out
	if f.subdir != "" {
		dest = filepath.Join(out, f.subdir)
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
	}
	src, err := os.ReadFile(f.fixture)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, "results_2026-05-16T12-00-00.json"), src, 0o644)
}

func outputPathFromArgs(args []string) string {
	for i, a := range args {
		if a == "--output_path" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func newTestRC() *hkctx.RunContext {
	return &hkctx.RunContext{
		Model: hkctx.ModelRef{
			Ref:    "meta-llama/Llama-3.1-8B",
			Source: "huggingface",
		},
		Benchmark: hkctx.BenchmarkRef{
			Harness: HarnessName,
			Task:    "mmlu",
		},
		Runtime: hkctx.RuntimeConfig{
			Seed:        42,
			BatchSize:   32,
			Temperature: 0.0,
			TopP:        1.0,
			Backend:     "vllm",
		},
		StartedAt: time.Now().UTC().Add(-2 * time.Minute),
	}
}

func TestAdapter_Run_PopulatesContext(t *testing.T) {
	rc := newTestRC()
	outDir := t.TempDir()

	fe := &fakeExecer{
		fixture: "testdata/sample_results.json",
		subdir:  "model_a",
	}
	a := &Adapter{command: "lm_eval", exec: fe}

	err := a.Run(context.Background(), rc, runner.RunOptions{OutputDir: outDir})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if rc.Results.PrimaryMetric != "acc" {
		t.Errorf("PrimaryMetric: got %q", rc.Results.PrimaryMetric)
	}
	if rc.Results.PrimaryValue != 0.6826 {
		t.Errorf("PrimaryValue: got %v", rc.Results.PrimaryValue)
	}
	if rc.Benchmark.HarnessCommit != "1a2b3c4d5e6f7890abcdef0123456789abcdef01" {
		t.Errorf("HarnessCommit: got %q", rc.Benchmark.HarnessCommit)
	}
	if rc.Benchmark.TaskVersion != "2" {
		t.Errorf("TaskVersion: got %q", rc.Benchmark.TaskVersion)
	}
	if rc.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set after Run")
	}
	if rc.DurationSeconds <= 0 {
		t.Errorf("DurationSeconds: got %d, want > 0", rc.DurationSeconds)
	}
}

func TestAdapter_Run_PassesBackendVerbatim(t *testing.T) {
	rc := newTestRC()
	rc.Runtime.Backend = "vllm"
	outDir := t.TempDir()

	fe := &fakeExecer{
		fixture: "testdata/sample_results.json",
	}
	a := &Adapter{command: "lm_eval", exec: fe}

	if err := a.Run(context.Background(), rc, runner.RunOptions{OutputDir: outDir}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Caller-pinned Runtime.Backend must survive the harness's own
	// `config.model` value (which the parser would otherwise apply).
	if rc.Runtime.Backend != "vllm" {
		t.Errorf("Runtime.Backend was overwritten: got %q", rc.Runtime.Backend)
	}
}

func TestAdapter_buildArgs(t *testing.T) {
	rc := newTestRC()
	rc.Runtime.Temperature = 0.7 // force --gen_kwargs path
	a := &Adapter{}
	args := a.buildArgs(rc, runner.RunOptions{
		OutputDir: "/tmp/x",
		ExtraArgs: []string{"--limit", "10"},
	})

	want := []string{
		"--model", "vllm",
		"--model_args", "pretrained=meta-llama/Llama-3.1-8B",
		"--tasks", "mmlu",
		"--batch_size", "32",
		"--seed", "42",
		"--output_path", "/tmp/x",
		"--gen_kwargs", "temperature=0.7,top_p=1",
		"--limit", "10",
	}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("buildArgs mismatch.\n got: %v\nwant: %v", args, want)
	}
}

func TestAdapter_Run_RequiresOutputDir(t *testing.T) {
	rc := newTestRC()
	a := &Adapter{command: "lm_eval", exec: &fakeExecer{fixture: "testdata/sample_results.json"}}
	err := a.Run(context.Background(), rc, runner.RunOptions{})
	if err == nil {
		t.Fatal("expected error when OutputDir is empty")
	}
}

func TestAdapter_Run_PropagatesExecError(t *testing.T) {
	rc := newTestRC()
	want := errors.New("simulated harness crash")
	fe := &fakeExecer{returnError: want}
	a := &Adapter{command: "lm_eval", exec: fe}

	err := a.Run(context.Background(), rc, runner.RunOptions{OutputDir: t.TempDir()})
	if err == nil || !errors.Is(err, want) {
		t.Errorf("expected wrapped exec error; got %v", err)
	}
}

func TestAdapter_RegisteredOnInit(t *testing.T) {
	r, err := runner.Get(HarnessName)
	if err != nil {
		t.Fatalf("HarnessName not registered: %v", err)
	}
	if r.Name() != HarnessName {
		t.Errorf("Name(): got %q", r.Name())
	}
}
