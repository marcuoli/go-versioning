# Code management

## Bounded delivery
Finish one bounded issue at a time. For ongoing development, maintain an issue
with a typed imperative title, one matching `type:*` label and accurate area
labels. Use the exact `## Validation and acceptance` heading with concrete
checkboxes. Keep incomplete or untested outcomes unchecked and record actual
evidence before closure.

Use focused branches and reviewed pull requests. Include implementation,
regression tests, documentation and a newest-first `CHANGELOG.md` entry on
the pull-request branch. Preserve unrelated code, comments, tests and local
changes. Never infer publication from passing local tests.

## Classification

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `build`, `security`, `chore`.
Areas: `area:versioning`, `area:git`, `area:cli`, `area:tooling`,
`area:documentation`, `area:release`, `area:security`.

## Validation and builds
Use Go 1.27.3 and the shared scripts for tests, vet, module verification and
CGO-disabled CLI builds. Run race tests with an existing C compiler when the
affected behavior requires them. Keep compiler/cache settings process-scoped.
Editor build tasks remain local and offline; publication is a deliberate
separate operation.

## Release identities
Apply the [application version policy](application-version-policy.md) to consumer
apps: the full identity is `MAJOR.MINOR.PATCH+YYYYMMDD.PR`, including display,
release names, versioned artifacts and immutable `v`-prefixed application tags.
Use the final merged commit's committer date in `America/Sao_Paulo` and the actual
merged PR number. A numeric core or resolved dependency version is insufficient
for identifying an application release.

This repository is also a Go dependency. Conventional immutable module tags such
as `v0.1.1` remain technical installation coordinates, following Go's module
path-major and canonicalization rules. Verify their exact source commit; never
substitute them for a consumer's displayed app identity. Preserve published tags
and releases. The shared [Codex skill](../skills/go-versioning/SKILL.md) makes this
policy discoverable without changing consumer repositories automatically.

## Source and public copy
Use the configured Gitea origin as the source of truth. The public GitHub copy
receives explicit branch/tag pushes through the dedicated repository deploy key.
Do not introduce divergent implementation history on the copy.
The copy deliberately rejects force updates and does not delete destination refs.

Before publication, audit tracked content and all new commits for private
information. Use public noreply commit metadata, generic endpoints and synthetic
examples. Never commit credentials, inventories, local paths, private service
configuration or unrelated application source.

Verify source/copy commits, tag identities, workflow results and fresh module
consumption before calling a release published. Final handoffs identify the
full checkout path, branch and whether work is committed, merged or published.
