# Application workflow examples

These examples belong in a consuming application. The go-versioning module
itself uses conventional Go module release tags.

[GitHub](workflow-examples/github-application-version.yml) and
[Gitea](workflow-examples/gitea-application-version.yml) examples allocate one
immutable application tag after a merged pull request to main. They install
the released CLI at an exact version and pass the full commit and PR identity.
They use complete history and fetch application tags before planning.

A missing version label inherits the previous core. Exactly one
`version:major`, `version:minor` or `version:patch` label selects a bump.
Multiple version labels fail. A new repository needs an explicit reviewed
`--initial-core` for its first allocation; these examples intentionally fail
without existing application history.

Enable repository content-write permission for the automation token. Configure
serialized allocation. The GitHub example uses `queue: max` to retain up to
100 pending runs instead of replacing the pending event. Queue overflow still
requires recovery, and dispatch order is not guaranteed. Both examples require
the immediate first parent to have exactly one application tag and fail closed
across a missing allocation, preventing a skipped bump from changing later cores.
Recover missed or failed allocations oldest first using their original merged
PR number, reviewed labels and full commit, then rerun the failed event. Bootstrap
the first application version explicitly before enabling either example.

Gitea 28.2 does not support `queue: max`; newer runs can replace blocked
pending runs even with `cancel-in-progress: false`. Check canceled allocation
events and recover them individually in merge order. The parent-tag admission
check prevents later events from silently skipping their version changes.
The CLI checks local ancestry and remote tag identity, but does not query a
hosting API to prove that a commit is a merged pull request. The workflow
supplies that authorization from its trusted merged event.

Tag allocation does not publish packages, create a release page or modify
application source. A separate release process should verify a clean checkout,
exact canonical tag and matching remote commit before injecting metadata.

The date/PR version is suitable for applications. Go dependencies should use
ordinary module versions, as this module does.
