<!--
Thanks for opening a PR. A few notes:

  * Keep one logical change per PR. If you find yourself wanting to write
    "and also" in the summary, split it into two PRs.
  * The pre-commit hook (make install-hooks) runs gofmt, vet, build
    locally. CI also runs tests + golangci-lint v2.12.2.
  * If your change touches the EvalRun predicate schema, update
    spec/eval-run-v1.md and CHANGELOG.md in the same PR.
-->

## Summary

<!-- 1-3 sentences: what changed and why. The why matters more than the what. -->

## Test plan

<!-- How you verified this. Include actual commands when relevant. -->
- [ ] `make check` (fmt + build + test)
- [ ] If touching attest/bundle/runner: end-to-end test against an in-memory OCI registry passes
- [ ] If touching signing: keyless verifier tests still pass against the vendored Sigstore bundle

## Notes for reviewers

<!-- Anything unusual: a workaround, a deferred follow-up, a security consideration, an open question. -->
