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

// Package determinism implements hanko's optional "did this run reproduce
// byte-for-byte?" check. The strategy in v0.1 is double-run-score-equal:
// re-invoke the same runner with the same RunContext and assert the
// primary score is identical to floating-point precision.
//
// True byte-equality of model outputs requires that the harness expose
// per-question completions — lm-eval-harness can do this with the
// --log_samples flag, but plumbing that through is deferred to v0.2.
// Score-equality is a strict superset of "the same model produced the
// same evaluation answers", so it's a meaningful contract: if a run
// reports passed=true here, you can re-derive the score from the bundle.
package determinism

import (
	"context"
	"fmt"
	"math"
	"os"

	hkctx "github.com/iahsanGill/hanko/pkg/context"
	"github.com/iahsanGill/hanko/pkg/runner"
)

// epsilon is the floating-point tolerance for score-equality. We expect
// exact equality on truly deterministic runs; the tiny tolerance is here
// to absorb harness-side aggregation order changes that are not part of
// hanko's promise.
const epsilon = 1e-9

// Verifier re-runs an eval against a fresh OutputDir and compares the
// primary score to the first run's. A separate Verifier type (rather
// than a free function) lets tests inject a fake Runner without going
// through the package's adapter registry.
type Verifier struct {
	Runner runner.Runner
	// Limit, when > 0, caps the second run to the first Limit questions
	// of the task — useful when the full eval is too expensive to repeat
	// just to confirm determinism. Passed through as --limit N for
	// harnesses that recognize it.
	Limit int
}

// NewVerifier returns a Verifier wired to the registered Runner for the
// harness named in rc.Benchmark.Harness.
func NewVerifier(rc *hkctx.RunContext) (*Verifier, error) {
	r, err := runner.Get(rc.Benchmark.Harness)
	if err != nil {
		return nil, err
	}
	return &Verifier{Runner: r}, nil
}

// Check runs the harness a second time with a fresh OutputDir, asserts
// the primary score matches the original, and mutates rc.DeterminismCheck
// to record the outcome. It does not modify any other field of rc; the
// first-run results stay authoritative.
func (v *Verifier) Check(ctx context.Context, rc *hkctx.RunContext) error {
	if v.Runner == nil {
		return fmt.Errorf("determinism: nil Runner")
	}
	original := rc.Results.PrimaryValue

	// Build a sibling context that mirrors the first run's inputs but
	// has empty results. Re-running into the same rc would clobber the
	// first run's data, so we use a shallow copy.
	sibling := *rc
	sibling.Results = hkctx.Results{}

	tmp, err := os.MkdirTemp("", "hanko-determinism-*")
	if err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	opts := runner.RunOptions{OutputDir: tmp}
	if v.Limit > 0 {
		opts.ExtraArgs = []string{"--limit", fmt.Sprintf("%d", v.Limit)}
	}
	if err := v.Runner.Run(ctx, &sibling, opts); err != nil {
		// Surface the inner failure so users can distinguish "the second
		// run blew up" from "the second run disagreed".
		rc.DeterminismCheck = hkctx.DeterminismCheck{
			Performed: true,
			Method:    hkctx.MethodDoubleRunScoreEqual,
			Passed:    false,
			Details:   fmt.Sprintf("second run failed: %v", err),
		}
		return fmt.Errorf("second run: %w", err)
	}

	rerun := sibling.Results.PrimaryValue
	diff := math.Abs(rerun - original)
	rc.DeterminismCheck = hkctx.DeterminismCheck{
		Performed: true,
		Method:    hkctx.MethodDoubleRunScoreEqual,
		Passed:    diff <= epsilon,
		Details:   fmt.Sprintf("first=%.10f second=%.10f diff=%.3g", original, rerun, diff),
	}
	return nil
}
