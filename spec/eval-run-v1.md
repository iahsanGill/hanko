# `EvalRun` Predicate, v1

**Type URI:** `https://withhanko.dev/EvalRun/v1`

Status: **draft** (pre-v0.1; subject to change before first stable release).

An `EvalRun` predicate is an [in-toto v1 Statement](https://github.com/in-toto/attestation/blob/main/spec/v1/statement.md) predicate describing a single execution of an LLM evaluation harness against a specific model. It pins every input that could affect the result so the run is independently reproducible (or, where deterministic execution is not possible, the conditions are recorded).

## Subject

The Statement's `subject` array carries one entry per evaluation task that was run. `subject[i].name` is the task identifier as understood by the harness (e.g. `lm-eval/mmlu`). `subject[i].digest` is the content hash of the task definition as loaded by the harness at runtime — typically the SHA-256 of the canonical task YAML or the harness-specific task file, recorded so that consumers can detect silent task drift.

## Predicate schema

```json
{
  "model": {
    "ref": "meta-llama/Llama-3.1-8B",
    "digest": "sha256:abcd1234...",
    "source": "huggingface"
  },
  "benchmark": {
    "harness": "lm-evaluation-harness",
    "harnessVersion": "0.4.5",
    "harnessCommit": "1a2b3c4d5e6f7890...",
    "task": "mmlu",
    "taskVersion": "v3"
  },
  "runtime": {
    "seed": 42,
    "batchSize": 32,
    "temperature": 0.0,
    "topP": 1.0,
    "fpDeterminism": true,
    "batchInvariantKernels": true,
    "backend": "vllm",
    "backendVersion": "0.6.5"
  },
  "hardware": {
    "gpu": "NVIDIA H100",
    "gpuCount": 8,
    "cudaVersion": "12.4",
    "driverVersion": "550.54.15",
    "cpu": "Intel Xeon Platinum 8480+"
  },
  "results": {
    "primaryMetric": "accuracy",
    "primaryValue": 0.683,
    "primaryStderr": 0.0042,
    "perTask": {
      "mmlu_abstract_algebra": 0.42,
      "mmlu_anatomy": 0.71
    }
  },
  "determinismCheck": {
    "performed": true,
    "method": "double-run-byte-equal",
    "passed": true,
    "details": "Re-ran 16 random questions; outputs byte-identical."
  },
  "startedAt": "2026-05-16T08:00:00Z",
  "completedAt": "2026-05-16T09:02:22Z",
  "durationSeconds": 3742,
  "hankoVersion": "0.1.0"
}
```

## Field semantics

### `model` (object, required)

Pins the model under evaluation.

- `ref` (string, required): canonical reference as understood by the source. For HuggingFace, `org/name`. For local paths, a stable identifier (e.g. `local:/path/relative-to-root`). For Ollama, `model:tag`.
- `digest` (string, required for `huggingface` and `local`): SHA-256 of the model artifact set. For HuggingFace, the SHA-256 of the canonical tree manifest of the repository at the resolved revision. For local paths, the SHA-256 of the directory's [model-transparency manifest](https://github.com/sigstore/model-transparency).
- `source` (string, required): one of `huggingface`, `local`, `ollama`, `s3`, `gs`. Determines how `ref` and `digest` are interpreted.

### `benchmark` (object, required)

Pins the eval harness and the task within it.

- `harness` (string, required): canonical short name of the harness. `lm-evaluation-harness`, `helm`, `inspect-ai`, `deepeval`, `promptfoo`, `openai-evals`. Custom harnesses use a vendor-prefixed identifier (e.g. `acme/internal-harness`).
- `harnessVersion` (string, optional): the harness's self-reported semver, if available.
- `harnessCommit` (string, required): the git commit SHA of the harness source tree at runtime. Establishes a content-address for the harness code.
- `task` (string, required): task identifier within the harness (`mmlu`, `gsm8k`, `humaneval`, etc.).
- `taskVersion` (string, optional): task version reported by the harness, where applicable.

### `runtime` (object, required)

Captures every runtime knob that can affect output.

- `seed` (integer, required): random seed in effect.
- `batchSize` (integer, required): inference batch size. A pinned batch size is necessary (but not sufficient) for deterministic inference; see [batchInvariantKernels](#batchinvariantkernels-boolean-required).
- `temperature`, `topP` (number, required): sampling parameters.
- `fpDeterminism` (boolean, required): whether floating-point determinism env flags were set (e.g. `CUBLAS_WORKSPACE_CONFIG=:4096:8`, `torch.use_deterministic_algorithms(True)`).
- `batchInvariantKernels` (boolean, required): whether batch-invariant inference kernels were engaged. See [Thinking Machines, 2025](https://thinkingmachines.ai/blog/defeating-nondeterminism-in-llm-inference/). When `true`, the run is reproducible across changes in batch context (concurrent requests, GPU count). When `false`, the score is a property of the run, not the model.
- `backend` (string, required): inference backend (`vllm`, `sglang`, `transformers`, `tgi`, `tensorrt-llm`, `llama.cpp`, `ollama`).
- `backendVersion` (string, required): the backend's reported version.

### `hardware` (object, required)

Hardware fingerprint at runtime. Optional fields may be omitted on inference backends that abstract them away (e.g. hosted APIs).

- `gpu`, `gpuCount`, `cudaVersion`, `driverVersion`, `cpu` (strings/integer, optional).

For hosted API providers where hardware is opaque, set `hardware.gpu = "hosted-api"` and add `hardware.provider` (e.g. `openai`, `anthropic`, `together`).

### `results` (object, required)

The eval scores. Schema is harness-aware:

- `primaryMetric` (string, required): canonical name of the headline metric (`accuracy`, `pass@1`, `exact_match`).
- `primaryValue` (number, required): headline score.
- `primaryStderr` (number, optional): standard error if the harness reports one.
- `perTask` (object, optional): map of sub-task name → score for benchmarks with multiple sub-tasks (e.g. MMLU's 57 subjects).
- `raw` (object, optional): the harness's full raw results document, for consumers who want full fidelity.

### `determinismCheck` (object, required)

Records whether `hanko` actively verified determinism by re-running a subset.

- `performed` (boolean, required): whether the check was attempted.
- `method` (string, required if `performed`): one of:
  - `double-run-byte-equal`: re-ran N questions, asserted byte-identical model outputs.
  - `double-run-score-equal`: re-ran the full eval, asserted identical aggregate score.
  - `skipped`: not performed.
- `passed` (boolean, required if `performed`): whether the assertion held.
- `details` (string, optional): human-readable summary.

### Timestamps

- `startedAt`, `completedAt` (RFC 3339 strings, required): wall-clock bracketing the run.
- `durationSeconds` (integer, required): convenience field; equal to `completedAt - startedAt` in seconds.

### `hankoVersion` (string, required)

Version of the `hanko` tool that produced this predicate. Allows consumers to know what conventions to expect.

## DSSE envelope

Statements are signed using [DSSE](https://github.com/secure-systems-lab/dsse) with a `payloadType` of `application/vnd.in-toto+json`.

## Identity binding

Signatures use [Sigstore keyless](https://docs.sigstore.dev/cosign/signing/overview/) by default, binding to the producer's OIDC identity (GitHub Actions Workload Identity, Google Workload Identity, etc.). Long-lived keys are supported via `--key`.

## Verification semantics

A verifier MUST:

1. Validate the DSSE signature against the claimed identity.
2. Validate the Statement schema (in-toto v1).
3. Validate the predicate matches this schema.
4. Surface `determinismCheck.passed = false` prominently; a non-deterministic run is not a trust failure but consumers must know.

A verifier SHOULD:

1. Compare `model.digest` against an independent registry resolution if possible.
2. Compare `benchmark.harnessCommit` against the upstream harness repository.
3. Reject the bundle if `runtime.batchInvariantKernels = false` AND `determinismCheck.passed = false` AND the consumer's policy requires deterministic results.

## Open questions before stable

- Should `subject` carry the dataset hash directly, or is harness commit + task version sufficient?
- Should `results.raw` be a separate OCI layer to keep the Statement small?
- Schema for hosted-API runs where hardware is opaque — is `provider` + `model.ref` enough?
- Predicate type registration with [in-toto/attestation](https://github.com/in-toto/attestation/tree/main/spec/predicates) — pursue before or after v0.2?

Feedback welcome via GitHub issue.
