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
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	hkctx "github.com/iahsanGill/hanko/pkg/context"
)

// rawResults mirrors the on-disk shape of lm-evaluation-harness results.
// We only decode the fields we need; unknown fields are ignored so the
// parser is forward-compatible with harness additions.
type rawResults struct {
	Results  map[string]map[string]any `json:"results"`
	Versions map[string]any            `json:"versions"`
	Config   struct {
		Model     string `json:"model"`
		ModelArgs string `json:"model_args"`
		BatchSize any    `json:"batch_size"`
	} `json:"config"`
	GitHash             string  `json:"git_hash"`
	Date                float64 `json:"date"`
	TransformersVersion string  `json:"transformers_version"`
}

// parsed holds the subset of fields we map into a RunContext, plus the
// hash of the harness commit. Splitting parse and merge keeps the parser
// pure: no RunContext mutation, no clock, no filesystem side effects.
type parsed struct {
	results        hkctx.Results
	gitHash        string
	taskVersion    string
	transformers   string
	backendFromCfg string
}

// findResultsFile locates the most-recently-written `results_*.json`
// under dir, walking subdirectories. lm-eval-harness writes results under
// `<output_path>/<model_dir>/results_<timestamp>.json`, so a recursive
// search is more robust than hardcoding a layout.
func findResultsFile(dir string) (string, error) {
	var found string
	var foundMod int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "results_") || !strings.HasSuffix(name, ".json") {
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
		return "", fmt.Errorf("no results_*.json file found under %s", dir)
	}
	return found, nil
}

// parseResultsFile reads and decodes a results_*.json file produced by
// lm-evaluation-harness, returning a parsed view scoped to the requested
// task. The task is required because lm-eval emits results for sub-tasks
// alongside the parent task (e.g. mmlu plus mmlu_abstract_algebra) and
// we want the parent's primary score as the headline.
func parseResultsFile(path, task string) (*parsed, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parseResultsBytes(b, task)
}

// parseResultsBytes is the pure-function half of parseResultsFile —
// factored out so tests can drive it with in-memory fixtures.
func parseResultsBytes(b []byte, task string) (*parsed, error) {
	var raw rawResults
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("decode results: %w", err)
	}

	taskRes, ok := raw.Results[task]
	if !ok {
		return nil, fmt.Errorf("task %q not present in results (have: %v)", task, sortedKeys(raw.Results))
	}

	metric, primary, stderr, err := pickPrimaryMetric(taskRes)
	if err != nil {
		return nil, fmt.Errorf("task %q: %w", task, err)
	}

	perTask := make(map[string]float64)
	for k, v := range raw.Results {
		if k == task {
			continue
		}
		// Only include sub-tasks (prefix match) so we don't carry
		// unrelated tasks if the user ran several at once.
		if !strings.HasPrefix(k, task+"_") {
			continue
		}
		_, val, _, err := pickPrimaryMetric(v)
		if err != nil {
			// Skip sub-tasks whose metric we can't identify rather
			// than fail the whole parse — caller still gets the
			// parent's primary score.
			continue
		}
		perTask[k] = val
	}

	p := &parsed{
		results: hkctx.Results{
			PrimaryMetric: metric,
			PrimaryValue:  primary,
			PrimaryStderr: stderr,
			PerTask:       perTask,
		},
		gitHash:        raw.GitHash,
		backendFromCfg: raw.Config.Model,
		transformers:   raw.TransformersVersion,
	}
	if v, ok := raw.Versions[task]; ok {
		p.taskVersion = fmt.Sprint(v)
	}
	return p, nil
}

// pickPrimaryMetric extracts the headline metric from a task's results
// map. lm-eval emits metrics as `<metric>,<filter>` keys where filter is
// usually `none`; we strip the filter for the predicate's PrimaryMetric.
// Preference order: acc > exact_match > pass@1 > the first non-stderr,
// non-alias key.
func pickPrimaryMetric(taskRes map[string]any) (string, float64, float64, error) {
	preferred := []string{"acc", "exact_match", "pass@1", "f1", "rouge1"}
	for _, name := range preferred {
		for _, suffix := range []string{",none", ""} {
			if v, ok := taskRes[name+suffix]; ok {
				if f, ok := asFloat(v); ok {
					stderr, _ := asFloat(taskRes[name+"_stderr"+suffix])
					return name, f, stderr, nil
				}
			}
		}
	}
	// Fallback: first numeric key that isn't an stderr or alias.
	for k, v := range taskRes {
		if k == "alias" || strings.Contains(k, "stderr") {
			continue
		}
		if f, ok := asFloat(v); ok {
			metric := k
			if idx := strings.Index(metric, ","); idx >= 0 {
				metric = metric[:idx]
			}
			return metric, f, 0, nil
		}
	}
	return "", 0, 0, fmt.Errorf("no numeric metric found in task results")
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func sortedKeys(m map[string]map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Keep deterministic order so error messages compare cleanly in tests.
	stringsSort(out)
	return out
}

// stringsSort is a tiny indirection that lets the parser file avoid an
// extra import for one call site. Equivalent to sort.Strings.
func stringsSort(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
