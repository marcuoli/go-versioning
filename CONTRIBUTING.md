# Contributing

Use Go 1.27.3 and an installed Git. The library uses only the standard library.
The root package is the normal application import; `gitversion` is optional
repository integration, and `cmd/go-versioning` is the separate command tool.

Read [architecture](docs/architecture.md) and [code management](docs/code-management.md).
Keep changes focused, add behavior-based regression tests, and run the validation
scripts before opening a pull request. Public examples must contain synthetic
data and generic endpoints.

The Gitea source repository accepts implementation changes. The public GitHub
repository is copied from that source. Keep source changes in one history;
see [copying](docs/mirroring.md) for synchronization and conflict behavior.

Each merged pull request includes its own `CHANGELOG.md` entry. Module release
tags are immutable ordinary Go module versions. Release publication is deliberate
and separate from default local builds.
