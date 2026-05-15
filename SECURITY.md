# Security policy

`hanko` produces cryptographically signed eval bundles. Bugs in the
signing or verification paths can result in forged or unverifiable
attestations, so security issues are treated as the highest priority.

## Reporting a vulnerability

**Do not open a public GitHub issue.** Instead:

1. Use GitHub's **private vulnerability reporting** for this repository:
   <https://github.com/iahsanGill/hanko/security/advisories/new>.
2. If that's unavailable, email the maintainer privately (see the GitHub
   profile linked from CODEOWNERS) with a subject starting `[hanko
   security]`.

Please include:
- A description of the issue and its security impact.
- The smallest reproduction you can put together — a failing test case
  or a sequence of `hanko` commands is ideal.
- The affected version(s) (`hanko version` output, or commit SHA).

You should expect an acknowledgement within 72 hours. Where possible
we'll work with you on a coordinated disclosure timeline.

## Scope

In scope:

- The signing pipeline (`pkg/attest`, `pkg/bundle`) — anything that
  could produce a bundle that fails to bind the predicate to the
  signer's identity, or that could be forged.
- The verification pipeline (`hanko verify`, `pkg/attest.Verify`,
  `pkg/attest.VerifyKeyless`) — anything that could cause a bundle with
  an invalid signature, wrong identity, or missing Rekor entry to verify
  as valid.
- The Sigstore keyless path — including OIDC token handling, Fulcio
  cert validation, Rekor inclusion proof validation, SCT checks, and
  trusted-root handling.
- Supply chain: anything that could compromise the build, signing keys,
  or release artifacts.

Out of scope:

- Bugs in upstream Sigstore / go-containerregistry / TUF — report those
  to the upstream project.
- Issues that require the attacker to already have the producer's
  private key or active OIDC session.
- Denial of service against the hanko CLI itself (it's a single-user
  tool; rate limiting is not in its threat model).

## Supported versions

While the project is pre-1.0 only the latest minor version receives
fixes. Once 1.0 ships, the policy will move to the previous minor
release as well.

| Version | Supported |
|---------|-----------|
| 0.2.x   | ✅        |
| 0.1.x   | ❌        |

## Bundle / predicate schema

A security-impacting change to the on-wire bundle layout or the
`EvalRun` predicate fields is treated as a breaking change. The version
in `payloadType` / the bundle's layer media type will be bumped, and the
old version will continue to verify (we don't silently change schemas
under existing version strings).
