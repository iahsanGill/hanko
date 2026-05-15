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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseLogBytes_PicksAccuracyAndStderr(t *testing.T) {
	b, err := os.ReadFile("testdata/sample_eval.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := parseLogBytes(b)
	if err != nil {
		t.Fatalf("parseLogBytes: %v", err)
	}
	if got, want := p.results.PrimaryMetric, "accuracy"; got != want {
		t.Errorf("PrimaryMetric: got %q want %q", got, want)
	}
	if got, want := p.results.PrimaryValue, 0.72; got != want {
		t.Errorf("PrimaryValue: got %v want %v", got, want)
	}
	if got, want := p.results.PrimaryStderr, 0.045; got != want {
		t.Errorf("PrimaryStderr: got %v want %v", got, want)
	}
	if got, want := p.gitCommit, "5f3a9c1d2e8a7b6c4d3e2f1a0b9c8d7e6f5a4b3c"; got != want {
		t.Errorf("gitCommit: got %q want %q", got, want)
	}
	if got, want := p.inspectVersion, "0.3.135"; got != want {
		t.Errorf("inspectVersion: got %q want %q", got, want)
	}
	if got, want := p.providerFromModel, "openai"; got != want {
		t.Errorf("providerFromModel: got %q want %q", got, want)
	}
}

func TestParseLogBytes_RejectsIncompleteRun(t *testing.T) {
	body := []byte(`{"version":2,"status":"error","eval":{"task":"x"}}`)
	_, err := parseLogBytes(body)
	if err == nil {
		t.Fatal("expected error for status != success")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("error should mention status: %v", err)
	}
}

func TestParseLogBytes_RejectsMissingScores(t *testing.T) {
	body := []byte(`{"version":2,"status":"success","eval":{"task":"x"},"results":{"scores":[]}}`)
	_, err := parseLogBytes(body)
	if err == nil {
		t.Fatal("expected error for empty results.scores")
	}
}

// TestPickPrimaryMetric_Preference exercises the fallback chain on a
// hand-rolled scorer that omits `accuracy` and `mean` to make sure the
// next-preferred metric wins. Belt-and-braces over the fixture, which
// only exercises the top of the preference list.
func TestPickPrimaryMetric_Preference(t *testing.T) {
	cases := []struct {
		name    string
		metrics map[string]rawEvalMetric
		want    string
	}{
		{"mean preferred over exact_match", map[string]rawEvalMetric{
			"mean":        {Value: 0.5},
			"exact_match": {Value: 0.4},
		}, "mean"},
		{"alphabetic fallback when none preferred", map[string]rawEvalMetric{
			"weird_metric_b": {Value: 0.9},
			"weird_metric_a": {Value: 0.8},
		}, "weird_metric_a"},
		{"stderr never primary even when alone-eligible", map[string]rawEvalMetric{
			"stderr":   {Value: 0.01},
			"my_score": {Value: 0.7},
		}, "my_score"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, _, _, err := pickPrimaryMetric(rawEvalScore{Metrics: tc.metrics})
			if err != nil {
				t.Fatalf("pickPrimaryMetric: %v", err)
			}
			if name != tc.want {
				t.Errorf("got %q want %q", name, tc.want)
			}
		})
	}
}

// TestFindLogFile_PicksNewest puts two .json files in a temp dir and
// confirms the parser's "most-recent" heuristic resolves to the right
// one — defends against a future Inspect filename change reshuffling
// timestamps relative to filename order.
func TestFindLogFile_PicksNewest(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "z_first.json")
	newer := filepath.Join(dir, "a_second.json")
	if err := os.WriteFile(older, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Bump the newer file's mtime forward; WriteFile order on fast disks
	// can give them identical timestamps otherwise.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(newer, future, future); err != nil {
		t.Fatal(err)
	}

	got, err := findLogFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != newer {
		t.Errorf("findLogFile: got %q want %q", got, newer)
	}
}

func TestFindLogFile_ErrorsWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	_, err := findLogFile(dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func TestProviderFrom(t *testing.T) {
	cases := map[string]string{
		"openai/gpt-4o-mini":          "openai",
		"anthropic/claude-3-5-sonnet": "anthropic",
		"meta-llama/Llama-3.1-8B":     "meta-llama",
		"unqualified-model":           "",
		"":                            "",
	}
	for in, want := range cases {
		if got := providerFrom(in); got != want {
			t.Errorf("providerFrom(%q) = %q want %q", in, got, want)
		}
	}
}

func TestJSONRawToString(t *testing.T) {
	cases := map[string]string{
		`0`:      "0",
		`"v2.0"`: "v2.0",
		``:       "",
		`null`:   "null",
	}
	for in, want := range cases {
		if got := jsonRawToString([]byte(in)); got != want {
			t.Errorf("jsonRawToString(%q) = %q want %q", in, got, want)
		}
	}
}
