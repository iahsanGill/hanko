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

// Package lmeval implements a Runner for EleutherAI/lm-evaluation-harness.
// It invokes the `lm_eval` CLI as a subprocess, parses the resulting JSON,
// and populates a hanko RunContext.
package lmeval

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	hkctx "github.com/iahsanGill/hanko/pkg/context"
	"github.com/iahsanGill/hanko/pkg/runner"
)

// HarnessName is the canonical identifier this adapter registers under.
// Matches the spec/eval-run-v1.md value for Benchmark.Harness.
const HarnessName = "lm-evaluation-harness"

// Adapter implements runner.Runner for lm-evaluation-harness.
type Adapter struct {
	command string
	exec    execer
}

// execer abstracts the subprocess call surface so tests can substitute a
// fake that simulates a successful (or failing) lm_eval run without
// requiring Python or the harness to be installed.
type execer interface {
	Run(ctx context.Context, name string, args []string) error
}

type osExecer struct{}

func (osExecer) Run(ctx context.Context, name string, args []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	// Pass through stdout/stderr so the user sees harness progress in
	// real time; the result file is the authoritative output anyway.
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// New returns an Adapter that invokes the `lm_eval` binary on PATH.
func New() *Adapter {
	return &Adapter{
		command: "lm_eval",
		exec:    osExecer{},
	}
}

func init() {
	runner.Register(New())
}

// Name implements runner.Runner.
func (a *Adapter) Name() string { return HarnessName }

// Run implements runner.Runner. It invokes lm_eval, parses the resulting
// JSON, and populates the fields of rc that come from the harness:
// Results, Benchmark.HarnessCommit, Benchmark.TaskVersion, Runtime.Backend
// (when not already set), CompletedAt, DurationSeconds.
func (a *Adapter) Run(ctx context.Context, rc *hkctx.RunContext, opts runner.RunOptions) error {
	if opts.OutputDir == "" {
		return fmt.Errorf("lmeval: RunOptions.OutputDir is required")
	}

	args := a.buildArgs(rc, opts)

	if err := a.exec.Run(ctx, a.command, args); err != nil {
		return fmt.Errorf("lmeval: %s %v failed: %w", a.command, args, err)
	}

	resultsPath, err := findResultsFile(opts.OutputDir)
	if err != nil {
		return fmt.Errorf("lmeval: locate results: %w", err)
	}
	p, err := parseResultsFile(resultsPath, rc.Benchmark.Task)
	if err != nil {
		return fmt.Errorf("lmeval: %w", err)
	}

	rc.Results = p.results
	rc.Benchmark.HarnessCommit = p.gitHash
	rc.Benchmark.TaskVersion = p.taskVersion
	// Only override Runtime.Backend if the caller didn't pin one — the
	// harness records its own `model` field which can disagree if the
	// user invoked with a different backend than they specified.
	if rc.Runtime.Backend == "" {
		rc.Runtime.Backend = p.backendFromCfg
	}
	rc.CompletedAt = time.Now().UTC()
	rc.DurationSeconds = int64(rc.CompletedAt.Sub(rc.StartedAt).Seconds())
	return nil
}

// buildArgs assembles the lm_eval CLI invocation from a RunContext and
// RunOptions. Kept as a separate function so it's directly testable.
//
// Mapping (RunContext field → lm_eval flag):
//
//	Runtime.Backend          → --model
//	Model.Ref                → --model_args pretrained=...
//	Benchmark.Task           → --tasks
//	Runtime.BatchSize        → --batch_size
//	Runtime.Seed             → --seed
//	OutputDir                → --output_path
//
// Sampling parameters (temperature, top_p) are passed through --gen_kwargs.
// FP-determinism and batch-invariance flags are env-controlled at the
// backend level (vLLM/SGLang), not lm_eval flags; they're recorded in
// RunContext but not added to the CLI.
func (a *Adapter) buildArgs(rc *hkctx.RunContext, opts runner.RunOptions) []string {
	args := []string{
		"--model", rc.Runtime.Backend,
		"--model_args", "pretrained=" + rc.Model.Ref,
		"--tasks", rc.Benchmark.Task,
		"--batch_size", strconv.Itoa(rc.Runtime.BatchSize),
		"--seed", strconv.Itoa(rc.Runtime.Seed),
		"--output_path", opts.OutputDir,
	}
	if rc.Runtime.Temperature != 0 || rc.Runtime.TopP != 1 {
		args = append(args, "--gen_kwargs",
			fmt.Sprintf("temperature=%g,top_p=%g", rc.Runtime.Temperature, rc.Runtime.TopP))
	}
	args = append(args, opts.ExtraArgs...)
	return args
}
