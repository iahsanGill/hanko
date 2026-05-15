# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
While the project is pre-1.0 the on-wire schema may change between minor
versions; consumers should pin against the bundle version they tested.

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
