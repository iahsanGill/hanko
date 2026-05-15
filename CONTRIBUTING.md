# Contributing to hanko

Thanks for considering a contribution. This guide covers the practical
mechanics — branching, testing, signing your work, and the kinds of
changes that need extra care.

## Workflow at a glance

1. Open an issue first if the change is non-trivial — feature request or
   bug report templates are on the [New issue](https://github.com/iahsanGill/hanko/issues/new/choose)
   page. Small fixes (typos, doc nits) can skip straight to a PR.
2. Branch from `main` using a descriptive prefix:
   - `feat/<short-name>` — new capability
   - `fix/<short-name>` — bug fix
   - `chore/<short-name>` — refactors, deps, tooling
   - `docs/<short-name>` — docs-only
3. Make commits that compile and pass tests at each step. We squash on
   merge, so the final main-branch commit will be one per PR — but a
   clean intermediate history makes review easier.
4. Open a PR against `main`. CI runs `go test -race`, `go vet`, `gofmt`,
   and `golangci-lint v2.12.2`. The pre-commit hook (`make install-hooks`)
   runs the cheap subset locally, before each commit.
5. A maintainer reviews. We'll either merge it (squash) or ask for
   changes. Merged PRs auto-delete their branches.

## Local development

```sh
make build          # go build ./...
make test           # go test ./... -race -count=1
make lint           # golangci-lint via docker (matches CI's v2.12.2)
make fmt            # gofmt -w
make check          # fmt + build + test (what CI runs)
make install-hooks  # symlink the pre-commit hook into .git/hooks
```

Tests use an in-memory OCI registry (`go-containerregistry`'s `registry.New()`)
and a fake harness execer (`pkg/runner/lmeval/lmeval_test.go`), so the
full suite runs offline — no Python install or network needed.

## Surfaces that need extra care

| Path | Why it's sensitive |
|---|---|
| `pkg/attest/` | Signing + verification primitives. A subtle bug here breaks the entire trust story. Bring tests. |
| `pkg/bundle/` | OCI layer media types are part of the on-wire contract. Adding a new media type is a spec change. |
| `spec/eval-run-v1.md` | The predicate schema. Any field rename / type change is a breaking schema change while pre-1.0; bump `hankoVersion` in the example and add a `[Unreleased]` note. |
| `.github/workflows/` | CI permissions and OIDC scopes. A change here can expand secret blast radius. |

If your PR touches `pkg/attest/` or `pkg/bundle/`, update
[CHANGELOG.md](./CHANGELOG.md)'s `[Unreleased]` section in the same PR.

## Commit messages

Format: `type: short imperative summary` matching the existing log.
Common types: `feat`, `fix`, `chore`, `docs`, `ci`, `style`, `test`.

The first line is what shows up in `git log --oneline`, so keep it
under ~70 characters. Use the body for the "why" — what motivated the
change, what alternatives you ruled out, any non-obvious tradeoffs.

## Adding a new harness adapter

The runner interface is in `pkg/runner/runner.go`. The lm-eval adapter
(`pkg/runner/lmeval/`) is the reference implementation:

1. New package under `pkg/runner/<harness>/`.
2. Implement `Name()` and `Run(ctx, *RunContext, RunOptions) error`.
3. Register via `init()`.
4. Add a side-effect import in `cmd/hanko/cli/run.go`.
5. Tests with a fake execer + fixture results JSON.
6. Document the harness identifier in `spec/eval-run-v1.md`'s
   `benchmark.harness` allowed values.

## Reporting security issues

See [SECURITY.md](./SECURITY.md). **Do not** open a public issue for a
vulnerability.

## License

By contributing you agree your work is licensed under Apache 2.0
(see [LICENSE](./LICENSE)).
