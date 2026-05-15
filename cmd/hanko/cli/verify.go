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
	"fmt"
	"io"

	"github.com/iahsanGill/hanko/pkg/attest"
	"github.com/iahsanGill/hanko/pkg/bundle"
	hkctx "github.com/iahsanGill/hanko/pkg/context"
	"github.com/spf13/cobra"
)

type verifyOpts struct {
	publicKey string
}

func newVerifyCmd() *cobra.Command {
	o := &verifyOpts{}
	cmd := &cobra.Command{
		Use:   "verify <oci-url>",
		Short: "Verify a signed hanko bundle",
		Long: `verify pulls a hanko bundle from an OCI registry, validates its
DSSE signature against --public-key, decodes the in-toto Statement, and
prints a human-readable summary of the EvalRun predicate.

Exit code 0 means: signature verified, payload type recognized, and the
predicate parses as a hanko EvalRun v1. Anything else exits non-zero.`,
		Args: cobra.ExactArgs(1),
		RunE: o.run,
	}
	cmd.Flags().StringVar(&o.publicKey, "public-key", "",
		"Path to the PEM-encoded Ed25519 public key to verify against (required)")
	_ = cmd.MarkFlagRequired("public-key")
	return cmd
}

func (o *verifyOpts) run(cmd *cobra.Command, args []string) error {
	ref := args[0]

	pub, err := attest.LoadPublicKey(o.publicKey)
	if err != nil {
		return fmt.Errorf("load public key: %w", err)
	}

	envBytes, err := bundle.Pull(ref)
	if err != nil {
		return fmt.Errorf("pull bundle: %w", err)
	}

	env, err := attest.UnmarshalEnvelope(envBytes)
	if err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}

	payload, err := attest.Verify(env, pub)
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	stmt, err := attest.ParseStatement(payload)
	if err != nil {
		return fmt.Errorf("statement parse: %w", err)
	}

	rc, err := stmt.PredicateAs()
	if err != nil {
		return fmt.Errorf("predicate decode: %w", err)
	}

	return printSummary(cmd.OutOrStdout(), ref, rc)
}

// printSummary renders the verified bundle as the human-readable block
// shown in the project's quick-start. Keep this format stable — it's
// what users grep and what gets pasted into bug reports.
func printSummary(w io.Writer, ref string, rc *hkctx.RunContext) error {
	determinism := "skipped"
	if rc.DeterminismCheck.Performed {
		if rc.DeterminismCheck.Passed {
			determinism = "passed (" + string(rc.DeterminismCheck.Method) + ")"
		} else {
			determinism = "FAILED (" + string(rc.DeterminismCheck.Method) + ")"
		}
	}

	_, err := fmt.Fprintf(w, `Verified bundle: %s

  Model:     %s (%s)
  Benchmark: %s/%s @ commit %s
  Runtime:   %s %s · seed=%d · batch=%d · batch-invariant=%v
  Hardware:  %s
  Result:    %s = %.4f (stderr %.4f)
  Determinism: %s

`,
		ref,
		rc.Model.Ref, rc.Model.Source,
		rc.Benchmark.Harness, rc.Benchmark.Task, abbrev(rc.Benchmark.HarnessCommit),
		rc.Runtime.Backend, rc.Runtime.BackendVersion, rc.Runtime.Seed, rc.Runtime.BatchSize, rc.Runtime.BatchInvariantKernels,
		hardwareSummary(rc),
		rc.Results.PrimaryMetric, rc.Results.PrimaryValue, rc.Results.PrimaryStderr,
		determinism,
	)
	return err
}

// abbrev shortens long hashes for human display. We keep the full hash
// in the structured output (--json, when added in v0.2); the abbrev is
// purely cosmetic.
func abbrev(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}

func hardwareSummary(rc *hkctx.RunContext) string {
	if rc.Hardware.GPU != "" {
		if rc.Hardware.GPUCount > 0 {
			return fmt.Sprintf("%dx %s (CUDA %s)", rc.Hardware.GPUCount, rc.Hardware.GPU, rc.Hardware.CUDAVersion)
		}
		return rc.Hardware.GPU
	}
	if rc.Hardware.Provider != "" {
		return "hosted-api/" + rc.Hardware.Provider
	}
	if rc.Hardware.CPU != "" {
		return rc.Hardware.CPU
	}
	return "(unspecified)"
}
