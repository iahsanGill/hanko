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
	"encoding/base64"
	"path/filepath"
	"testing"
	"time"

	hkctx "github.com/iahsanGill/hanko/pkg/context"
)

func sampleRC() *hkctx.RunContext {
	return &hkctx.RunContext{
		Model: hkctx.ModelRef{Ref: "meta-llama/Llama-3.1-8B", Source: "huggingface"},
		Benchmark: hkctx.BenchmarkRef{
			Harness:       "lm-evaluation-harness",
			HarnessCommit: "1a2b3c4d5e6f7890abcdef0123456789abcdef01",
			Task:          "mmlu",
			TaskVersion:   "2",
		},
		Runtime: hkctx.RuntimeConfig{
			Seed: 42, BatchSize: 32, TopP: 1.0,
			FPDeterminism: true, BatchInvariantKernels: true,
			Backend: "vllm",
		},
		Results: hkctx.Results{
			PrimaryMetric: "acc",
			PrimaryValue:  0.6826,
		},
		StartedAt:    time.Now().UTC().Add(-time.Hour),
		CompletedAt:  time.Now().UTC(),
		HankoVersion: "0.1.0",
	}
}

func TestBuild(t *testing.T) {
	rc := sampleRC()
	stmt, err := Build(rc)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if stmt.Type != StatementType {
		t.Errorf("Type: got %q", stmt.Type)
	}
	if stmt.PredicateType != PredicateType {
		t.Errorf("PredicateType: got %q", stmt.PredicateType)
	}
	if len(stmt.Subject) != 1 {
		t.Fatalf("expected one subject, got %d", len(stmt.Subject))
	}
	if want := "lm-evaluation-harness/mmlu"; stmt.Subject[0].Name != want {
		t.Errorf("Subject.Name: got %q, want %q", stmt.Subject[0].Name, want)
	}
	if stmt.Subject[0].Digest["sha256"] != rc.Benchmark.HarnessCommit {
		t.Errorf("Subject.Digest: got %v", stmt.Subject[0].Digest)
	}
}

func TestBuild_RejectsEmpty(t *testing.T) {
	if _, err := Build(&hkctx.RunContext{}); err == nil {
		t.Error("expected error for empty harness/task/commit")
	}
}

func TestSignVerify_Roundtrip(t *testing.T) {
	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	stmt, err := Build(sampleRC())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := stmt.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	env, err := Sign(payload, PayloadTypeURI, priv)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if env.PayloadType != PayloadTypeURI {
		t.Errorf("PayloadType: got %q", env.PayloadType)
	}
	if env.Signatures[0].KeyID == "" {
		t.Error("KeyID should be populated")
	}

	got, err := Verify(env, pub)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Error("verified payload does not match original")
	}

	// Round-trip through ParseStatement to confirm the recovered
	// payload still satisfies the schema checks.
	parsed, err := ParseStatement(got)
	if err != nil {
		t.Fatalf("ParseStatement: %v", err)
	}
	rcBack, err := parsed.PredicateAs()
	if err != nil {
		t.Fatalf("PredicateAs: %v", err)
	}
	if rcBack.Results.PrimaryValue != 0.6826 {
		t.Errorf("round-trip PrimaryValue: got %v", rcBack.Results.PrimaryValue)
	}
}

func TestVerify_RejectsTamperedPayload(t *testing.T) {
	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	env, err := Sign([]byte(`{"score":1}`), PayloadTypeURI, priv)
	if err != nil {
		t.Fatal(err)
	}
	env.Payload = base64.StdEncoding.EncodeToString([]byte(`{"score":0}`))
	if _, err := Verify(env, pub); err == nil {
		t.Error("expected verification to fail after payload tampering")
	}
}

func TestVerify_RejectsWrongKey(t *testing.T) {
	_, priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	otherPub, _, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	env, err := Sign([]byte("payload"), PayloadTypeURI, priv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(env, otherPub); err == nil {
		t.Error("expected verification with wrong public key to fail")
	}
}

func TestParseStatement_RejectsWrongType(t *testing.T) {
	stmt := &Statement{
		Type:          "not-in-toto",
		Subject:       []Subject{{Name: "x", Digest: map[string]string{"sha256": "abc"}}},
		PredicateType: PredicateType,
		Predicate:     map[string]string{"a": "b"},
	}
	b, _ := stmt.Marshal()
	if _, err := ParseStatement(b); err == nil {
		t.Error("expected error for wrong _type")
	}
}

func TestKeyPersistence_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "key.pem")
	pubPath := filepath.Join(dir, "key.pub")

	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePrivateKeyPEM(privPath, priv); err != nil {
		t.Fatalf("WritePrivateKeyPEM: %v", err)
	}
	if err := WritePublicKeyPEM(pubPath, pub); err != nil {
		t.Fatalf("WritePublicKeyPEM: %v", err)
	}

	gotPriv, err := LoadPrivateKey(privPath)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	if !bytes.Equal(gotPriv, priv) {
		t.Error("loaded private key differs from original")
	}
	gotPub, err := LoadPublicKey(pubPath)
	if err != nil {
		t.Fatalf("LoadPublicKey: %v", err)
	}
	if !bytes.Equal(gotPub, pub) {
		t.Error("loaded public key differs from original")
	}
}

func TestWritePrivateKey_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "key.pem")
	_, priv, _ := GenerateKey()
	if err := WritePrivateKeyPEM(p, priv); err != nil {
		t.Fatal(err)
	}
	if err := WritePrivateKeyPEM(p, priv); err == nil {
		t.Error("expected WritePrivateKeyPEM to refuse overwriting an existing file")
	}
}
