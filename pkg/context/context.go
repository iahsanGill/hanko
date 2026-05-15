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

// Package context captures the canonical run context of an evaluation —
// every input that could affect the result, pinned to a content hash where
// possible. The captured context is the input to the EvalRun predicate
// (see spec/eval-run-v1.md).
package context

import "time"

// RunContext is the full set of inputs that bound an eval run. It maps
// 1:1 to the EvalRun v1 predicate; see spec/eval-run-v1.md for field
// semantics.
type RunContext struct {
	Model            ModelRef         `json:"model"`
	Benchmark        BenchmarkRef     `json:"benchmark"`
	Runtime          RuntimeConfig    `json:"runtime"`
	Hardware         HardwareInfo     `json:"hardware"`
	Results          Results          `json:"results"`
	DeterminismCheck DeterminismCheck `json:"determinismCheck"`
	StartedAt        time.Time        `json:"startedAt"`
	CompletedAt      time.Time        `json:"completedAt"`
	DurationSeconds  int64            `json:"durationSeconds"`
	HankoVersion     string           `json:"hankoVersion"`
}

// ModelRef pins the model under evaluation.
type ModelRef struct {
	Ref    string `json:"ref"`
	Digest string `json:"digest,omitempty"`
	Source string `json:"source"`
}

// BenchmarkRef pins the eval harness and the task within it.
type BenchmarkRef struct {
	Harness        string `json:"harness"`
	HarnessVersion string `json:"harnessVersion,omitempty"`
	HarnessCommit  string `json:"harnessCommit"`
	Task           string `json:"task"`
	TaskVersion    string `json:"taskVersion,omitempty"`
}

// RuntimeConfig captures every runtime knob that can affect output.
type RuntimeConfig struct {
	Seed                  int     `json:"seed"`
	BatchSize             int     `json:"batchSize"`
	Temperature           float64 `json:"temperature"`
	TopP                  float64 `json:"topP"`
	FPDeterminism         bool    `json:"fpDeterminism"`
	BatchInvariantKernels bool    `json:"batchInvariantKernels"`
	Backend               string  `json:"backend"`
	BackendVersion        string  `json:"backendVersion"`
}

// HardwareInfo is the hardware fingerprint. Fields may be empty for
// hosted-API backends where hardware is opaque; in that case set
// Provider instead.
type HardwareInfo struct {
	GPU           string `json:"gpu,omitempty"`
	GPUCount      int    `json:"gpuCount,omitempty"`
	CUDAVersion   string `json:"cudaVersion,omitempty"`
	DriverVersion string `json:"driverVersion,omitempty"`
	CPU           string `json:"cpu,omitempty"`
	Provider      string `json:"provider,omitempty"`
}

// Results carries the eval scores. The Raw field, if present, holds the
// harness's full results document for consumers who want full fidelity.
type Results struct {
	PrimaryMetric string             `json:"primaryMetric"`
	PrimaryValue  float64            `json:"primaryValue"`
	PrimaryStderr float64            `json:"primaryStderr,omitempty"`
	PerTask       map[string]float64 `json:"perTask,omitempty"`
	Raw           any                `json:"raw,omitempty"`
}

// DeterminismCheck records whether hanko actively verified that the run
// is reproducible. A non-deterministic run is not a trust failure but
// consumers must be able to see the condition.
type DeterminismCheck struct {
	Performed bool                    `json:"performed"`
	Method    DeterminismCheck_Method `json:"method,omitempty"`
	Passed    bool                    `json:"passed,omitempty"`
	Details   string                  `json:"details,omitempty"`
}

// DeterminismCheck_Method enumerates the supported re-run strategies.
type DeterminismCheck_Method string

const (
	// MethodSkipped: no determinism check was attempted.
	MethodSkipped DeterminismCheck_Method = "skipped"
	// MethodDoubleRunByteEqual: a subset of questions was re-run and
	// model outputs were compared byte-for-byte.
	MethodDoubleRunByteEqual DeterminismCheck_Method = "double-run-byte-equal"
	// MethodDoubleRunScoreEqual: the full eval was re-run and the
	// aggregate score was compared.
	MethodDoubleRunScoreEqual DeterminismCheck_Method = "double-run-score-equal"
)
