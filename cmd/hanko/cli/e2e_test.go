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
	"bytes"
	"context"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"

	hkctx "github.com/iahsanGill/hanko/pkg/context"
	"github.com/iahsanGill/hanko/pkg/runner"
)

// e2eRunner satisfies runner.Runner without invoking any real harness.
// Used in this file (not in run_test.go's fakeRunner) so we can test
// against a different registered name without shadowing.
type e2eRunner struct{ name string }

func (e *e2eRunner) Name() string { return e.name }
func (e *e2eRunner) Run(_ context.Context, rc *hkctx.RunContext, _ runner.RunOptions) error {
	rc.Results.PrimaryMetric = "acc"
	rc.Results.PrimaryValue = 0.7
	rc.Results.PrimaryStderr = 0.01
	rc.Benchmark.HarnessCommit = "1a2b3c4d5e6f7890abcdef0123456789abcdef01"
	rc.Benchmark.TaskVersion = "2"
	rc.Runtime.BackendVersion = "vllm/0.6.5"
	return nil
}

// TestE2E_KeyGenSignPushPullVerify covers the entire v0.1 flow: gen a
// keypair, run an eval with --output --key (sign + push to a local
// registry), then verify the pushed bundle with the public key. This is
// the test that asserts hanko's end-to-end story holds together.
func TestE2E_KeyGenSignPushPullVerify(t *testing.T) {
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	const harness = "e2e-harness"
	runner.Register(&e2eRunner{name: harness})

	dir := t.TempDir()
	privPath := filepath.Join(dir, "hanko.key")
	pubPath := privPath + ".pub"

	// 1. key gen
	{
		root := Root()
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		root.SetArgs([]string{"key", "gen", "--out", privPath})
		if err := root.Execute(); err != nil {
			t.Fatalf("key gen: %v\nstderr: %s", err, stderr.String())
		}
		if !strings.Contains(stdout.String(), privPath) {
			t.Errorf("key gen stdout should mention private key path: %s", stdout.String())
		}
	}

	// 2. run --output --key (sign + push to local registry)
	ref := u.Host + "/test/evals:run1"
	{
		root := Root()
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		root.SetArgs([]string{
			"run",
			"--model", "test/model",
			"--task", "mmlu",
			"--harness", harness,
			"--output", ref,
			"--key", privPath,
		})
		if err := root.Execute(); err != nil {
			t.Fatalf("run --output: %v\nstderr: %s", err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Published bundle:") {
			t.Errorf("run --output should announce publication: %s", stdout.String())
		}
	}

	// 3. verify against the pushed bundle
	{
		root := Root()
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		root.SetArgs([]string{"verify", ref, "--public-key", pubPath})
		if err := root.Execute(); err != nil {
			t.Fatalf("verify: %v\nstderr: %s", err, stderr.String())
		}
		out := stdout.String()
		checks := []string{
			"Verified bundle:",
			"test/model",
			"e2e-harness/mmlu",
			"acc = 0.7000",
		}
		for _, want := range checks {
			if !strings.Contains(out, want) {
				t.Errorf("verify output missing %q in:\n%s", want, out)
			}
		}
	}
}

// TestE2E_VerifyRejectsWrongKey confirms a signature minted with one key
// can't be verified by another. Belt-and-braces over the unit test in
// pkg/attest — the wiring is what shipping consumers see.
func TestE2E_VerifyRejectsWrongKey(t *testing.T) {
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	const harness = "e2e-wrongkey"
	runner.Register(&e2eRunner{name: harness})

	dir := t.TempDir()
	keyA := filepath.Join(dir, "a.key")
	keyB := filepath.Join(dir, "b.key")

	for _, k := range []string{keyA, keyB} {
		root := Root()
		root.SetOut(new(bytes.Buffer))
		root.SetErr(new(bytes.Buffer))
		root.SetArgs([]string{"key", "gen", "--out", k})
		if err := root.Execute(); err != nil {
			t.Fatalf("key gen %s: %v", k, err)
		}
	}

	ref := u.Host + "/test/evals:wrongkey"
	{
		root := Root()
		root.SetOut(new(bytes.Buffer))
		root.SetErr(new(bytes.Buffer))
		root.SetArgs([]string{
			"run",
			"--model", "m", "--task", "t", "--harness", harness,
			"--output", ref, "--key", keyA,
		})
		if err := root.Execute(); err != nil {
			t.Fatalf("run: %v", err)
		}
	}

	// Try to verify with keyB.pub — must fail.
	root := Root()
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"verify", ref, "--public-key", keyB + ".pub"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected verify to fail with wrong public key")
	}
	if !strings.Contains(err.Error(), "verification failed") &&
		!strings.Contains(err.Error(), "no matching signature") {
		t.Errorf("error should signal signature failure: %v", err)
	}
}

// TestE2E_DeterminismCheck_PassesOnDeterministicFakeRunner exercises the
// --determinism-check flag with a runner that returns a stable score
// across invocations. The published bundle must record the check as
// performed=true / passed=true.
func TestE2E_DeterminismCheck_PassesOnDeterministicFakeRunner(t *testing.T) {
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	const harness = "e2e-determinism"
	runner.Register(&e2eRunner{name: harness})

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "k")
	{
		root := Root()
		root.SetOut(new(bytes.Buffer))
		root.SetErr(new(bytes.Buffer))
		root.SetArgs([]string{"key", "gen", "--out", keyPath})
		if err := root.Execute(); err != nil {
			t.Fatalf("key gen: %v", err)
		}
	}

	ref := u.Host + "/test/evals:det"
	{
		root := Root()
		root.SetOut(new(bytes.Buffer))
		root.SetErr(new(bytes.Buffer))
		root.SetArgs([]string{
			"run",
			"--model", "m", "--task", "t", "--harness", harness,
			"--determinism-check",
			"--output", ref, "--key", keyPath,
		})
		if err := root.Execute(); err != nil {
			t.Fatalf("run --determinism-check: %v", err)
		}
	}

	// Verify and confirm the printed summary mentions the passed check.
	root := Root()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"verify", ref, "--public-key", keyPath + ".pub"})
	if err := root.Execute(); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !strings.Contains(stdout.String(), "Determinism: passed") {
		t.Errorf("verify summary should report determinism passed:\n%s", stdout.String())
	}
}

// TestE2E_RunOutputRequiresKey makes sure we don't silently push an
// unsigned envelope when the user forgets --key.
func TestE2E_RunOutputRequiresKey(t *testing.T) {
	root := Root()
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{
		"run", "--model", "m", "--task", "t",
		"--output", "oci://example.invalid/x:y",
	})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected --output without --key to be rejected")
	}
	if !strings.Contains(err.Error(), "--key is required") {
		t.Errorf("error should mention --key required: %v", err)
	}
}
