# hanko

> Cryptographic seal for LLM eval results — reproducible, attested, content-addressed evaluation bundles.

`hanko` wraps an existing LLM evaluation harness (initially [lm-evaluation-harness](https://github.com/EleutherAI/lm-evaluation-harness)) and produces a verifiable bundle that anyone can independently check:

- **Canonical run context** — model digest, harness commit, runtime config, hardware fingerprint, all pinned
- **Determinism verification** — opt-in double-run check that asserts the aggregate primary score is byte-equal across two invocations
- **Signed attestation** — in-toto v1 Statement wrapping an `EvalRun` predicate, signed in a DSSE envelope with a local Ed25519 key
- **OCI-native distribution** — bundles ship as content-addressed OCI artifacts in any registry (GHCR, ECR, GAR, self-hosted, …)

## Why

LLM eval scores cited in papers, leaderboards, and release notes today come with no proof. Run the same MMLU on the same Llama-3.1-8B twice and you can get different numbers ([Thinking Machines, 2025](https://thinkingmachines.ai/blog/defeating-nondeterminism-in-llm-inference/)). Run it across two harnesses and you get different numbers. UC Berkeley showed in April 2026 that [eight major agent benchmarks can be exploited to 100% scores](https://rdi.berkeley.edu/blog/trustworthy-benchmarks-cont/) without solving any tasks.

`hanko` puts a cryptographic seal on eval results so the score in your README is verifiable, not vibes.

## Status

**v0.2** ships: Sigstore keyless signing, a second harness adapter
(Inspect AI), and Linux GPU/CUDA/driver hardware probing on top of the
v0.1 end-to-end pipeline.

| Capability | Status |
|---|---|
| `EvalRun` in-toto v1 predicate spec | ✅ |
| Canonical run-context capture (model · benchmark · runtime · hardware) | ✅ |
| lm-evaluation-harness adapter | ✅ |
| Ed25519 keypair generation | ✅ |
| DSSE-signed attestation + OCI artifact push | ✅ |
| `hanko verify` against a published bundle | ✅ |
| `--determinism-check` (double-run aggregate-score equality) | ✅ |
| Sigstore keyless signing (Fulcio + Rekor) | ✅ (v0.2) |
| Inspect AI adapter | ✅ (v0.2) |
| GPU / CUDA / driver hardware probing (Linux) | ✅ (v0.2) |
| HELM / DeepEval adapters | planned post-v0.2 |
| BenchJack integrity-guard integration | depends on upstream release |

## Quick start

```sh
# 1. Mint a signing keypair (PKCS8 PEM private, PKIX PEM public).
hanko key gen --out hanko.key

# 2. Run an eval, sign the result, push as an OCI artifact.
hanko run \
  --model meta-llama/Llama-3.1-8B \
  --task mmlu \
  --backend vllm \
  --determinism-check \
  --output oci://ghcr.io/you/evals/llama-3.1-8b-mmlu:v1 \
  --key hanko.key

# 3. Anyone can independently verify the published bundle.
hanko verify oci://ghcr.io/you/evals/llama-3.1-8b-mmlu:v1 \
  --public-key hanko.key.pub
```

Sample `verify` output:

```
Verified bundle: ghcr.io/you/evals/llama-3.1-8b-mmlu:v1

  Model:     meta-llama/Llama-3.1-8B (huggingface)
  Benchmark: lm-evaluation-harness/mmlu @ commit 1a2b3c4d5e6f
  Runtime:   vllm transformers/4.40.0 · seed=42 · batch=32 · batch-invariant=true
  Hardware:  darwin/arm64/10-core
  Result:    acc = 0.6826 (stderr 0.0040)
  Determinism: passed (double-run-score-equal)
```

### Keyless (Sigstore) signing — v0.2

No keypair, no public key exchange. `hanko run --sigstore` mints an
ephemeral keypair, requests a 10-minute Fulcio cert bound to the
producer's OIDC identity, signs the DSSE envelope, and logs the
signature to Rekor. Verifiers check the cert chain + identity claim
against the Sigstore trusted root.

```sh
# In a CI job with `permissions: id-token: write`, hanko picks up the
# ambient OIDC token automatically — no flag plumbing needed.
hanko run \
  --model meta-llama/Llama-3.1-8B \
  --task mmlu \
  --backend vllm \
  --sigstore \
  --output oci://ghcr.io/you/evals/llama-3.1-8b-mmlu:v1

# Verify with the expected workflow identity + GitHub Actions issuer.
hanko verify oci://ghcr.io/you/evals/llama-3.1-8b-mmlu:v1 \
  --certificate-identity "https://github.com/you/repo/.github/workflows/eval.yml@refs/heads/main" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

A full end-to-end smoke test (sign on staging Sigstore + push to GHCR +
verify) lives in [.github/workflows/sigstore-demo.yml](./.github/workflows/sigstore-demo.yml).

### Other modes

```sh
# Dry-run: capture the run context without invoking the harness.
hanko run --model meta-llama/Llama-3.1-8B --task mmlu --dry-run

# Inspect mode: run the harness but skip signing/publishing.
hanko run --model meta-llama/Llama-3.1-8B --task mmlu --backend vllm
```

## Predicate schema

The signed payload is an in-toto v1 Statement carrying an `EvalRun` predicate. See [spec/eval-run-v1.md](./spec/eval-run-v1.md) for the field-by-field semantics. The schema is in draft and may change before stable; consumers verifying bundles should pin against the version they tested.

## Architecture

```
┌───────────────────────────────────────────────────────────┐
│  EVAL HARNESSES (run the actual eval)                     │
│  lm-evaluation-harness · (HELM · Inspect AI · …)          │
└───────────────────────────────────────────────────────────┘
                          ↓ hanko ↓
┌───────────────────────────────────────────────────────────┐
│  ATTESTATION ECOSYSTEM (sign · verify · distribute)       │
│  in-toto · DSSE · Sigstore · OCI registries               │
└───────────────────────────────────────────────────────────┘
```

`pkg/runner` defines the harness seam; `pkg/runner/lmeval` implements the lm-evaluation-harness adapter (subprocess + JSON parse). `pkg/attest` builds the in-toto Statement and produces a DSSE-signed envelope. `pkg/bundle` packages the envelope as a single-layer OCI artifact. `pkg/determinism` implements the double-run score-equality check.

## License

Apache 2.0
