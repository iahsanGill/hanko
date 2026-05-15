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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseResultsBytes_MMLU(t *testing.T) {
	b, err := os.ReadFile("testdata/sample_results.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := parseResultsBytes(b, "mmlu")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if p.results.PrimaryMetric != "acc" {
		t.Errorf("PrimaryMetric: got %q, want %q", p.results.PrimaryMetric, "acc")
	}
	if p.results.PrimaryValue != 0.6826 {
		t.Errorf("PrimaryValue: got %v, want 0.6826", p.results.PrimaryValue)
	}
	if p.results.PrimaryStderr != 0.00404 {
		t.Errorf("PrimaryStderr: got %v, want 0.00404", p.results.PrimaryStderr)
	}
	if len(p.results.PerTask) != 2 {
		t.Errorf("PerTask should have 2 sub-tasks, got %d: %v", len(p.results.PerTask), p.results.PerTask)
	}
	if got := p.results.PerTask["mmlu_abstract_algebra"]; got != 0.42 {
		t.Errorf("PerTask[mmlu_abstract_algebra]: got %v, want 0.42", got)
	}

	if p.gitHash != "1a2b3c4d5e6f7890abcdef0123456789abcdef01" {
		t.Errorf("gitHash: got %q", p.gitHash)
	}
	if p.taskVersion != "2" {
		t.Errorf("taskVersion: got %q, want %q", p.taskVersion, "2")
	}
	if p.backendFromCfg != "vllm" {
		t.Errorf("backendFromCfg: got %q, want %q", p.backendFromCfg, "vllm")
	}
}

func TestParseResultsBytes_UnknownTask(t *testing.T) {
	b, err := os.ReadFile("testdata/sample_results.json")
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseResultsBytes(b, "definitely-not-a-task")
	if err == nil {
		t.Fatal("expected error for unknown task")
	}
	if !strings.Contains(err.Error(), "not present in results") {
		t.Errorf("error should mention 'not present in results': %v", err)
	}
}

func TestParseResultsBytes_MalformedJSON(t *testing.T) {
	_, err := parseResultsBytes([]byte("not-json"), "mmlu")
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestPickPrimaryMetric_Preference(t *testing.T) {
	// When both acc and exact_match are present, acc must win.
	r := map[string]any{
		"acc,none":         0.5,
		"acc_stderr,none":  0.01,
		"exact_match,none": 0.9,
		"alias":            "x",
	}
	m, v, _, err := pickPrimaryMetric(r)
	if err != nil {
		t.Fatal(err)
	}
	if m != "acc" || v != 0.5 {
		t.Errorf("pickPrimaryMetric: got (%q, %v), want (\"acc\", 0.5)", m, v)
	}
}

func TestPickPrimaryMetric_FallbackToFirstNumeric(t *testing.T) {
	r := map[string]any{
		"alias":              "x",
		"some_custom,none":   0.123,
		"some_custom_stderr": 0.01,
	}
	m, v, _, err := pickPrimaryMetric(r)
	if err != nil {
		t.Fatal(err)
	}
	if m != "some_custom" {
		t.Errorf("fallback metric: got %q, want %q", m, "some_custom")
	}
	if v != 0.123 {
		t.Errorf("fallback value: got %v, want 0.123", v)
	}
}

func TestFindResultsFile_PicksNewest(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "model_a")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	older := filepath.Join(sub, "results_2026-05-01T00-00-00.json")
	newer := filepath.Join(sub, "results_2026-05-16T12-00-00.json")
	for _, p := range []string{older, newer} {
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Force older mtime on the older file.
	past := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(older, past, past); err != nil {
		t.Fatal(err)
	}

	got, err := findResultsFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != newer {
		t.Errorf("findResultsFile: got %q, want %q", got, newer)
	}
}

func TestFindResultsFile_NoneFound(t *testing.T) {
	dir := t.TempDir()
	_, err := findResultsFile(dir)
	if err == nil {
		t.Fatal("expected error when no results file present")
	}
}
