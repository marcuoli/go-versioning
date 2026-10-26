# Architecture

The module is designed for other Go applications to import.

## Public runtime API
The root package `github.com/marcuoli/go-versioning` owns version parsing,
calendar-date allocation, SemVer core changes and structured build metadata.
Runtime discovery reads only the main executable's embedded metadata. It never
executes Git or probes the caller's current directory.

Keeping this package at the module root gives callers a short import path.
A `pkg` directory would add another path component without adding an access
boundary. An `internal` directory would prevent external applications from
importing the package.

## Optional Git API
`github.com/marcuoli/go-versioning/gitversion` owns explicit repository inspection,
first-parent/tag selection, complete-history checks and immutable tag operations.
It imports root policy; root policy never imports Git execution.
Callers that need Git use `NewContext` and provide their own context and directory.

## Command and maintenance
`cmd/go-versioning` owns command parsing, help, JSON/human output and exit status.
It calls the two library packages and handles interrupt cancellation.
`examples/app` demonstrates an application consumer.
`scripts`, editor tasks and CI hold development automation; `docs` holds
operator and contributor guidance, with design records under `docs/superpowers`.

The root API and optional Git API are public. Existing helpers remain private
within their owning package. Introduce an `internal` package only when there
is a concrete shared private responsibility, rather than scaffolding empty layers.

Go 1.27.3 is the development and module baseline. Runtime packages use only the
standard library, and distribution builds retain `CGO_ENABLED=0`.
