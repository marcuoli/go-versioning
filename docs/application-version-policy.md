# Full application version identity

The application/release identity is `MAJOR.MINOR.PATCH+YYYYMMDD.PR`.
For example, `0.8.0+20261105.16` identifies one release; `0.8.0` is its numeric
core. Keep the complete identity wherever an app version is presented.

| Surface | Example or required value |
| --- | --- |
| CLI version output, UI header/footer, About page | `0.8.0+20261105.16` |
| API/build metadata and version-bearing logs | The same complete value |
| Release name and changelog release heading | Include `0.8.0+20261105.16` |
| Immutable application Git tag | `v0.8.0+20261105.16` |
| Versioned artifact name | `app_0.8.0+20261105.16_windows_amd64.zip` |
| Numeric core for bump policy or a required technical field | `0.8.0`, explicitly identified as the core |

A language, package manager or file format may restrict a technical coordinate.
Record that restriction and its mapping to the full application identity and
exact source commit. Do not present the restricted coordinate as another app
version or remove the suffix from human-facing release information. Escape a
version when a transport requires it; retain the original complete value.

## Allocate from the actual merge

- Read the final integrated commit's committer timestamp and select its calendar
  date in `America/Sao_Paulo`. The build clock, feature-branch commit and hosting
  server's display timezone do not select the date.
- Obtain the positive PR number and final merge commit from a trusted merged-PR
  event or the hosting API. A branch, issue, workflow run or guessed next PR
  number is not merge evidence. The CLI validates Git state but does not query
  the hosting API itself.
- Inherit the numeric core unless the app's reviewed major/minor/patch policy
  selects a bump. Keep the existing first-parent allocation and missed-event
  recovery rules in [workflow examples](workflows.md).
- Verify that `v<full identity>` resolves to that exact merged commit locally and
  remotely. An idempotent rerun must agree on both name and commit. Never move,
  delete or reuse a published tag to fix a version.

The example date/PR above is synthetic; it is not an instruction to allocate a
particular tag in any repository. Existing published tags and releases remain
immutable when adopting this policy.

## Carry the identity into the app

Use the root package's `Info.Version` without shortening it. Release builds inject
the full version, full commit SHA and actual RFC3339 build timestamp. Verify the
built app's version output and relevant display/API surfaces against the selected
tag and commit. Never derive an app's own version from a dependency's module
version or from whichever repository happens to be the current directory.

`gitversion` takes an explicit repository context. Source inspection deliberately
produces `CORE-snapshot+YYYYMMDD.PR`, including at a release tag; a production
release requires verified injected metadata. `0.0.0+unknown` remains the explicit
unknown identity. Neither snapshots nor unknown builds may claim a release.
Build metadata does not alter SemVer precedence; use Git ancestry and merge
identity rather than sorting full versions as ordinary strings.

## Go module installation coordinates

Go's module resolver does not preserve arbitrary build metadata as a canonical
module version. The official Go 1.27.3 implementation documents and implements
this in:

- [`cmd/go/internal/modload/query.go`](https://github.com/golang/go/blob/go1.27.3/src/cmd/go/internal/modload/query.go): a noncanonical tag, including `+metadata`, is looked up as a revision and may resolve to a suitable pseudo-version. A failed metadata-tag query can fall back to a canonical tag without that suffix.
- [`cmd/go/internal/modfetch/coderepo.go`](https://github.com/golang/go/blob/go1.27.3/src/cmd/go/internal/modfetch/coderepo.go): module version lists omit noncanonical tags; revision resolution selects a usable canonical tag at that commit or derives a pseudo-version.
- [Go module version reference](https://go.dev/ref/mod#versions): module versions, pseudo-versions and path-major constraints.

Therefore a Git application tag `v0.8.0+20261105.16` does not by itself guarantee
that `go get`, `go install` or `@latest` selects a canonical module coordinate
with that spelling. A query can record a pseudo-version, or resolve to a valid
canonical module tag when one is available at the selected commit. Recheck the
actual toolchain and installation path rather than assuming another Go version
has identical behavior.

An exact full-tag revision query is supported; retaining only the application tag
is a valid choice. For example:

```sh
go get example.com/team/app@v0.8.0+20261105.16
```

With only that full tag available, Go 1.27.3 can record a coordinate shaped like
`v0.8.1-0.TIMESTAMP-HASH`. That is Go's pseudo-version bookkeeping, not an app
release named `0.8.1`. The app and release still display `0.8.0+20261105.16`.
A direct-backend result can report `Origin.Ref` as
`refs/tags/v0.8.0+20261105.16` and `Origin.Hash` as the tag's exact source commit.
Honor a user's full-tag-only policy: do not add a bare `v0.8.0` tag or strip the
requested suffix to work around module canonicalization. A failed lookup still
needs its actual error investigated; do not silently select another revision.
Successful `go get`/plain `go install` resolution proves source selection, not
that the full app identity was embedded. Verify the app's release metadata/build
path and resulting output separately; do not relabel an unknown build as a release.

For a consumer that is also a module, record the full app identity, application
tag, final commit and actual resolved module coordinate separately. Verify that
the resolved coordinate's source commit is the intended one; when the direct Go
backend exposes it, inspect `go list -m -json`'s `Origin.Hash`, and otherwise use
an equivalent verified VCS/proxy mapping. A matching numeric core is insufficient.
Conventional module tags, including this library's existing `v0.1.1`, are technical
dependency coordinates. A new module coordinate requires its own reviewed release
policy and authorization; this guidance does not create a bare `vCORE` alias,
publish anything, or replace the full displayed app/release identity.

## Overall build-number continuity

Keep `CORE+YYYYMMDD.PR` separate from the overall build number. The shared allocator
manages the latter as a one-line numeric `VERSION` at the permanent canonical main
checkout root (I: in the maintained layout). Journals/lock/receipts remain external
on V:. Use [location-only migration](counter-location.md) for existing authorities;
it never allocates, reseeds or changes IDs/receipts/retries.
Before initialization, inspect existing VERSION/build files, history, scripts and
hooks and preserve the verified prior sequence/value with evidence. If none exists,
the explicitly approved fallback is the one-time total unique commits reachable
from the recorded canonical integration HEAD in complete, non-shallow history.
Never use unrelated `--all` history or recalculate it for each build. This seed
provides continuity; it does not count historical compilation attempts.

Use the explicit seed/migration API/CLI in [build counters](build-counters.md).
Migration accounts for all attempts present under lock and preserves old receipts,
allocation retries and artifacts. Inspect VERSION files holding full release
identities deliberately before adopting the numeric-file contract. A checkout's
release identity must remain complete and distinct from the authority's counter.

## Shared Codex guidance

The source-controlled [go-versioning skill](../skills/go-versioning/SKILL.md)
contains the compact consumer workflow. To install it, copy the complete
`skills/go-versioning` folder into your Codex skills directory, preserving
`agents/openai.yaml`. The default location is `$CODEX_HOME/skills` or
`~/.codex/skills` when `CODEX_HOME` is unset. Start a new chat or refresh skill
discovery when needed, then invoke `$go-versioning` or let the skill be selected
for a matching versioning task. Installing guidance does not modify other apps.
