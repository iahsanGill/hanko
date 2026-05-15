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

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRun_DryRun_EmitsContext(t *testing.T) {
	root := Root()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{
		"run",
		"--model", "meta-llama/Llama-3.1-8B",
		"--task", "mmlu",
		"--dry-run",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v\nstderr: %s", err, stderr.String())
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("dry-run output is not valid JSON: %v\noutput: %s", err, stdout.String())
	}

	model, _ := got["model"].(map[string]any)
	if model["ref"] != "meta-llama/Llama-3.1-8B" {
		t.Errorf("model.ref: got %v", model["ref"])
	}
	bench, _ := got["benchmark"].(map[string]any)
	if bench["task"] != "mmlu" {
		t.Errorf("benchmark.task: got %v", bench["task"])
	}
}

func TestRun_NoDryRun_NotImplemented(t *testing.T) {
	root := Root()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{
		"run",
		"--model", "m",
		"--task", "t",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error from non-dry-run, got nil")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("error should signal not-implemented; got: %v", err)
	}
}

func TestRun_RequiresModelAndTask(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"missing both", []string{"run", "--dry-run"}},
		{"missing task", []string{"run", "--model", "m", "--dry-run"}},
		{"missing model", []string{"run", "--task", "t", "--dry-run"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := Root()
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs(tc.args)
			if err := root.Execute(); err == nil {
				t.Error("expected error for missing required flag, got nil")
			}
		})
	}
}
