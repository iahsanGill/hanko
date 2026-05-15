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
	"encoding/json"
	"fmt"

	"github.com/iahsanGill/hanko/internal/version"
	hkctx "github.com/iahsanGill/hanko/pkg/context"
	"github.com/spf13/cobra"
)

type runOpts struct {
	model       string
	modelSource string
	harness     string
	task        string

	seed                  int
	batchSize             int
	temperature           float64
	topP                  float64
	fpDeterminism         bool
	batchInvariantKernels bool
	backend               string

	dryRun bool
	output string
}

func newRunCmd() *cobra.Command {
	o := &runOpts{}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run an eval and produce a signed bundle",
		Long: `run invokes an evaluation harness against a model, captures the
canonical run context, signs the result with Sigstore, and publishes the
bundle as an OCI artifact at --output.

In v0.1 only --dry-run is fully wired: it captures and prints the context
without invoking the harness. The harness adapter and signing/push pipeline
land in subsequent v0.1 milestones.`,
		RunE: o.run,
	}
	f := cmd.Flags()
	f.StringVar(&o.model, "model", "", "Model reference, e.g. meta-llama/Llama-3.1-8B (required)")
	f.StringVar(&o.modelSource, "model-source", "huggingface", "Source of the model: huggingface, local, ollama, s3, gs")
	f.StringVar(&o.harness, "harness", "lm-evaluation-harness", "Evaluation harness")
	f.StringVar(&o.task, "task", "", "Task within the harness, e.g. mmlu (required)")

	f.IntVar(&o.seed, "seed", 42, "Random seed")
	f.IntVar(&o.batchSize, "batch-size", 32, "Inference batch size")
	f.Float64Var(&o.temperature, "temperature", 0.0, "Sampling temperature")
	f.Float64Var(&o.topP, "top-p", 1.0, "Top-p (nucleus) sampling")
	f.BoolVar(&o.fpDeterminism, "fp-determinism", true, "Whether FP-determinism env flags are set")
	f.BoolVar(&o.batchInvariantKernels, "batch-invariant", true, "Whether batch-invariant inference kernels are engaged")
	f.StringVar(&o.backend, "backend", "vllm", "Inference backend: vllm, sglang, transformers, ollama, ...")

	f.BoolVar(&o.dryRun, "dry-run", false, "Capture and print the run context without invoking the harness")
	f.StringVarP(&o.output, "output", "o", "", "OCI URL to publish the signed bundle, e.g. oci://ghcr.io/user/evals/run-name")

	_ = cmd.MarkFlagRequired("model")
	_ = cmd.MarkFlagRequired("task")
	return cmd
}

func (o *runOpts) run(cmd *cobra.Command, _ []string) error {
	ctx := hkctx.Capture(hkctx.CaptureOptions{
		Model:                 o.model,
		ModelSource:           o.modelSource,
		Harness:               o.harness,
		Task:                  o.task,
		Seed:                  o.seed,
		BatchSize:             o.batchSize,
		Temperature:           o.temperature,
		TopP:                  o.topP,
		FPDeterminism:         o.fpDeterminism,
		BatchInvariantKernels: o.batchInvariantKernels,
		Backend:               o.backend,
		HankoVersion:          version.Version,
	})

	if o.dryRun {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(ctx)
	}

	// Non-dry-run execution lands in subsequent milestones: harness
	// adapter (week 2), attestation + signing + push (week 3),
	// determinism verification (week 4). Returning an explicit error
	// here is the right surface for users until those land.
	return fmt.Errorf("non-dry-run execution is not yet implemented; pass --dry-run to inspect the captured context")
}
