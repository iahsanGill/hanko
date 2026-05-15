# hanko

> Cryptographic seal for LLM eval results — reproducible, attested, content-addressed evaluation bundles.

`hanko` wraps an existing LLM evaluation harness (initially [lm-evaluation-harness](https://github.com/EleutherAI/lm-evaluation-harness)) and produces a verifiable bundle that anyone can independently check:

- **Canonical run context** — model digest, harness commit, runtime config, hardware fingerprint, all pinned
- **Determinism verification** — confirms batch-invariant kernels engaged, seeds honored, batch size pinned
- **Signed attestation** — in-toto v1 Statement wrapping an `EvalRun` predicate, signed via Sigstore keyless
- **OCI-native distribution** — bundles ship as content-addressed OCI artifacts in any registry

## Why

LLM eval scores cited in papers, leaderboards, and release notes today come with no proof. Run the same MMLU on the same Llama-3.1-8B twice and you get different numbers ([Thinking Machines, 2025](https://thinkingmachines.ai/blog/defeating-nondeterminism-in-llm-inference/)). Run it across two harnesses and you get different numbers. UC Berkeley showed in April 2026 that [eight major benchmarks can be exploited to 100% scores](https://rdi.berkeley.edu/blog/trustworthy-benchmarks-cont/) without solving any tasks.

`hanko` puts a cryptographic seal on eval results so the score in your README is verifiable, not vibes.

## Status

**Pre-alpha.** v0.1 scaffold under construction. See [scope.md](./docs/scope.md) for the MVP plan.

## Quick start

Not yet — check back at v0.1.0.

## License

Apache 2.0
