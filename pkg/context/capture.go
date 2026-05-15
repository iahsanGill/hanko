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
	"fmt"
	"os"
	"runtime"
	"time"
)

// CaptureOptions are the inputs to Capture. They mirror the CLI surface
// of `hanko run` — each field maps to one or more user-facing flags or
// environment variables.
type CaptureOptions struct {
	// Model identification.
	Model       string
	ModelSource string

	// Benchmark identification.
	Harness string
	Task    string

	// Runtime knobs.
	Seed                  int
	BatchSize             int
	Temperature           float64
	TopP                  float64
	FPDeterminism         bool
	BatchInvariantKernels bool
	Backend               string

	// Hanko self-version (injected at build time).
	HankoVersion string
}

// Capture builds a partially-populated RunContext from CaptureOptions.
// In v0.1 most fields are taken at face value from the caller; later
// passes (harness adapter, hardware probe, post-run results) fill in
// the rest.
//
// Capture intentionally does not fail when probes cannot resolve a
// value: it records what it knows and leaves the unknown fields empty
// for downstream code to populate. The CLI is responsible for asserting
// that required fields are present before emitting an attestation.
func Capture(opts CaptureOptions) RunContext {
	rc := RunContext{
		Model: ModelRef{
			Ref:    opts.Model,
			Source: opts.ModelSource,
		},
		Benchmark: BenchmarkRef{
			Harness: opts.Harness,
			Task:    opts.Task,
		},
		Runtime: RuntimeConfig{
			Seed:                  opts.Seed,
			BatchSize:             opts.BatchSize,
			Temperature:           opts.Temperature,
			TopP:                  opts.TopP,
			FPDeterminism:         opts.FPDeterminism,
			BatchInvariantKernels: opts.BatchInvariantKernels,
			Backend:               opts.Backend,
		},
		Hardware: probeHardware(),
		DeterminismCheck: DeterminismCheck{
			Performed: false,
			Method:    MethodSkipped,
		},
		StartedAt:    time.Now().UTC(),
		HankoVersion: opts.HankoVersion,
	}
	return rc
}

// probeHardware returns a best-effort hardware fingerprint. On platforms
// without nvidia-smi or equivalent it returns CPU info from runtime only.
// Hardware probing for GPU/CUDA/driver is a v0.2 follow-up; the schema
// already accommodates the additional fields.
func probeHardware() HardwareInfo {
	hw := HardwareInfo{
		CPU: cpuLabel(),
	}
	return hw
}

// cpuLabel returns a coarse CPU identifier suitable for the fingerprint.
// Refinement to a specific model string (e.g. "Intel Xeon Platinum 8480+")
// requires platform-specific probes and is deferred to v0.2.
func cpuLabel() string {
	return fmt.Sprintf("%s/%s/%d-core", runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
}

// EnvDefaultBool resolves a boolean flag from an environment variable,
// falling back to def if unset or unparseable. Exposed so callers can
// share the convention across packages.
func EnvDefaultBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	switch v {
	case "1", "true", "TRUE", "True", "yes":
		return true
	case "0", "false", "FALSE", "False", "no":
		return false
	default:
		return def
	}
}
