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
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	sgbundle "github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
	"google.golang.org/protobuf/encoding/protojson"
)

// KeylessSignOptions configures the Sigstore keyless signing flow.
type KeylessSignOptions struct {
	// IDToken is the OIDC token Fulcio will use to mint a short-lived
	// signing cert. If empty, ResolveOIDCToken() is consulted to source
	// one from the environment.
	IDToken string

	// UseStaging routes signing through Sigstore's staging Fulcio/Rekor
	// instances. Set this for tests so we don't write to production Rekor
	// (the public-good log is permanent).
	UseStaging bool

	// Timeout bounds each Fulcio/Rekor call (default 90s).
	Timeout time.Duration
}

// KeylessVerifyOptions configures the Sigstore keyless verification flow.
type KeylessVerifyOptions struct {
	// Identity is the expected SAN value (email or workflow URI) on the
	// signing cert. Either Identity or IdentityRegex must be set.
	Identity      string
	IdentityRegex string

	// Issuer is the expected OIDC issuer URL claim on the signing cert.
	// Either Issuer or IssuerRegex must be set.
	Issuer      string
	IssuerRegex string

	// UseStaging selects the staging trusted root for verification. Must
	// match the instance used for signing.
	UseStaging bool

	// TrustedRootJSON overrides the trusted root used to verify the cert
	// chain. When set, no TUF roundtrip happens. Set this in unit tests
	// that operate on canned bundles.
	TrustedRootJSON []byte
}

// SignKeyless signs payload via Sigstore keyless:
//
//  1. generate an ephemeral keypair,
//  2. request a code-signing cert from Fulcio (proving identity via OIDC),
//  3. sign the DSSE PAE bytes,
//  4. record a transparency log entry on Rekor.
//
// The returned bytes are a JSON-serialized Sigstore protobuf bundle
// (mediaType `application/vnd.dev.sigstore.bundle.v0.3+json`).
func SignKeyless(ctx context.Context, payload []byte, payloadType string, opts KeylessSignOptions) ([]byte, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 90 * time.Second
	}

	token := opts.IDToken
	if token == "" {
		t, err := ResolveOIDCToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve OIDC token: %w", err)
		}
		token = t
	}
	if token == "" {
		return nil, errors.New("no OIDC token (set --sigstore-id-token, SIGSTORE_ID_TOKEN, or run in a workflow with id-token: write)")
	}

	tufOpts := tuf.DefaultOptions()
	if opts.UseStaging {
		tufOpts.Root = tuf.StagingRoot()
		tufOpts.RepositoryBaseURL = tuf.StagingMirror
	}
	tufClient, err := tuf.New(tufOpts)
	if err != nil {
		return nil, fmt.Errorf("init TUF client: %w", err)
	}
	trustedRoot, err := root.GetTrustedRoot(tufClient)
	if err != nil {
		return nil, fmt.Errorf("fetch trusted root: %w", err)
	}
	signingConfig, err := root.GetSigningConfig(tufClient)
	if err != nil {
		return nil, fmt.Errorf("fetch signing config: %w", err)
	}

	fulcioSvc, err := root.SelectService(signingConfig.FulcioCertificateAuthorityURLs(), sign.FulcioAPIVersions, time.Now())
	if err != nil {
		return nil, fmt.Errorf("select Fulcio service: %w", err)
	}
	rekorSvcs, err := root.SelectServices(signingConfig.RekorLogURLs(), signingConfig.RekorLogURLsConfig(), sign.RekorAPIVersions, time.Now())
	if err != nil {
		return nil, fmt.Errorf("select Rekor service: %w", err)
	}

	kp, err := sign.NewEphemeralKeypair(nil)
	if err != nil {
		return nil, fmt.Errorf("ephemeral keypair: %w", err)
	}

	bundleOpts := sign.BundleOptions{
		Context:     ctx,
		TrustedRoot: trustedRoot,
		CertificateProvider: sign.NewFulcio(&sign.FulcioOptions{
			BaseURL: fulcioSvc.URL,
			Timeout: opts.Timeout,
			Retries: 1,
		}),
		CertificateProviderOptions: &sign.CertificateProviderOptions{IDToken: token},
	}
	for _, svc := range rekorSvcs {
		bundleOpts.TransparencyLogs = append(bundleOpts.TransparencyLogs,
			sign.NewRekor(&sign.RekorOptions{
				BaseURL: svc.URL,
				Timeout: opts.Timeout,
				Retries: 1,
				Version: svc.MajorAPIVersion,
			}))
	}

	pb, err := sign.Bundle(&sign.DSSEData{Data: payload, PayloadType: payloadType}, kp, bundleOpts)
	if err != nil {
		return nil, fmt.Errorf("sigstore sign: %w", err)
	}
	return protojson.Marshal(pb)
}

// VerifyKeyless validates a serialized Sigstore bundle and returns the
// verified DSSE payload bytes. It checks: cert chain to the trusted root,
// SAN matches expected identity, issuer matches expected issuer, Rekor
// inclusion proof + transparency-log threshold, signed certificate
// timestamps (CT log), and the DSSE signature itself.
//
// Returns the inner payload bytes — typically an in-toto Statement JSON.
func VerifyKeyless(bundleJSON []byte, opts KeylessVerifyOptions) ([]byte, error) {
	if opts.Identity == "" && opts.IdentityRegex == "" {
		return nil, errors.New("identity required: set --certificate-identity or --certificate-identity-regex")
	}
	if opts.Issuer == "" && opts.IssuerRegex == "" {
		return nil, errors.New("issuer required: set --certificate-oidc-issuer or --certificate-oidc-issuer-regex")
	}

	var pb protobundle.Bundle
	if err := protojson.Unmarshal(bundleJSON, &pb); err != nil {
		return nil, fmt.Errorf("decode sigstore bundle: %w", err)
	}
	b, err := sgbundle.NewBundle(&pb)
	if err != nil {
		return nil, fmt.Errorf("validate sigstore bundle: %w", err)
	}

	var trustedRootJSON []byte
	switch {
	case len(opts.TrustedRootJSON) > 0:
		trustedRootJSON = opts.TrustedRootJSON
	default:
		tufOpts := tuf.DefaultOptions()
		if opts.UseStaging {
			tufOpts.Root = tuf.StagingRoot()
			tufOpts.RepositoryBaseURL = tuf.StagingMirror
		}
		tufClient, err := tuf.New(tufOpts)
		if err != nil {
			return nil, fmt.Errorf("init TUF client: %w", err)
		}
		trustedRootJSON, err = tufClient.GetTarget("trusted_root.json")
		if err != nil {
			return nil, fmt.Errorf("fetch trusted root via TUF: %w", err)
		}
	}

	trustedRoot, err := root.NewTrustedRootFromJSON(trustedRootJSON)
	if err != nil {
		return nil, fmt.Errorf("parse trusted root: %w", err)
	}

	certID, err := verify.NewShortCertificateIdentity(opts.Issuer, opts.IssuerRegex, opts.Identity, opts.IdentityRegex)
	if err != nil {
		return nil, fmt.Errorf("build identity matcher: %w", err)
	}

	verifier, err := verify.NewVerifier(trustedRoot,
		verify.WithSignedCertificateTimestamps(1),
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1),
	)
	if err != nil {
		return nil, fmt.Errorf("new verifier: %w", err)
	}

	policy := verify.NewPolicy(verify.WithoutArtifactUnsafe(), verify.WithCertificateIdentity(certID))
	if _, err := verifier.Verify(b, policy); err != nil {
		return nil, fmt.Errorf("sigstore verify: %w", err)
	}

	env, err := b.Envelope()
	if err != nil {
		return nil, fmt.Errorf("extract envelope: %w", err)
	}
	if env.PayloadType != PayloadTypeURI {
		return nil, fmt.Errorf("payload type %q (want %q)", env.PayloadType, PayloadTypeURI)
	}
	raw, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	return raw, nil
}

// ResolveOIDCToken sources an OIDC identity token for keyless signing in
// this order:
//
//  1. SIGSTORE_ID_TOKEN env var (matches cosign convention).
//  2. GitHub Actions ambient OIDC, when ACTIONS_ID_TOKEN_REQUEST_URL is set
//     (i.e. a workflow with `permissions: id-token: write`).
//
// Returns ("", nil) when no source is configured; the caller decides
// whether that's an error.
func ResolveOIDCToken(ctx context.Context) (string, error) {
	if t := os.Getenv("SIGSTORE_ID_TOKEN"); t != "" {
		return t, nil
	}
	return fetchGitHubActionsOIDC(ctx)
}

// fetchGitHubActionsOIDC requests an ID token from the GitHub Actions
// OIDC endpoint, with `audience=sigstore` per the Sigstore convention.
// Returns ("", nil) when not running inside a GitHub Actions workflow.
func fetchGitHubActionsOIDC(ctx context.Context) (string, error) {
	endpoint := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	bearer := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if endpoint == "" || bearer == "" {
		return "", nil
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse OIDC endpoint: %w", err)
	}
	q := u.Query()
	q.Set("audience", "sigstore")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build OIDC request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+bearer)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OIDC request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read OIDC response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC endpoint returned %d: %s", resp.StatusCode, body)
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode OIDC response: %w", err)
	}
	return out.Value, nil
}
