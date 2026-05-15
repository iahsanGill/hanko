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
	"errors"
	"fmt"
	"io"

	"github.com/iahsanGill/hanko/pkg/attest"
	"github.com/iahsanGill/hanko/pkg/bundle"
	hkctx "github.com/iahsanGill/hanko/pkg/context"
	"github.com/spf13/cobra"
)

type verifyOpts struct {
	publicKey string

	// Sigstore keyless verification.
	certIdentity      string
	certIdentityRegex string
	certIssuer        string
	certIssuerRegex   string
	sigstoreStaging   bool
}

func newVerifyCmd() *cobra.Command {
	o := &verifyOpts{}
	cmd := &cobra.Command{
		Use:   "verify <oci-url>",
		Short: "Verify a signed hanko bundle",
		Long: `verify pulls a hanko bundle from an OCI registry, validates its
signature, decodes the in-toto Statement, and prints a human-readable
summary of the EvalRun predicate.

Two verification modes; the right one is selected by inspecting the
pulled layer's media type:

  --public-key <path>                v0.1 BYO-key bundles. Reads a
                                      PEM-encoded Ed25519 public key.

  --certificate-identity <san>       v0.2 Sigstore keyless bundles. The
  --certificate-oidc-issuer <url>    cert SAN and OIDC issuer claims are
                                      checked against the expected values
                                      below; the cert chain, Rekor
                                      inclusion proof, SCT, and DSSE
                                      signature are all validated against
                                      the Sigstore trusted root (fetched
                                      via TUF on first run).

Exit code 0 means signature verified, payload type recognized, and the
predicate parses as a hanko EvalRun v1. Anything else exits non-zero.`,
		Args: cobra.ExactArgs(1),
		RunE: o.run,
	}
	f := cmd.Flags()
	f.StringVar(&o.publicKey, "public-key", "",
		"Path to a PEM-encoded Ed25519 public key (v0.1 bundles).")
	f.StringVar(&o.certIdentity, "certificate-identity", "",
		"Expected SAN value on the Sigstore signing cert (v0.2 bundles).")
	f.StringVar(&o.certIdentityRegex, "certificate-identity-regexp", "",
		"Regex form of --certificate-identity.")
	f.StringVar(&o.certIssuer, "certificate-oidc-issuer", "",
		"Expected OIDC issuer claim on the Sigstore signing cert.")
	f.StringVar(&o.certIssuerRegex, "certificate-oidc-issuer-regexp", "",
		"Regex form of --certificate-oidc-issuer.")
	f.BoolVar(&o.sigstoreStaging, "sigstore-staging", false,
		"Use the Sigstore staging trusted root. Must match the signing instance.")
	return cmd
}

func (o *verifyOpts) run(cmd *cobra.Command, args []string) error {
	if err := o.validate(); err != nil {
		return err
	}
	ref := args[0]

	envBytes, layerType, err := bundle.PullTyped(ref)
	if err != nil {
		return fmt.Errorf("pull bundle: %w", err)
	}

	var rc *hkctx.RunContext
	switch layerType {
	case bundle.LayerMediaType:
		if o.publicKey == "" {
			return fmt.Errorf("bundle is a v0.1 DSSE envelope; --public-key required")
		}
		rc, err = o.verifyDSSE(envBytes)
	case bundle.SigstoreLayerMediaType:
		if o.certIdentity == "" && o.certIdentityRegex == "" {
			return fmt.Errorf("bundle is a v0.2 Sigstore bundle; --certificate-identity (or -regexp) required")
		}
		if o.certIssuer == "" && o.certIssuerRegex == "" {
			return fmt.Errorf("bundle is a v0.2 Sigstore bundle; --certificate-oidc-issuer (or -regexp) required")
		}
		rc, err = o.verifySigstore(envBytes)
	default:
		return fmt.Errorf("unknown bundle layer media type %q", layerType)
	}
	if err != nil {
		return err
	}

	return printSummary(cmd.OutOrStdout(), ref, rc)
}

func (o *verifyOpts) validate() error {
	hasKey := o.publicKey != ""
	hasKeyless := o.certIdentity != "" || o.certIdentityRegex != "" || o.certIssuer != "" || o.certIssuerRegex != ""
	if hasKey && hasKeyless {
		return errors.New("--public-key and --certificate-* flags are mutually exclusive; pick one verification mode")
	}
	if !hasKey && !hasKeyless {
		return errors.New("a verification mode is required: --public-key, or --certificate-identity + --certificate-oidc-issuer")
	}
	return nil
}

func (o *verifyOpts) verifyDSSE(envBytes []byte) (*hkctx.RunContext, error) {
	pub, err := attest.LoadPublicKey(o.publicKey)
	if err != nil {
		return nil, fmt.Errorf("load public key: %w", err)
	}
	env, err := attest.UnmarshalEnvelope(envBytes)
	if err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	payload, err := attest.Verify(env, pub)
	if err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}
	stmt, err := attest.ParseStatement(payload)
	if err != nil {
		return nil, fmt.Errorf("statement parse: %w", err)
	}
	rc, err := stmt.PredicateAs()
	if err != nil {
		return nil, fmt.Errorf("predicate decode: %w", err)
	}
	return rc, nil
}

func (o *verifyOpts) verifySigstore(bundleJSON []byte) (*hkctx.RunContext, error) {
	payload, err := attest.VerifyKeyless(bundleJSON, attest.KeylessVerifyOptions{
		Identity:      o.certIdentity,
		IdentityRegex: o.certIdentityRegex,
		Issuer:        o.certIssuer,
		IssuerRegex:   o.certIssuerRegex,
		UseStaging:    o.sigstoreStaging,
	})
	if err != nil {
		return nil, fmt.Errorf("sigstore verification failed: %w", err)
	}
	stmt, err := attest.ParseStatement(payload)
	if err != nil {
		return nil, fmt.Errorf("statement parse: %w", err)
	}
	rc, err := stmt.PredicateAs()
	if err != nil {
		return nil, fmt.Errorf("predicate decode: %w", err)
	}
	return rc, nil
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
// in the structured output (--json, when added); the abbrev is purely
// cosmetic.
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
