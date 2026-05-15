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

// Package bundle packages a hanko DSSE envelope as a single-layer OCI
// artifact and pushes/pulls it via go-containerregistry. We use the OCI
// artifact pattern (manifest with custom artifactType, no config blob)
// so the result is content-addressable and storable in any
// distribution-spec-compliant registry — GHCR, ECR, GAR, self-hosted.
package bundle

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// ArtifactType is the OCI artifactType set on the manifest. Consumers
// can filter for hanko bundles by matching this string against the
// manifest's artifactType field.
const ArtifactType = "application/vnd.dev.hanko.evalrun.v1+json"

// LayerMediaType is the OCI media type stamped on the single layer that
// carries the DSSE envelope JSON bytes.
const LayerMediaType = "application/vnd.dev.hanko.evalrun.dsse.v1+json"

// StripScheme removes a leading oci:// from a reference. Exported so the
// CLI can display the same canonicalized form Push/Pull operate on. The
// go-containerregistry parser doesn't understand the scheme, so we strip
// it before parsing.
func StripScheme(ref string) string {
	return strings.TrimPrefix(ref, "oci://")
}

// Push uploads the DSSE envelope bytes as a single-layer OCI artifact
// at ref. ref may be a tagged reference (ghcr.io/u/r:latest) or a
// repository (in which case the artifact is pushed under :latest).
//
// The returned descriptor includes the manifest digest, suitable for
// pinning the produced bundle (oci://...@sha256:...).
func Push(envBytes []byte, ref string, opts ...remote.Option) (v1.Hash, error) {
	parsedRef, err := name.ParseReference(StripScheme(ref))
	if err != nil {
		return v1.Hash{}, fmt.Errorf("parse ref %q: %w", ref, err)
	}

	layer := static.NewLayer(envBytes, LayerMediaType)

	img, err := mutate.Append(empty.Image, mutate.Addendum{Layer: layer})
	if err != nil {
		return v1.Hash{}, fmt.Errorf("append layer: %w", err)
	}
	// Mark the image as an OCI artifact, not a runnable image. Without
	// this push would still succeed but a clued-in consumer can't
	// distinguish hanko bundles from other artifacts in the registry.
	img = mutate.MediaType(img, types.OCIManifestSchema1)
	img = mutate.ConfigMediaType(img, types.MediaType(ArtifactType))

	if err := remote.Write(parsedRef, img, opts...); err != nil {
		return v1.Hash{}, fmt.Errorf("push %s: %w", parsedRef, err)
	}

	digest, err := img.Digest()
	if err != nil {
		return v1.Hash{}, fmt.Errorf("compute digest: %w", err)
	}
	return digest, nil
}

// Pull retrieves the DSSE envelope bytes from the OCI artifact at ref.
// ref may carry a tag or a digest (the @sha256:... form), in either
// case we expect exactly one layer carrying the envelope.
func Pull(ref string, opts ...remote.Option) ([]byte, error) {
	parsedRef, err := name.ParseReference(StripScheme(ref))
	if err != nil {
		return nil, fmt.Errorf("parse ref %q: %w", ref, err)
	}
	img, err := remote.Image(parsedRef, opts...)
	if err != nil {
		return nil, fmt.Errorf("pull %s: %w", parsedRef, err)
	}
	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("list layers: %w", err)
	}
	if len(layers) != 1 {
		return nil, fmt.Errorf("expected exactly 1 layer, got %d", len(layers))
	}
	rc, err := layers[0].Uncompressed()
	if err != nil {
		return nil, fmt.Errorf("read layer: %w", err)
	}
	defer func() { _ = rc.Close() }()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc); err != nil {
		return nil, fmt.Errorf("drain layer: %w", err)
	}
	return buf.Bytes(), nil
}
