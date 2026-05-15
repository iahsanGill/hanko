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

package bundle

import (
	"bytes"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
)

// TestPushPull_Roundtrip stands up an in-memory OCI registry, pushes a
// fake DSSE envelope as a hanko bundle, and pulls it back. This is the
// load-bearing test for the bundle package: if push/pull diverge on
// layer count or media type the round-trip fails immediately.
func TestPushPull_Roundtrip(t *testing.T) {
	srv := httptest.NewServer(registry.New())
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ref := u.Host + "/hanko/evals/mmlu:latest"

	envBytes := []byte(`{"payloadType":"application/vnd.in-toto+json","payload":"eyJfdHlwZSI6IngifQ==","signatures":[{"sig":"AA=="}]}`)

	digest, err := Push(envBytes, ref)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if digest.Hex == "" {
		t.Error("Push returned empty digest")
	}

	got, err := Pull(ref)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if !bytes.Equal(got, envBytes) {
		t.Errorf("pull returned different bytes; got %q, want %q", got, envBytes)
	}

	// Also pull by digest to confirm content-addressed access works.
	gotByDigest, err := Pull(u.Host + "/hanko/evals/mmlu@" + digest.String())
	if err != nil {
		t.Fatalf("Pull by digest: %v", err)
	}
	if !bytes.Equal(gotByDigest, envBytes) {
		t.Error("pull-by-digest returned different bytes")
	}
}

func TestPush_AcceptsOCIScheme(t *testing.T) {
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	ref := "oci://" + u.Host + "/scheme/test:v1"

	_, err := Push([]byte(`{"k":"v"}`), ref)
	if err != nil {
		t.Fatalf("Push with oci:// scheme: %v", err)
	}
}

func TestPull_RejectsMissingArtifact(t *testing.T) {
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	if _, err := Pull(u.Host + "/never/pushed:nope"); err == nil {
		t.Error("expected error pulling a non-existent ref")
	}
}

func TestPush_InvalidRef(t *testing.T) {
	_, err := Push([]byte(`{}`), "not a valid ref at all!!")
	if err == nil {
		t.Error("expected error from invalid ref")
	}
	if !strings.Contains(err.Error(), "parse ref") {
		t.Errorf("error should mention parse failure; got: %v", err)
	}
}
