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
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
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

// probeHardware returns a best-effort hardware fingerprint. CPU info
// comes from runtime; GPU/driver/CUDA come from gpuProbe (which shells
// out to nvidia-smi on Linux and returns empty everywhere else). Empty
// fields propagate to the predicate as omitted JSON keys.
func probeHardware() HardwareInfo {
	hw := HardwareInfo{
		CPU: cpuLabel(),
	}
	if gpus, cuda, err := gpuProbe(); err == nil && len(gpus) > 0 {
		// Many-GPU servers are usually homogeneous (e.g. 8x H100).
		// Record the first GPU's name + driver as canonical; the count
		// surfaces the rest. Heterogeneous rigs lose detail here, but
		// they're rare for eval workloads and the operator can pin
		// `--no-hw-probe` (future flag) if they need verbatim multi-GPU
		// listings in a different field.
		hw.GPU = gpus[0].Name
		hw.GPUCount = len(gpus)
		hw.DriverVersion = gpus[0].Driver
		hw.CUDAVersion = cuda
	}
	return hw
}

// cpuLabel returns a coarse CPU identifier suitable for the fingerprint.
// Refinement to a specific model string (e.g. "Intel Xeon Platinum 8480+")
// requires platform-specific probes and is left for a future pass.
func cpuLabel() string {
	return fmt.Sprintf("%s/%s/%d-core", runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
}

// gpuInfo is the per-card record gpuProbe returns. Fields stay lowercase
// because the probe is an internal seam; the export to the predicate
// happens in HardwareInfo.
type gpuInfo struct {
	Name   string
	Driver string
}

// gpuProbe is the package-level seam tests substitute via a stub. The
// default implementation shells out to nvidia-smi on Linux and returns
// (nil, "", nil) on every other platform — a missing-tool isn't an
// error, just a signal that GPU metadata is unavailable. Returning an
// error reserves that channel for unexpected failures (nvidia-smi
// present but spits malformed output, etc.).
var gpuProbe = defaultGPUProbe

// defaultGPUProbe queries nvidia-smi for GPU + driver + CUDA version.
// On non-Linux hosts or when nvidia-smi isn't on PATH, returns no GPUs
// and no error — the absence is just reported as empty hardware fields,
// which is the correct on-wire shape for hosted-API or CPU-only runs.
func defaultGPUProbe() ([]gpuInfo, string, error) {
	if runtime.GOOS != "linux" {
		return nil, "", nil
	}
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil, "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	queryCmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=name,driver_version", "--format=csv,noheader,nounits")
	queryOut, err := queryCmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("nvidia-smi --query-gpu: %w", err)
	}
	gpus, err := parseNvidiaSMIQuery(queryOut)
	if err != nil {
		return nil, "", err
	}

	cudaCmd := exec.CommandContext(ctx, "nvidia-smi")
	cudaOut, _ := cudaCmd.Output() // CUDA line is best-effort; ignore non-zero exit
	cuda := parseCUDAVersion(cudaOut)

	return gpus, cuda, nil
}

// parseNvidiaSMIQuery decodes the CSV nvidia-smi writes for
// --format=csv,noheader,nounits: one line per card, `name, driver`.
// Whitespace around commas varies between driver versions; trim before
// splitting on the separator.
func parseNvidiaSMIQuery(out []byte) ([]gpuInfo, error) {
	var gpus []gpuInfo
	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			return nil, fmt.Errorf("nvidia-smi query: malformed line %q", line)
		}
		gpus = append(gpus, gpuInfo{
			Name:   strings.TrimSpace(parts[0]),
			Driver: strings.TrimSpace(parts[1]),
		})
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("scan nvidia-smi output: %w", err)
	}
	return gpus, nil
}

// parseCUDAVersion finds the "CUDA Version: <x.y>" snippet in
// nvidia-smi's default human-readable banner. Returns "" when the
// banner format doesn't match — common with older drivers and not an
// error worth surfacing.
func parseCUDAVersion(banner []byte) string {
	const marker = "CUDA Version: "
	i := bytes.Index(banner, []byte(marker))
	if i < 0 {
		return ""
	}
	rest := banner[i+len(marker):]
	// Take characters until we hit whitespace, `|`, or newline.
	end := 0
	for end < len(rest) {
		c := rest[end]
		if c == ' ' || c == '\t' || c == '|' || c == '\n' || c == '\r' {
			break
		}
		end++
	}
	return string(rest[:end])
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
