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
	"errors"
	"reflect"
	"testing"
)

// withGPUProbe swaps the package-level gpuProbe for the duration of t.
// Each test that touches GPU probing must use this helper so global
// state doesn't bleed across cases.
func withGPUProbe(t *testing.T, fn func() ([]gpuInfo, string, error)) {
	t.Helper()
	orig := gpuProbe
	t.Cleanup(func() { gpuProbe = orig })
	gpuProbe = fn
}

func TestProbeHardware_NoGPU(t *testing.T) {
	withGPUProbe(t, func() ([]gpuInfo, string, error) { return nil, "", nil })

	hw := probeHardware()
	if hw.CPU == "" {
		t.Error("CPU should always be populated")
	}
	if hw.GPU != "" || hw.GPUCount != 0 || hw.DriverVersion != "" || hw.CUDAVersion != "" {
		t.Errorf("expected empty GPU fields when probe returns no GPUs, got %+v", hw)
	}
}

func TestProbeHardware_SingleGPU(t *testing.T) {
	withGPUProbe(t, func() ([]gpuInfo, string, error) {
		return []gpuInfo{{Name: "NVIDIA A100-SXM4-80GB", Driver: "535.104.05"}}, "12.2", nil
	})

	hw := probeHardware()
	if hw.GPU != "NVIDIA A100-SXM4-80GB" {
		t.Errorf("GPU: got %q", hw.GPU)
	}
	if hw.GPUCount != 1 {
		t.Errorf("GPUCount: got %d, want 1", hw.GPUCount)
	}
	if hw.DriverVersion != "535.104.05" {
		t.Errorf("DriverVersion: got %q", hw.DriverVersion)
	}
	if hw.CUDAVersion != "12.2" {
		t.Errorf("CUDAVersion: got %q", hw.CUDAVersion)
	}
}

func TestProbeHardware_MultiGPU_HomogeneousFingerprint(t *testing.T) {
	withGPUProbe(t, func() ([]gpuInfo, string, error) {
		// 8x H100 — typical eval rig.
		gpu := gpuInfo{Name: "NVIDIA H100 80GB HBM3", Driver: "550.54.14"}
		return []gpuInfo{gpu, gpu, gpu, gpu, gpu, gpu, gpu, gpu}, "12.4", nil
	})

	hw := probeHardware()
	if hw.GPU != "NVIDIA H100 80GB HBM3" {
		t.Errorf("GPU: got %q", hw.GPU)
	}
	if hw.GPUCount != 8 {
		t.Errorf("GPUCount: got %d, want 8", hw.GPUCount)
	}
	if hw.CUDAVersion != "12.4" {
		t.Errorf("CUDAVersion: got %q", hw.CUDAVersion)
	}
}

// TestProbeHardware_TolerantOfProbeError confirms the probe failing
// (nvidia-smi present but spitting nonsense, etc.) doesn't take down
// the capture path — hardware probing is best-effort by contract.
func TestProbeHardware_TolerantOfProbeError(t *testing.T) {
	withGPUProbe(t, func() ([]gpuInfo, string, error) {
		return nil, "", errors.New("nvidia-smi: unexpected output format")
	})

	hw := probeHardware()
	if hw.CPU == "" {
		t.Error("CPU should still be populated when GPU probe errors")
	}
	if hw.GPU != "" {
		t.Errorf("GPU should be empty when probe errors, got %q", hw.GPU)
	}
}

// TestCapture_GPUFieldsFlowThrough wires the stub into the public
// Capture entrypoint to confirm the fields land on RunContext.Hardware.
func TestCapture_GPUFieldsFlowThrough(t *testing.T) {
	withGPUProbe(t, func() ([]gpuInfo, string, error) {
		return []gpuInfo{{Name: "NVIDIA L40S", Driver: "555.42.06"}}, "12.5", nil
	})

	rc := Capture(CaptureOptions{
		Model: "x", ModelSource: "local",
		Harness: "lm-evaluation-harness", Task: "mmlu",
		Backend: "vllm",
	})
	if rc.Hardware.GPU != "NVIDIA L40S" {
		t.Errorf("GPU: got %q", rc.Hardware.GPU)
	}
	if rc.Hardware.CUDAVersion != "12.5" {
		t.Errorf("CUDAVersion: got %q", rc.Hardware.CUDAVersion)
	}
}

func TestParseNvidiaSMIQuery(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []gpuInfo
	}{
		{
			"single GPU, tight spacing",
			"NVIDIA A100-SXM4-80GB,535.104.05\n",
			[]gpuInfo{{Name: "NVIDIA A100-SXM4-80GB", Driver: "535.104.05"}},
		},
		{
			"comma-space separator (common driver default)",
			"NVIDIA H100 80GB HBM3, 550.54.14\nNVIDIA H100 80GB HBM3, 550.54.14\n",
			[]gpuInfo{
				{Name: "NVIDIA H100 80GB HBM3", Driver: "550.54.14"},
				{Name: "NVIDIA H100 80GB HBM3", Driver: "550.54.14"},
			},
		},
		{
			"empty output (driver present but no GPUs visible)",
			"\n",
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseNvidiaSMIQuery([]byte(tc.in))
			if err != nil {
				t.Fatalf("parseNvidiaSMIQuery: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestParseNvidiaSMIQuery_Malformed(t *testing.T) {
	_, err := parseNvidiaSMIQuery([]byte("NVIDIA-only-one-column\n"))
	if err == nil {
		t.Fatal("expected error on malformed line missing the driver column")
	}
}

func TestParseCUDAVersion(t *testing.T) {
	// Real nvidia-smi banner excerpts across driver generations.
	cases := map[string]string{
		"+-----------------------------------------------------------------------------+\n" +
			"| NVIDIA-SMI 550.54.14    Driver Version: 550.54.14    CUDA Version: 12.4     |\n" +
			"|-------------------------------+----------------------+----------------------+\n": "12.4",
		"| NVIDIA-SMI 470.182.03   Driver Version: 470.182.03   CUDA Version: 11.4     |\n": "11.4",
		"completely unrelated text without the marker":                                      "",
		"": "",
	}
	for in, want := range cases {
		if got := parseCUDAVersion([]byte(in)); got != want {
			t.Errorf("parseCUDAVersion(%q):\n got %q\nwant %q", in[:min(len(in), 60)], got, want)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
