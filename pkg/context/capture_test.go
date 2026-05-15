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

package context

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCapture_PopulatesFromOpts(t *testing.T) {
	rc := Capture(CaptureOptions{
		Model:        "meta-llama/Llama-3.1-8B",
		ModelSource:  "huggingface",
		Harness:      "lm-evaluation-harness",
		Task:         "mmlu",
		Seed:         42,
		BatchSize:    32,
		Temperature:  0.0,
		TopP:         1.0,
		Backend:      "vllm",
		HankoVersion: "0.0.1",
	})

	if rc.Model.Ref != "meta-llama/Llama-3.1-8B" {
		t.Errorf("Model.Ref: got %q", rc.Model.Ref)
	}
	if rc.Benchmark.Task != "mmlu" {
		t.Errorf("Benchmark.Task: got %q", rc.Benchmark.Task)
	}
	if rc.Runtime.Seed != 42 {
		t.Errorf("Runtime.Seed: got %d", rc.Runtime.Seed)
	}
	if rc.Hardware.CPU == "" {
		t.Error("Hardware.CPU should be populated by probe")
	}
	if rc.DeterminismCheck.Method != MethodSkipped {
		t.Errorf("DeterminismCheck.Method default: got %q, want %q",
			rc.DeterminismCheck.Method, MethodSkipped)
	}
	if rc.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
	if rc.HankoVersion != "0.0.1" {
		t.Errorf("HankoVersion: got %q", rc.HankoVersion)
	}
}

// TestCapture_JSONShape pins the field names against the predicate spec.
// If this test fails the on-wire schema has drifted from spec/eval-run-v1.md
// and the spec or the code must be updated to match.
func TestCapture_JSONShape(t *testing.T) {
	rc := Capture(CaptureOptions{
		Model:       "x",
		ModelSource: "local",
		Harness:     "lm-evaluation-harness",
		Task:        "mmlu",
		Backend:     "vllm",
	})
	b, err := json.Marshal(rc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)

	requiredKeys := []string{
		`"model"`, `"benchmark"`, `"runtime"`, `"hardware"`,
		`"results"`, `"determinismCheck"`,
		`"startedAt"`, `"completedAt"`, `"durationSeconds"`,
		`"hankoVersion"`,
		`"ref"`, `"source"`,
		`"harness"`, `"harnessCommit"`, `"task"`,
		`"seed"`, `"batchSize"`, `"temperature"`, `"topP"`,
		`"fpDeterminism"`, `"batchInvariantKernels"`,
		`"backend"`, `"backendVersion"`,
		`"primaryMetric"`, `"primaryValue"`,
		`"performed"`,
	}
	for _, k := range requiredKeys {
		if !strings.Contains(s, k) {
			t.Errorf("JSON missing required key %s in %s", k, s)
		}
	}
}

func TestEnvDefaultBool(t *testing.T) {
	tests := []struct {
		name string
		val  string
		set  bool
		def  bool
		want bool
	}{
		{"unset uses default true", "", false, true, true},
		{"unset uses default false", "", false, false, false},
		{"true literal", "true", true, false, true},
		{"1 literal", "1", true, false, true},
		{"false literal", "false", true, true, false},
		{"0 literal", "0", true, true, false},
		{"garbage uses default", "garbage", true, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := "HANKO_TEST_" + strings.ReplaceAll(tc.name, " ", "_")
			if tc.set {
				t.Setenv(key, tc.val)
			}
			if got := EnvDefaultBool(key, tc.def); got != tc.want {
				t.Errorf("EnvDefaultBool: got %v, want %v", got, tc.want)
			}
		})
	}
}
