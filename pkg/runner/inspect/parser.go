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
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	hkctx "github.com/iahsanGill/hanko/pkg/context"
)

// rawEvalLog mirrors the subset of UK AISI Inspect AI's EvalLog v2 JSON
// shape that we care about. Unknown fields are deliberately ignored so
// the parser stays forward-compatible with future Inspect releases.
//
// Reference: src/inspect_ai/log/_log.py in github.com/UKGovernmentBEIS/inspect_ai
// and the public schema at https://inspect.aisi.org.uk/eval-logs.html.
type rawEvalLog struct {
	Version int            `json:"version"`
	Status  string         `json:"status"`
	Eval    rawEval        `json:"eval"`
	Results *rawEvalResult `json:"results,omitempty"`
}

type rawEval struct {
	Task        string          `json:"task"`
	TaskID      string          `json:"task_id"`
	TaskVersion json.RawMessage `json:"task_version"` // int or string in the wild
	Model       string          `json:"model"`
	Revision    *rawRevision    `json:"revision,omitempty"`
	Packages    map[string]any  `json:"packages,omitempty"`
}

type rawRevision struct {
	Type   string `json:"type"`
	Origin string `json:"origin,omitempty"`
	Commit string `json:"commit,omitempty"`
}

type rawEvalResult struct {
	TotalSamples     int            `json:"total_samples"`
	CompletedSamples int            `json:"completed_samples"`
	Scores           []rawEvalScore `json:"scores"`
	Reductions       []rawReduction `json:"reductions,omitempty"`
}

type rawEvalScore struct {
	Name    string                   `json:"name"`
	Scorer  string                   `json:"scorer"`
	Reducer string                   `json:"reducer,omitempty"`
	Metrics map[string]rawEvalMetric `json:"metrics"`
}

type rawEvalMetric struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

// rawReduction carries the per-sample reduced scores Inspect writes when
// --no-log-samples isn't set. We don't currently surface per-sample data
// in the predicate, but we keep the type so a future predicate revision
// (`Results.PerSample`) can light up without re-parsing.
type rawReduction struct {
	Scorer  string `json:"scorer"`
	Reducer string `json:"reducer"`
}

// parsed is the harness-agnostic extraction the adapter merges into a
// RunContext. Splitting parse and merge keeps the parser pure: no
// context mutation, no clock, no filesystem side effects beyond reading
// the file passed in.
type parsed struct {
	results           hkctx.Results
	gitCommit         string
	taskVersion       string
	inspectVersion    string
	providerFromModel string
}

// findLogFile locates the most-recently-modified Inspect AI EvalLog file
// under dir. Inspect writes files as `<ts>_<task>_<id>.json` by default,
// but the layout is configurable; walking + picking newest is more
// robust than a hardcoded glob.
func findLogFile(dir string) (string, error) {
	var found string
	var foundMod int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if mod := info.ModTime().UnixNano(); mod > foundMod {
			found = path
			foundMod = mod
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk %s: %w", dir, err)
	}
	if found == "" {
		return "", fmt.Errorf("no Inspect AI .json log found under %s", dir)
	}
	return found, nil
}

// parseLogFile reads and decodes an EvalLog JSON file, picking the
// primary metric off the first scorer block.
func parseLogFile(path string) (*parsed, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parseLogBytes(b)
}

// parseLogBytes is the pure half of parseLogFile — factored out so
// tests can drive it with in-memory fixtures.
func parseLogBytes(b []byte) (*parsed, error) {
	var raw rawEvalLog
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("decode eval log: %w", err)
	}

	if raw.Status != "" && raw.Status != "success" {
		return nil, fmt.Errorf("eval status %q (want success); refusing to record an incomplete run", raw.Status)
	}
	if raw.Results == nil || len(raw.Results.Scores) == 0 {
		return nil, fmt.Errorf("eval log carries no results.scores; nothing to record")
	}

	primary := raw.Results.Scores[0]
	metricName, value, stderr, err := pickPrimaryMetric(primary)
	if err != nil {
		return nil, fmt.Errorf("scorer %q: %w", primary.Scorer, err)
	}

	p := &parsed{
		results: hkctx.Results{
			PrimaryMetric: metricName,
			PrimaryValue:  value,
			PrimaryStderr: stderr,
		},
		taskVersion:       jsonRawToString(raw.Eval.TaskVersion),
		providerFromModel: providerFrom(raw.Eval.Model),
	}
	if raw.Eval.Revision != nil {
		p.gitCommit = raw.Eval.Revision.Commit
	}
	if v, ok := raw.Eval.Packages["inspect_ai"]; ok {
		p.inspectVersion = fmt.Sprint(v)
	}
	return p, nil
}

// pickPrimaryMetric extracts the headline metric from a scorer's
// `metrics` map. Inspect AI emits metrics like
// `{"accuracy": {value: 0.72}, "stderr": {value: 0.045}}` — no
// comma-suffix lm-eval gymnastics.
//
// Preference order: accuracy > mean > exact_match > pass@1 > the first
// numeric key that isn't `stderr`. `stderr` is always pulled out into
// its own field when present.
func pickPrimaryMetric(s rawEvalScore) (string, float64, float64, error) {
	preferred := []string{"accuracy", "mean", "exact_match", "pass@1", "f1", "rouge1"}
	var primaryKey string
	for _, k := range preferred {
		if _, ok := s.Metrics[k]; ok {
			primaryKey = k
			break
		}
	}
	if primaryKey == "" {
		// Fallback: alphabetic-first non-stderr key so the choice is
		// deterministic across runs.
		keys := make([]string, 0, len(s.Metrics))
		for k := range s.Metrics {
			if k == "stderr" {
				continue
			}
			keys = append(keys, k)
		}
		if len(keys) == 0 {
			return "", 0, 0, fmt.Errorf("scorer has only stderr (no primary metric)")
		}
		sort.Strings(keys)
		primaryKey = keys[0]
	}
	value := s.Metrics[primaryKey].Value
	stderr := s.Metrics["stderr"].Value
	return primaryKey, value, stderr, nil
}

// providerFrom returns the part of an Inspect AI model spec before the
// first slash — the provider name (e.g. "openai" from
// "openai/gpt-4o-mini"). Returns "" when there's no slash, which means
// the user passed an unqualified ref Inspect would default-route.
func providerFrom(model string) string {
	if i := strings.IndexByte(model, '/'); i > 0 {
		return model[:i]
	}
	return ""
}

// jsonRawToString collapses an "int or string" JSON field into its
// canonical string form. Inspect AI writes `task_version` as a JSON
// integer for built-in tasks and as a string for user-supplied custom
// versions; both round-trip cleanly into the predicate's string field.
func jsonRawToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Strip JSON string quoting; for numbers, just return the literal.
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
