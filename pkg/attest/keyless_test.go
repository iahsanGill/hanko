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

package attest

import (
	"bytes"
	"context"
	_ "embed"
	"os"
	"strings"
	"testing"
)

// testdata/sigstore-bundle-public-good.json is the sigstore-go example
// bundle: a SLSA Provenance Statement signed by the sigstore-js release
// workflow, with the Fulcio cert + Rekor entry inside. The cert has long
// since expired by wall clock, but sigstore-go evaluates the cert against
// the integrated Rekor timestamp, so verification stays valid forever.
//
//go:embed testdata/sigstore-bundle-public-good.json
var sigstoreBundlePublicGood []byte

// testdata/trusted-root-public-good.json is a snapshot of the Sigstore
// public-good trusted root taken when the bundle above was signed. Pinned
// here so the test stays offline and deterministic.
//
//go:embed testdata/trusted-root-public-good.json
var trustedRootPublicGood []byte

// TestVerifyKeyless_PublicGoodBundle confirms the keyless verifier
// accepts a real Sigstore bundle: cert chains to Fulcio, SAN matches the
// expected sigstore-js workflow identity, OIDC issuer matches GitHub
// Actions, and the Rekor inclusion proof checks. The returned payload is
// the verified DSSE payload bytes (a SLSA Provenance Statement).
func TestVerifyKeyless_PublicGoodBundle(t *testing.T) {
	payload, err := VerifyKeyless(sigstoreBundlePublicGood, KeylessVerifyOptions{
		Identity:        "https://github.com/sigstore/sigstore-js/.github/workflows/release.yml@refs/heads/main",
		Issuer:          "https://token.actions.githubusercontent.com",
		TrustedRootJSON: trustedRootPublicGood,
	})
	if err != nil {
		t.Fatalf("VerifyKeyless: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("payload is empty")
	}
	// The payload is an in-toto Statement — confirm the recovered bytes
	// look like one (we don't try to parse it as a hanko EvalRun because
	// it's a SLSA provenance Statement; that's fine — we're testing the
	// Sigstore signing material, not the predicate schema).
	for _, want := range []string{"_type", "predicateType", "subject"} {
		if !bytes.Contains(payload, []byte(want)) {
			t.Errorf("verified payload missing %q (length %d)", want, len(payload))
		}
	}
}

// TestVerifyKeyless_RejectsWrongIdentity confirms an attacker who swaps
// in their own identity match value can't verify a bundle signed by a
// different identity, even with the correct trusted root.
func TestVerifyKeyless_RejectsWrongIdentity(t *testing.T) {
	_, err := VerifyKeyless(sigstoreBundlePublicGood, KeylessVerifyOptions{
		Identity:        "evil@example.invalid",
		Issuer:          "https://token.actions.githubusercontent.com",
		TrustedRootJSON: trustedRootPublicGood,
	})
	if err == nil {
		t.Fatal("expected verification to fail with wrong identity")
	}
	if !strings.Contains(err.Error(), "verif") && !strings.Contains(err.Error(), "identit") {
		t.Errorf("error should signal identity/verification failure: %v", err)
	}
}

// TestVerifyKeyless_RejectsWrongIssuer confirms the OIDC issuer claim is
// part of the policy — same cert, wrong issuer, no match.
func TestVerifyKeyless_RejectsWrongIssuer(t *testing.T) {
	_, err := VerifyKeyless(sigstoreBundlePublicGood, KeylessVerifyOptions{
		Identity:        "https://github.com/sigstore/sigstore-js/.github/workflows/release.yml@refs/heads/main",
		Issuer:          "https://accounts.google.com",
		TrustedRootJSON: trustedRootPublicGood,
	})
	if err == nil {
		t.Fatal("expected verification to fail with wrong issuer")
	}
}

// TestVerifyKeyless_RejectsCorruptBundle exercises the bundle-parse
// error path: garbage JSON in, structured error out.
func TestVerifyKeyless_RejectsCorruptBundle(t *testing.T) {
	_, err := VerifyKeyless([]byte("{not a bundle}"), KeylessVerifyOptions{
		Identity:        "x",
		Issuer:          "y",
		TrustedRootJSON: trustedRootPublicGood,
	})
	if err == nil {
		t.Fatal("expected corrupt bundle to fail")
	}
	if !strings.Contains(err.Error(), "decode") && !strings.Contains(err.Error(), "validate") {
		t.Errorf("error should signal bundle parse failure: %v", err)
	}
}

// TestVerifyKeyless_RequiresIdentityOrIssuer guards against the
// "forgot to set policy" footgun — the policy MUST require an identity
// match; we never silently accept any signer.
func TestVerifyKeyless_RequiresIdentityOrIssuer(t *testing.T) {
	if _, err := VerifyKeyless(sigstoreBundlePublicGood, KeylessVerifyOptions{
		Issuer:          "https://token.actions.githubusercontent.com",
		TrustedRootJSON: trustedRootPublicGood,
	}); err == nil {
		t.Error("expected error when identity not set")
	}
	if _, err := VerifyKeyless(sigstoreBundlePublicGood, KeylessVerifyOptions{
		Identity:        "x",
		TrustedRootJSON: trustedRootPublicGood,
	}); err == nil {
		t.Error("expected error when issuer not set")
	}
}

// TestResolveOIDCToken_PrefersEnvVar confirms the SIGSTORE_ID_TOKEN env
// var short-circuits the GitHub Actions ambient probe.
func TestResolveOIDCToken_PrefersEnvVar(t *testing.T) {
	t.Setenv("SIGSTORE_ID_TOKEN", "from-env")
	// Set the GHA vars too so we'd accidentally hit network if the env
	// var weren't honored first; the test fails loudly in that case.
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "http://127.0.0.1:1/should-not-be-called")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "x")

	got, err := ResolveOIDCToken(context.Background())
	if err != nil {
		t.Fatalf("ResolveOIDCToken: %v", err)
	}
	if got != "from-env" {
		t.Errorf("got %q, want %q", got, "from-env")
	}
}

// TestResolveOIDCToken_EmptyWhenNothingConfigured matches the documented
// contract — no source configured returns ("", nil), not an error. The
// caller decides what to do with the absence.
func TestResolveOIDCToken_EmptyWhenNothingConfigured(t *testing.T) {
	for _, k := range []string{"SIGSTORE_ID_TOKEN", "ACTIONS_ID_TOKEN_REQUEST_URL", "ACTIONS_ID_TOKEN_REQUEST_TOKEN"} {
		_ = os.Unsetenv(k)
	}
	got, err := ResolveOIDCToken(context.Background())
	if err != nil {
		t.Fatalf("ResolveOIDCToken: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty token, got %q", got)
	}
}
