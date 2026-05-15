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
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// Envelope is the DSSE wire form. See
// https://github.com/secure-systems-lab/dsse/blob/master/envelope.md
type Envelope struct {
	PayloadType string      `json:"payloadType"`
	Payload     string      `json:"payload"` // base64(payload_bytes)
	Signatures  []Signature `json:"signatures"`
}

// Signature is one signer over the PAE of (PayloadType, Payload).
type Signature struct {
	KeyID string `json:"keyid,omitempty"`
	Sig   string `json:"sig"` // base64(signature_bytes)
}

// pae implements the DSSE Pre-Authentication Encoding:
//
//	"DSSEv1 <len type> <type> <len payload> <payload>"
//
// where lengths are decimal ASCII. The signer signs the PAE, not the raw
// payload, so a verifier that trusts only the payload bytes can't be
// fooled by a different PayloadType claiming the same body.
func pae(payloadType string, payload []byte) []byte {
	header := fmt.Sprintf("DSSEv1 %d %s %d ", len(payloadType), payloadType, len(payload))
	out := make([]byte, 0, len(header)+len(payload))
	out = append(out, header...)
	out = append(out, payload...)
	return out
}

// Sign produces a DSSE envelope over the given payload using the
// supplied Ed25519 private key. KeyID is the hex SHA-256 of the public
// key bytes — short, deterministic, lets verifiers pick the right key
// out of a keyring without leaking key material.
func Sign(payload []byte, payloadType string, key ed25519.PrivateKey) (*Envelope, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("attest.Sign: bad private key length %d", len(key))
	}
	sig := ed25519.Sign(key, pae(payloadType, payload))
	return &Envelope{
		PayloadType: payloadType,
		Payload:     base64.StdEncoding.EncodeToString(payload),
		Signatures: []Signature{{
			KeyID: keyIDFor(key.Public().(ed25519.PublicKey)),
			Sig:   base64.StdEncoding.EncodeToString(sig),
		}},
	}, nil
}

// Verify checks the envelope's signature against pub and, on success,
// returns the decoded payload. Multiple signatures are accepted but at
// least one must verify against pub.
func Verify(env *Envelope, pub ed25519.PublicKey) ([]byte, error) {
	if env == nil {
		return nil, errors.New("attest.Verify: nil envelope")
	}
	if len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("attest.Verify: bad public key length %d", len(pub))
	}
	if env.PayloadType == "" {
		return nil, errors.New("attest.Verify: envelope missing payloadType")
	}
	payload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	wantKID := keyIDFor(pub)
	signed := pae(env.PayloadType, payload)
	for _, s := range env.Signatures {
		// Skip signatures with a non-matching KeyID when one is present,
		// so a wrong-key signature can't shadow the right one.
		if s.KeyID != "" && s.KeyID != wantKID {
			continue
		}
		sig, err := base64.StdEncoding.DecodeString(s.Sig)
		if err != nil {
			continue
		}
		if ed25519.Verify(pub, signed, sig) {
			return payload, nil
		}
	}
	return nil, errors.New("attest.Verify: no matching signature")
}

func keyIDFor(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])
}

// MarshalEnvelope is a thin convenience wrapper; the on-wire format is
// just json.Marshal, but keeping the helper makes intent explicit at
// call sites.
func MarshalEnvelope(e *Envelope) ([]byte, error) {
	return json.Marshal(e)
}

// UnmarshalEnvelope is the symmetric reader.
func UnmarshalEnvelope(b []byte) (*Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return &e, nil
}
