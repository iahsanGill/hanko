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
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

// PEM block types. We use PKCS8 for the private key (multi-algorithm
// container, standard tooling can read it) and PKIX for the public key.
const (
	PrivateKeyPEMType = "PRIVATE KEY"
	PublicKeyPEMType  = "PUBLIC KEY"
)

// GenerateKey creates a fresh Ed25519 keypair.
func GenerateKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

// WritePrivateKeyPEM writes priv to path in PKCS8 PEM form with 0o600
// permissions. Returns an error if the file already exists, so callers
// don't accidentally clobber an existing key.
func WritePrivateKeyPEM(path string, priv ed25519.PrivateKey) error {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("marshal PKCS8: %w", err)
	}
	block := &pem.Block{Type: PrivateKeyPEMType, Bytes: der}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return pem.Encode(f, block)
}

// WritePublicKeyPEM writes pub to path in PKIX PEM form with 0o644
// permissions. Like WritePrivateKeyPEM, refuses to overwrite.
func WritePublicKeyPEM(path string, pub ed25519.PublicKey) error {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return fmt.Errorf("marshal PKIX: %w", err)
	}
	block := &pem.Block{Type: PublicKeyPEMType, Bytes: der}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return pem.Encode(f, block)
}

// LoadPrivateKey reads a PKCS8 PEM Ed25519 private key from path.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("no PEM block in private key file")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS8: %w", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not Ed25519 (got %T)", key)
	}
	return priv, nil
}

// LoadPublicKey reads a PKIX PEM Ed25519 public key from path.
func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("no PEM block in public key file")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKIX: %w", err)
	}
	pub, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not Ed25519 (got %T)", key)
	}
	return pub, nil
}
