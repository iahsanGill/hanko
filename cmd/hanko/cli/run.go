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
	"errors"
	"fmt"
	"os"

	"github.com/iahsanGill/hanko/internal/version"
	"github.com/iahsanGill/hanko/pkg/attest"
	"github.com/iahsanGill/hanko/pkg/bundle"
	hkctx "github.com/iahsanGill/hanko/pkg/context"
	"github.com/iahsanGill/hanko/pkg/determinism"
	"github.com/iahsanGill/hanko/pkg/runner"
	// Side-effect imports: each adapter package registers itself under
	// its canonical HarnessName via init(). Adding more harness packages
	// here is how new harnesses get discovered by the CLI.
	_ "github.com/iahsanGill/hanko/pkg/runner/inspect"
	_ "github.com/iahsanGill/hanko/pkg/runner/lmeval"
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

	dryRun           bool
	output           string
	keyPath          string
	determinismCheck bool

	// Keyless (Sigstore) signing.
	sigstore           bool
	sigstoreIDToken    string
	sigstoreUseStaging bool
}

func newRunCmd() *cobra.Command {
	o := &runOpts{}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run an eval and produce a signed bundle",
		Long: `run invokes an evaluation harness against a model, captures the
canonical run context, signs the resulting EvalRun Statement, and
publishes the result as an OCI artifact at --output.

Signing modes:
  --key <path>      sign with a long-lived Ed25519 keypair (v0.1 path).
                    Verifiers need the matching public key out of band.
  --sigstore        sign keyless via Sigstore: an ephemeral keypair is
                    minted, Fulcio issues a 10-minute X.509 cert bound to
                    your OIDC identity, the DSSE envelope is signed and
                    logged to Rekor. Verifiers need only the expected
                    identity + issuer, no key exchange. The OIDC token is
                    sourced from --sigstore-id-token, $SIGSTORE_ID_TOKEN,
                    or GitHub Actions ambient OIDC, in that order.

Other modes:
  --dry-run                  capture & print the run context without
                             invoking the harness.
  (no --output)              invoke the harness and print the populated
                             EvalRun context as JSON.`,
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
	f.StringVar(&o.keyPath, "key", "", "Path to an Ed25519 PEM private key for signing (PKCS8). Required when --output is set.")
	f.BoolVar(&o.determinismCheck, "determinism-check", false,
		"Re-run the same eval and assert the aggregate primary score is byte-equal. Doubles the runtime cost; off by default.")

	f.BoolVar(&o.sigstore, "sigstore", false,
		"Sign keyless via Sigstore (Fulcio + Rekor). Mutually exclusive with --key.")
	f.StringVar(&o.sigstoreIDToken, "sigstore-id-token", "",
		"OIDC token for Sigstore. Falls back to $SIGSTORE_ID_TOKEN, then GitHub Actions ambient OIDC.")
	f.BoolVar(&o.sigstoreUseStaging, "sigstore-staging", false,
		"Route Sigstore signing through the staging instance (does not write to production Rekor).")

	_ = cmd.MarkFlagRequired("model")
	_ = cmd.MarkFlagRequired("task")
	return cmd
}

func (o *runOpts) run(cmd *cobra.Command, _ []string) error {
	if err := o.validate(); err != nil {
		return err
	}

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
		return writeJSON(cmd.OutOrStdout(), ctx)
	}

	r, err := runner.Get(o.harness)
	if err != nil {
		return err
	}

	outDir, err := os.MkdirTemp("", "hanko-eval-*")
	if err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	if err := r.Run(cmd.Context(), &ctx, runner.RunOptions{OutputDir: outDir}); err != nil {
		return err
	}

	if o.determinismCheck {
		v := &determinism.Verifier{Runner: r}
		if err := v.Check(cmd.Context(), &ctx); err != nil {
			// Recorded as failed in ctx.DeterminismCheck; surface the
			// inner error for visibility but don't abort — the primary
			// run still produced a meaningful result the operator can
			// publish with passed=false documented in the bundle.
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: determinism re-run failed: %v\n", err)
		}
	}

	if o.output == "" {
		// No publish target: print the populated EvalRun context as JSON.
		return writeJSON(cmd.OutOrStdout(), ctx)
	}

	// Publish flow: build → sign → push.
	stmt, err := attest.Build(&ctx)
	if err != nil {
		return fmt.Errorf("build statement: %w", err)
	}
	payload, err := stmt.Marshal()
	if err != nil {
		return fmt.Errorf("marshal statement: %w", err)
	}

	var layerBytes []byte
	var layerType string
	switch {
	case o.sigstore:
		layerBytes, err = attest.SignKeyless(cmd.Context(), payload, attest.PayloadTypeURI, attest.KeylessSignOptions{
			IDToken:    o.sigstoreIDToken,
			UseStaging: o.sigstoreUseStaging,
		})
		if err != nil {
			return fmt.Errorf("sigstore sign: %w", err)
		}
		layerType = bundle.SigstoreLayerMediaType
	default:
		priv, err := attest.LoadPrivateKey(o.keyPath)
		if err != nil {
			return fmt.Errorf("load private key: %w", err)
		}
		env, err := attest.Sign(payload, attest.PayloadTypeURI, priv)
		if err != nil {
			return fmt.Errorf("sign: %w", err)
		}
		layerBytes, err = attest.MarshalEnvelope(env)
		if err != nil {
			return fmt.Errorf("marshal envelope: %w", err)
		}
		layerType = bundle.LayerMediaType
	}

	digest, err := bundle.PushTyped(layerBytes, o.output, layerType)
	if err != nil {
		return fmt.Errorf("push bundle: %w", err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Published bundle: %s@%s\n", bundle.StripScheme(o.output), digest.String())
	return nil
}

// validate enforces flag combinations the cobra annotations can't express.
func (o *runOpts) validate() error {
	if o.keyPath != "" && o.sigstore {
		return errors.New("--key and --sigstore are mutually exclusive; pick one signing mode")
	}
	if o.output != "" && o.keyPath == "" && !o.sigstore {
		return errors.New("--output requires a signing mode: --key <path> or --sigstore")
	}
	if o.keyPath != "" && o.output == "" {
		return errors.New("--key has no effect without --output; remove it or set --output")
	}
	if o.sigstore && o.output == "" {
		return errors.New("--sigstore has no effect without --output; remove it or set --output")
	}
	return nil
}

func writeJSON(w interface{ Write(p []byte) (int, error) }, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
