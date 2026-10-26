# Repository guidance

This is an importable Go library with optional Git integration and a CLI.

## Package boundaries
- Keep the public application-version and build-metadata API at the module root.
- Keep explicit repository operations in the public `gitversion` subpackage.
- Keep commands, flags and CLI presentation under `cmd/go-versioning`.
- Add `internal` packages only when actual private implementation needs them.
- Preserve public API compatibility and the separation between runtime metadata and Git discovery.

## Development
- Use Go 1.27.3 and the standard library. Review any new dependency deliberately.
- Finish one bounded issue before starting another; preserve existing behavior and unrelated work.
- Before editing tracked files, inspect both working changes and committed content.
- Use the shared validation scripts. Run meaningful regression tests, the full test suite,
  vet, module verification, applicable race tests and CGO-disabled builds.
- Keep caches and generated binaries outside tracked source; `bin/` is ignored.
- The default editor build is local and offline. Publishing is a separate explicit operation.

## Change management
- For ongoing work, use typed imperative issue titles: feat, fix, docs, refactor,
  test, build, security or chore, with one matching `type:*` label and accurate `area:*` labels.
- Include `## Validation and acceptance` and concrete Markdown checkboxes in each issue.
  Record evidence before checking an outcome or closing an issue.
- Use `codex/<type>/<issue>-<description>` for agent branches (`feature` for feat).
- Use focused feature branches and reviewed pull requests. Every merged PR updates
  `CHANGELOG.md` on its own branch; preserve existing entries.
- Distinguish implemented, committed, merged and published outcomes in handoffs.
- Report the verified full checkout path and branch when handing work back.

## Publication and public safety
- The configured Gitea origin is the source of truth; GitHub is a public one-way copy.
- Audit tracked content and Git history before publication. Use synthetic examples and
  public commit metadata. Keep internal endpoints, private paths, credentials, unrelated
  application source, inventories and generated logs out of commits.
- Keep credentials in existing helpers or repository-scoped secrets, outside arguments/logs.
- Run `govulncheck ./...` with a maintained scanner before a release.
- Module releases use immutable conventional tags such as `v0.1.0`; do not move or reuse them.
  Date/PR application tags are this library's output contract, not its module-release scheme.
- Verify identical source/copy branch and tag identities and successful automation before
  reporting copying or publication complete.
