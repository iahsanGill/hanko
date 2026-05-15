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

package determinism

import (
	"context"
	"errors"
	"strings"
	"testing"

	hkctx "github.com/iahsanGill/hanko/pkg/context"
	"github.com/iahsanGill/hanko/pkg/runner"
)

// fakeRunner produces a deterministic score on every invocation. Tests
// override the score field to script equality / disagreement scenarios.
// lastExtraArgs captures the most recent RunOptions.ExtraArgs slice so
// callers can assert the Limit-to-flag plumbing without coupling to
// any specific harness adapter.
type fakeRunner struct {
	score         float64
	failWith      error
	lastExtraArgs []string
}

func (f *fakeRunner) Name() string { return "fake-determinism" }
func (f *fakeRunner) Run(_ context.Context, rc *hkctx.RunContext, opts runner.RunOptions) error {
	f.lastExtraArgs = opts.ExtraArgs
	if f.failWith != nil {
		return f.failWith
	}
	rc.Results.PrimaryMetric = "acc"
	rc.Results.PrimaryValue = f.score
	return nil
}

func baseRC(score float64) *hkctx.RunContext {
	return &hkctx.RunContext{
		Benchmark: hkctx.BenchmarkRef{Harness: "fake-determinism", Task: "x"},
		Results:   hkctx.Results{PrimaryMetric: "acc", PrimaryValue: score},
	}
}

func TestVerifier_PassesOnMatchingScore(t *testing.T) {
	rc := baseRC(0.683)
	v := &Verifier{Runner: &fakeRunner{score: 0.683}}
	if err := v.Check(context.Background(), rc); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !rc.DeterminismCheck.Passed {
		t.Errorf("expected Passed=true, got Details=%q", rc.DeterminismCheck.Details)
	}
	if rc.DeterminismCheck.Method != hkctx.MethodDoubleRunScoreEqual {
		t.Errorf("Method: got %q", rc.DeterminismCheck.Method)
	}
	if rc.Results.PrimaryValue != 0.683 {
		t.Errorf("first-run score must be preserved; got %v", rc.Results.PrimaryValue)
	}
}

func TestVerifier_FailsOnDifferingScore(t *testing.T) {
	rc := baseRC(0.683)
	v := &Verifier{Runner: &fakeRunner{score: 0.700}}
	if err := v.Check(context.Background(), rc); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rc.DeterminismCheck.Passed {
		t.Errorf("expected Passed=false; details=%q", rc.DeterminismCheck.Details)
	}
	if !strings.Contains(rc.DeterminismCheck.Details, "0.6830000000") {
		t.Errorf("details should include original score: %q", rc.DeterminismCheck.Details)
	}
}

func TestVerifier_RecordsFailureWhenSecondRunErrors(t *testing.T) {
	rc := baseRC(0.683)
	sentinel := errors.New("simulated harness crash")
	v := &Verifier{Runner: &fakeRunner{failWith: sentinel}}
	err := v.Check(context.Background(), rc)
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel error, got %v", err)
	}
	if rc.DeterminismCheck.Passed {
		t.Errorf("Passed should be false when second run failed")
	}
	if !strings.Contains(rc.DeterminismCheck.Details, "second run failed") {
		t.Errorf("details should mention the failure: %q", rc.DeterminismCheck.Details)
	}
}

func TestVerifier_NilRunner(t *testing.T) {
	v := &Verifier{}
	err := v.Check(context.Background(), baseRC(0.5))
	if err == nil {
		t.Error("expected error for nil Runner")
	}
}
