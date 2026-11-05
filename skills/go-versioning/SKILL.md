---
name: go-versioning
description: Apply the full date/PR application identity when integrating or releasing apps using go-versioning, or when the user requests the shared CORE+YYYYMMDD.PR convention. Keep package-manager coordinates separate from displayed app versions.
---

# Full application versions

Use `CORE+YYYYMMDD.PR`, for example `0.8.0+20261105.16`, as the app/release
identity. Keep it intact in CLI version output, UI headers/footers, About pages,
API build metadata, version-bearing logs, release names, changelog release
headings and versioned artifact names. `0.8.0` is only the numeric core.
The immutable application tag is `v0.8.0+20261105.16`.

## Establish and carry the identity

- Use the actual final merged commit and PR from a trusted hosting event/API.
  Select `YYYYMMDD` from that commit's committer timestamp in
  `America/Sao_Paulo`, never from the build clock or a feature-branch commit.
  Inherit the core unless the app's reviewed bump policy changes it.
- Reuse go-versioning's root version/build metadata API and explicit-repository
  `gitversion` tooling. Carry `Info.Version` through presentation unchanged.
  Never discover the app version from a dependency's module version or an
  implicit current directory.
- Release builds inject the full identity, full commit SHA and actual RFC3339
  build timestamp. Verify the built app's version/display/API values and the
  exact local/remote tag-to-commit identity.
- Preserve explicit `CORE-snapshot+YYYYMMDD.PR` source builds and
  `0.0.0+unknown`; neither claims a release. Build metadata does not affect
  SemVer precedence: use ancestry and merge identity for ordering.

## Respect installation constraints

Language/package-manager coordinates are technical identifiers. Document each
required coordinate and map it to the full app identity and exact source commit;
do not substitute it for the app's displayed version. Go 1.27.3 does not treat
arbitrary `+metadata` tags as canonical module versions: resolution can select a
pseudo-version or a usable canonical module tag at the selected commit, and
`@latest` need not select the application tag. An exact full-tag revision query
such as `go get example.com/team/app@v0.8.0+20261105.16` can succeed and record
`v0.8.1-0.TIMESTAMP-HASH`; keep the displayed app version `0.8.0+20261105.16`.
Verify the actual resolved source commit and, where exposed, `Origin.Ref` and
`Origin.Hash`. Honor an explicit full-tag-only policy; never add a bare-core tag
or silently strip the suffix to fix module bookkeeping. A matching core alone
proves too little. Plain `go install` does not establish that full app metadata
was injected; verify the build/output separately and preserve unknown status.

For Go-specific source references, release verification and installation guidance,
read the [maintained policy](https://github.com/marcuoli/go-versioning/blob/main/docs/application-version-policy.md).
Do not automatically create a bare `vCORE` alias to satisfy a package manager.
Keep any separately approved module-release coordinates explicit.

## Overall build counters

Use the released shared `buildcounter` API/CLI for explicit build-time allocation,
not a PR, commit count or dependency version. Initialize one external authority
explicitly; no inferred historical baseline. Failed builds consume reservations;
retries of the same allocation request retain their original number/time. A new
compile invocation uses a new request ID. All cooperating checkouts/CI must use
the same live store; cloud-sync/independent stores cannot claim one global order.
Read the [counter contract](https://github.com/marcuoli/go-versioning/blob/main/docs/build-counters.md)
for failure/recovery and filesystem limits. Keep `BuildCounterID` provenance.

Prefer shared CLI linker flags and read-only `CurrentBuild()` so consumers do not
duplicate parsing/allocation logic. Unknown counters remain zero; an untagged app
can have a genuine counter while its release version remains `0.0.0+unknown`.
A renderer only displays those fields. Never allocate at application startup or
substitute commit time for an unknown actual build timestamp.

## Scope

Apply this to the requested app. Preserve published tags/releases and unrelated
work; do not mass-edit consumers. Check the owning repository's issue, review,
changelog and release requirements. This skill grants no permission to merge,
publish, rewrite history, create tags or message other chats.
