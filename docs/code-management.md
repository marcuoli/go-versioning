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

## Validation and builds
Use Go 1.27.3 and the shared scripts for tests, vet, module verification and
CGO-disabled CLI builds. Run race tests with an existing C compiler when the
affected behavior requires them. Keep compiler/cache settings process-scoped.
Editor build tasks remain local and offline; publication is a deliberate
separate operation.

## Release identities
This repository is a Go dependency: use conventional immutable module tags such
as `v0.1.0`, following module path-major rules for later major versions.
The library also implements the separate application identity
`MAJOR.MINOR.PATCH+YYYYMMDD.PR`. Do not confuse these two contracts or move a
published tag to repair a release.

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
