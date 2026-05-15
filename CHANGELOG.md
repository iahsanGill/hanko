# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
While the project is pre-1.0 the on-wire schema may change between minor
versions; consumers should pin against the bundle version they tested.

## [Unreleased]

### Added

- **Linux GPU / CUDA / driver hardware probing** — `probeHardware()` now
  shells out to `nvidia-smi --query-gpu=name,driver_version
  --format=csv,noheader,nounits` (with a 5-second timeout) and parses
  the CUDA version off the human-readable banner, populating
  `Hardware.GPU`, `Hardware.GPUCount`, `Hardware.DriverVersion`, and
  `Hardware.CUDAVersion`. The probe seam is an injectable package-level
  function so tests don't need NVIDIA hardware. Missing `nvidia-smi` or
  a non-Linux host is treated as "no GPU" — not an error — so CPU-only
  and hosted-API runs still produce clean bundles.
- **Inspect AI adapter** (`pkg/runner/inspect`) — second harness adapter,
  registered as `inspect-ai`. Invokes `inspect eval <task> --model
  <provider>/<name> --log-format json --log-dir <out>`, parses the
  resulting `EvalLog v2` JSON, and populates `Results`,
  `Benchmark.HarnessCommit` (from `eval.revision.commit`),
  `Benchmark.TaskVersion`, and `Runtime.BackendVersion`
  (`inspect_ai/<version>` from `eval.packages`). Refuses to record runs
  whose `status` is not `success`. Provider prefix (`openai/`,
  `anthropic/`, `hf/`, …) is preserved when the caller passes an
  already-qualified `--model` and synthesized from `--backend + --model`
  otherwise.
- **Sigstore keyless signing** (`--sigstore` on `hanko run`) — replaces
  long-lived Ed25519 keys with an ephemeral keypair plus a 10-minute
  Fulcio-issued X.509 cert bound to the producer's OIDC identity, signed
  and logged to Rekor. OIDC token is sourced from `--sigstore-id-token`,
  `$SIGSTORE_ID_TOKEN`, or GitHub Actions ambient OIDC, in that order.
  The bundle is published under a new OCI layer media type
  `application/vnd.dev.hanko.evalrun.sigstore.v1+json`; v0.1 BYO-key
  bundles (`application/vnd.dev.hanko.evalrun.dsse.v1+json`) remain
  fully supported.
- **`hanko verify --certificate-identity --certificate-oidc-issuer`** —
  Sigstore-bundle verification path. Validates the Fulcio cert chain
  against the trusted root (fetched via TUF on first run), enforces
  cert-SAN + OIDC-issuer policy, requires Rekor inclusion proof and the
  SCT embedded in the cert, then validates the DSSE signature. Verifier
  picks the v0.1 or v0.2 path automatically based on the pulled layer's
  media type.
- **`.github/workflows/sigstore-demo.yml`** — manual end-to-end smoke
  test: signs a demo bundle via ambient GitHub Actions OIDC against the
  **Sigstore staging** instance, pushes to GHCR, then verifies in a
  separate job asserting the workflow's identity matches. Staging-only,
  so demo runs don't pollute the production Rekor log.
- Default OCI auth flows through the docker keychain, so
  `docker login ghcr.io` (or equivalent) is honored by `hanko run --output`
  and `hanko verify` transparently.

## [0.1.0] — 2026-05-16

The end-to-end pipeline ships. `hanko run` invokes an evaluation harness,
captures the canonical run context, builds an in-toto Statement carrying
an `EvalRun` predicate, signs it with a local Ed25519 key, and publishes
the result as an OCI artifact in any distribution-spec registry.
`hanko verify` pulls a bundle, validates the DSSE signature, decodes the
predicate, and prints a human-readable summary.

### Added

- **`EvalRun` v1 predicate spec** ([spec/eval-run-v1.md](./spec/eval-run-v1.md)) —
  field-by-field semantics for the in-toto predicate, plus a verification
  procedure. Draft; subject to refinement before stable.
- **`pkg/context`** — canonical run-context capture: model reference,
  benchmark / harness commit / task version, runtime config (seed, batch
  size, FP-determinism flags, batch-invariant kernels claim), hardware
  fingerprint, results, determinism check.
- **`pkg/runner` + `pkg/runner/lmeval`** — pluggable harness interface
  with an EleutherAI lm-evaluation-harness adapter (subprocess + JSON
  parse). Adapter populates `Results`, `Benchmark.HarnessCommit`,
  `Benchmark.TaskVersion`, and `Runtime.BackendVersion` from the
  harness's output.
- **`pkg/attest`** — in-toto v1 Statement builder, Ed25519 keypair
  generation and PKCS8 / PKIX PEM persistence, DSSE Pre-Authentication
  Encoding (PAE) sign/verify, `ParseStatement` with schema validation,
  `PredicateAs` for typed predicate decoding.
- **`pkg/bundle`** — OCI artifact builder using
  [google/go-containerregistry](https://github.com/google/go-containerregistry).
  Single-layer artifact with custom `artifactType`, content-addressable
  digests, push/pull against any distribution-spec registry.
- **`pkg/determinism`** — opt-in double-run score-equality check. Re-runs
  the harness with the same context, asserts the primary metric value
  matches to floating-point precision, records the outcome (passed +
  details) into the EvalRun predicate.
- **CLI** — `hanko version`, `hanko key gen --out`,
  `hanko run --model --task --backend --output --key --determinism-check`,
  `hanko verify <oci-url> --public-key`. End-to-end test exercises
  the full pipeline against an in-memory OCI registry.

### Known limitations

- Sigstore keyless signing (Fulcio + Rekor) is not yet wired; v0.1 uses
  bring-your-own ed25519 keys. The bundle format is identical, so an
  upgrade path to keyless adds only a new signer, not a new envelope.
- Only the lm-evaluation-harness adapter ships; HELM, Inspect AI, and
  DeepEval adapters are planned for v0.2.
- The harness commit is populated from lm-evaluation-harness's
  `git_hash` field; for a Python install without git metadata the field
  may be empty, which `attest.Build` rejects. Run hanko against a
  source-installed harness for now.
- Hardware probing only records the CPU label on macOS/Linux; GPU /
  CUDA / driver version probing lands in v0.2.

[0.1.0]: https://github.com/iahsanGill/hanko/releases/tag/v0.1.0
