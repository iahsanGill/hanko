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

// Package attest wraps a hanko RunContext into an in-toto v1 Statement of
// type EvalRun v1 and provides DSSE sign/verify primitives over Ed25519
// keys. The resulting envelope is what gets stored as the OCI artifact
// payload in pkg/bundle.
package attest

import (
	"encoding/json"
	"errors"
	"fmt"

	hkctx "github.com/iahsanGill/hanko/pkg/context"
)

const (
	// StatementType is the in-toto v1 Statement type URI.
	StatementType = "https://in-toto.io/Statement/v1"

	// PredicateType is the URI of hanko's EvalRun predicate type.
	// Schema lives in spec/eval-run-v1.md.
	PredicateType = "https://withhanko.dev/EvalRun/v1"

	// PayloadTypeURI identifies a DSSE payload that carries an in-toto
	// Statement encoded as JSON. This is the standard value used by
	// cosign, GitHub artifact attestations, and SLSA tooling.
	PayloadTypeURI = "application/vnd.in-toto+json"
)

// Statement is an in-toto v1 Statement. It is generic over the predicate
// type so the same struct can carry any predicate URI; hanko populates it
// with PredicateType=hanko's EvalRun URI and Predicate=RunContext.
type Statement struct {
	Type          string    `json:"_type"`
	Subject       []Subject `json:"subject"`
	PredicateType string    `json:"predicateType"`
	Predicate     any       `json:"predicate"`
}

// Subject identifies one of the artifacts the Statement is about. For an
// EvalRun the canonical subject is the harness + task pair. The digest
// pins the harness commit so consumers can detect silent harness drift.
type Subject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// Build produces an in-toto Statement for the given RunContext. The
// subject's name encodes the harness + task ("<harness>/<task>") so it's
// stable across multiple runs of the same eval; the subject's sha256
// digest pins the harness commit.
func Build(rc *hkctx.RunContext) (*Statement, error) {
	if rc == nil {
		return nil, errors.New("attest.Build: nil RunContext")
	}
	if rc.Benchmark.Harness == "" {
		return nil, errors.New("attest.Build: RunContext.Benchmark.Harness is empty")
	}
	if rc.Benchmark.Task == "" {
		return nil, errors.New("attest.Build: RunContext.Benchmark.Task is empty")
	}
	if rc.Benchmark.HarnessCommit == "" {
		return nil, errors.New("attest.Build: RunContext.Benchmark.HarnessCommit is empty; harness adapter must populate it")
	}
	return &Statement{
		Type: StatementType,
		Subject: []Subject{{
			Name: fmt.Sprintf("%s/%s", rc.Benchmark.Harness, rc.Benchmark.Task),
			Digest: map[string]string{
				"sha256": rc.Benchmark.HarnessCommit,
			},
		}},
		PredicateType: PredicateType,
		Predicate:     rc,
	}, nil
}

// Marshal serializes the Statement to canonical JSON. The shape is fixed
// by struct field order in the JSON tags above; DSSE PAE then frames
// these bytes for signing.
func (s *Statement) Marshal() ([]byte, error) {
	return json.Marshal(s)
}

// ParseStatement decodes payload bytes (typically the output of a
// successful DSSE Verify) into a Statement and checks the schema
// constants match what hanko produces. Returns an error if the type or
// predicate type are unrecognized, the predicate is missing, or the
// subject array is empty.
func ParseStatement(payload []byte) (*Statement, error) {
	var s Statement
	if err := json.Unmarshal(payload, &s); err != nil {
		return nil, fmt.Errorf("decode statement: %w", err)
	}
	if s.Type != StatementType {
		return nil, fmt.Errorf("statement: unexpected _type %q (want %q)", s.Type, StatementType)
	}
	if s.PredicateType != PredicateType {
		return nil, fmt.Errorf("statement: unexpected predicateType %q (want %q)", s.PredicateType, PredicateType)
	}
	if s.Predicate == nil {
		return nil, errors.New("statement: predicate is missing")
	}
	if len(s.Subject) == 0 {
		return nil, errors.New("statement: subject is empty")
	}
	return &s, nil
}

// PredicateAs decodes the Statement's predicate into a typed *RunContext.
// The on-wire Statement carries predicate as any so generic verifiers can
// still parse it; callers that need typed access call this helper.
func (s *Statement) PredicateAs() (*hkctx.RunContext, error) {
	b, err := json.Marshal(s.Predicate)
	if err != nil {
		return nil, fmt.Errorf("re-marshal predicate: %w", err)
	}
	var rc hkctx.RunContext
	if err := json.Unmarshal(b, &rc); err != nil {
		return nil, fmt.Errorf("decode predicate as RunContext: %w", err)
	}
	return &rc, nil
}
