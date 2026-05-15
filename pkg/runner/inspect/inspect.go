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

// Package inspect implements a Runner for UK AISI's Inspect AI evaluation
// harness (https://github.com/UKGovernmentBEIS/inspect_ai). It invokes
// the `inspect` CLI as a subprocess with `--log-format json`, parses the
// resulting EvalLog file, and populates a hanko RunContext. Shape mirrors
// pkg/runner/lmeval — same Runner contract, same testability seams — so
// adding a third harness is mechanical.
package inspect

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
// Mirrors the value spec/eval-run-v1.md anticipates for Benchmark.Harness
// and matches what `inspect --version`'s docstring calls the project.
const HarnessName = "inspect-ai"

// Adapter implements runner.Runner for Inspect AI.
type Adapter struct {
	command string
	exec    execer
}

// execer is the subprocess-call seam tests substitute. Same shape as
// lmeval's execer so a future generic harness driver can share it.
type execer interface {
	Run(ctx context.Context, name string, args []string) error
}

type osExecer struct{}

func (osExecer) Run(ctx context.Context, name string, args []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// New returns an Adapter that invokes the `inspect` binary on PATH.
func New() *Adapter {
	return &Adapter{
		command: "inspect",
		exec:    osExecer{},
	}
}

func init() {
	runner.Register(New())
}

// Name implements runner.Runner.
func (a *Adapter) Name() string { return HarnessName }

// Run implements runner.Runner. Invokes `inspect eval ... --log-format
// json --log-dir <out>`, locates the freshly written EvalLog JSON,
// parses it, and writes its findings back to rc:
//
//   - Results            ← primary scorer's metric + stderr + per-sample reductions
//   - Benchmark.HarnessCommit ← eval.revision.commit (cwd of the eval source)
//   - Benchmark.TaskVersion   ← eval.task_version
//   - Runtime.BackendVersion  ← "inspect_ai/<version>" from eval.packages
//   - Runtime.Backend         ← parsed from eval.model "provider/name" if caller didn't pin one
//   - CompletedAt / DurationSeconds
func (a *Adapter) Run(ctx context.Context, rc *hkctx.RunContext, opts runner.RunOptions) error {
	if opts.OutputDir == "" {
		return fmt.Errorf("inspect: RunOptions.OutputDir is required")
	}

	args := a.buildArgs(rc, opts)

	if err := a.exec.Run(ctx, a.command, args); err != nil {
		return fmt.Errorf("inspect: %s %v failed: %w", a.command, args, err)
	}

	logPath, err := findLogFile(opts.OutputDir)
	if err != nil {
		return fmt.Errorf("inspect: locate log: %w", err)
	}
	p, err := parseLogFile(logPath)
	if err != nil {
		return fmt.Errorf("inspect: %w", err)
	}

	rc.Results = p.results
	rc.Benchmark.HarnessCommit = p.gitCommit
	rc.Benchmark.TaskVersion = p.taskVersion
	if rc.Runtime.BackendVersion == "" && p.inspectVersion != "" {
		rc.Runtime.BackendVersion = "inspect_ai/" + p.inspectVersion
	}
	if rc.Runtime.Backend == "" && p.providerFromModel != "" {
		rc.Runtime.Backend = p.providerFromModel
	}
	rc.CompletedAt = time.Now().UTC()
	rc.DurationSeconds = int64(rc.CompletedAt.Sub(rc.StartedAt).Seconds())
	return nil
}

// buildArgs assembles the `inspect eval` invocation from a RunContext.
// Kept as a free-standing method so tests can pin the exact CLI shape.
//
// Mapping (RunContext field → inspect flag):
//
//	Benchmark.Task            → positional argument (the task spec)
//	Runtime.Backend + Model.Ref → --model <backend>/<model>
//	Runtime.Seed              → --seed
//	Runtime.Temperature       → --temperature
//	Runtime.TopP              → --top-p
//	OutputDir                 → --log-dir (with --log-format json)
//
// Per-batch / FP-determinism flags are env-controlled at the inference
// backend, not Inspect AI flags; they live in the RunContext but aren't
// surfaced on the CLI.
func (a *Adapter) buildArgs(rc *hkctx.RunContext, opts runner.RunOptions) []string {
	model := rc.Model.Ref
	// Inspect AI takes `provider/name` for --model. When caller has both
	// fields set, prefer that joined form; otherwise pass Model.Ref as-is
	// (the user may have already supplied a provider-qualified ref).
	if rc.Runtime.Backend != "" && !hasProviderPrefix(rc.Model.Ref) {
		model = rc.Runtime.Backend + "/" + rc.Model.Ref
	}

	args := []string{
		"eval", rc.Benchmark.Task,
		"--model", model,
		"--log-format", "json",
		"--log-dir", opts.OutputDir,
		"--seed", strconv.Itoa(rc.Runtime.Seed),
		"--temperature", strconv.FormatFloat(rc.Runtime.Temperature, 'f', -1, 64),
		"--top-p", strconv.FormatFloat(rc.Runtime.TopP, 'f', -1, 64),
	}
	args = append(args, opts.ExtraArgs...)
	return args
}

// hasProviderPrefix reports whether ref already carries an Inspect-style
// provider prefix (e.g. "openai/gpt-4o-mini"). A single slash anywhere
// is enough — false positives would mean a HuggingFace org/name ref like
// "meta-llama/Llama-3.1-8B" gets treated as already-prefixed, which is
// the correct behavior when the user passes such a value: Inspect's hf
// provider expects the slashed form.
func hasProviderPrefix(ref string) bool {
	for _, c := range ref {
		if c == '/' {
			return true
		}
	}
	return false
}
