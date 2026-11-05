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

Use the full allocated application version in release names, display text,
versioned artifact names and injected metadata; use its `v`-prefixed tag for the
exact final merged commit. Keep the date/PR suffix when the app is also imported
as a Go module. Go's resolved module version is a technical installation
coordinate and may be a pseudo-version; it is not an alternate app identity.
See the [application version policy](application-version-policy.md) for Go 1.27.3
resolution behavior, explicit coordinate-to-commit mapping and verification.

## Build-attempt metadata

Tag allocation and build counters are independent. Use the explicit shared
[build counter](build-counters.md) only when builders can reach one persistent
authority; a runner-local counter or Actions run number does not establish a
global sequence. Reserve once for a logical multi-target build and distribute its
validated metadata to jobs; failed builds retain the reservation. Runtime startup
only reads injected metadata. Preserve the full date/PR release identity.
