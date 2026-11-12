# go-versioning

Reusable application version metadata and immutable Git tag allocation, using only the Go standard library.

```sh
go get github.com/marcuoli/go-versioning@v0.3.1+20261106.13
go install github.com/marcuoli/go-versioning/cmd/go-versioning@v0.3.1+20261106.13
```

Requires Go 1.27.3 or later. Repository operations also require Git.


## Public package boundaries

| Import | Responsibility |
| --- | --- |
| `github.com/marcuoli/go-versioning` | Application versions and runtime build metadata |
| `github.com/marcuoli/go-versioning/gitversion` | Explicit Git inspection and tag allocation |
| `github.com/marcuoli/go-versioning/buildcounter` | Explicit external build-attempt authority and reservations |

The CLI is separate under `cmd/go-versioning`. See [architecture](docs/architecture.md)
for ownership and [contributing](CONTRIBUTING.md) for the development workflow.

## Application versions

| Identity | Example |
| --- | --- |
| Release | `1.2.3+20260102.42` |
| Canonical application tag | `v1.2.3+20260102.42` |
| Source build | `1.2.3-snapshot+20260102.42` |
| Unknown | `0.0.0+unknown` |

The core is inherited until an explicit major, minor or patch bump. The date is
the target commit's committer date in `America/Sao_Paulo`; the positive suffix
is the merged pull-request number. Build metadata does not affect SemVer
precedence. Order builds by Git ancestry and pull-request identity.

Use the complete identity everywhere an application version is shown: CLI output,
UI headers/footers, About pages, API build metadata, logs, release names and
versioned artifact names. For example, `0.8.0+20261105.16` remains intact;
`0.8.0` is only its numeric core. The immutable application tag is
`v0.8.0+20261105.16` on the actual final merge commit.

Read the [application version policy](docs/application-version-policy.md) for
release verification and package-manager constraints. This library's conventional
Go module tags, such as `v0.1.1`, are dependency installation coordinates. They do
not replace an application's full displayed identity. Go 1.27.3 can resolve a
`+metadata` tag to a pseudo-version, so never infer the app version from a
resolved module version or claim that a full app tag is a canonical Go module
version without checking it.

For Codex, use the shared [`go-versioning` skill](skills/go-versioning/SKILL.md)
when integrating or releasing a consumer. Copy that folder into your Codex
`skills` directory to enable discovery, or invoke the installed `$go-versioning`
skill explicitly. Consumer source changes remain scoped to the requested app.

## Embed application build metadata

```go
package main

import (
    "fmt"
    "log"

    versioning "github.com/marcuoli/go-versioning"
)

var version, commit, buildTime string

func main() {
    info, err := versioning.Current(versioning.Info{
        Version: version, Commit: commit, BuildTime: buildTime,
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(info.Version)
}
```

A release build supplies the full application version, full commit SHA and an
RFC3339 timestamp using `-ldflags "-X main.version=... -X main.commit=... -X main.buildTime=..."`.
Complete injected metadata wins. Partial metadata can use the main executable's
embedded VCS information when the commit identities agree. Missing versions
remain explicitly unknown. No timestamp is fabricated.

The runtime package never executes Git or discovers a repository from the
current directory. Source tools can opt in with `gitversion.NewContext(ctx, repositoryDirectory)`
and `Repository.Inspect(ctx)`; that explicit repository produces a snapshot.

## Shared build counters

Build tools can explicitly initialize an external `buildcounter` authority and
reserve a real increasing number per logical build attempt. Failed builds consume
numbers; allocation-request retries are idempotent. All cooperating checkouts/CI
must share that live authority. No historical count or release version is invented.

The authority binds a one-line numeric `VERSION` at permanent canonical main,
with journals/receipts external. See [location migration](docs/counter-location.md).
This is separate from the application
release identity. Investigate and preserve an existing sequence before migration;
only when none exists, seed once from complete canonical HEAD reachable commits.
Explicit `counter-migrate` preserves earlier reservations and allocation retries.

The CLI `counter-init`, `counter-relocate`, `counter-migrate`, `counter-status` and `build` commands produce validated metadata or shared
linker flags. Applications call `versioning.CurrentBuild()`; startup never
allocates or accesses a store. Optional `Info.OverallBuildNumber` and
`Info.BuildCounterID` preserve zero/empty unknown defaults. A known number is
independent of whether the app's release version is unknown.

Read [build counters](docs/build-counters.md) for exact persistence, failure,
retry, scope, recovery and build-script contracts before enabling allocation.

## CLI

```sh
go-versioning inspect --repo . --json
go-versioning next --repo . --branch main --commit FULL_COMMIT_SHA --pr 42 --json
go-versioning tag --repo . --branch main --commit FULL_COMMIT_SHA --pr 42
go-versioning tag --repo . --branch main --commit FULL_COMMIT_SHA --pr 42 --push --remote origin
```

`next` is a read-only local plan. `tag` creates a local lightweight tag;
`--push --remote NAME` explicitly enables publication to an existing named
Git remote. The tool never receives a token or credential-bearing URL.
Use `--bump major|minor|patch` to advance the core. For a repository with no
application version history, supply `--initial-core 1.0.0` without a bump.
Use each subcommand's `--help` for flags.

Allocation requires complete history without shallow boundaries or active grafts and an explicit target
branch, full commit SHA and positive pull-request number. Canonical tags on
first-parent history take precedence; legacy `build/` tags are read-only
fallback. Multiple/conflicting application tags fail. The tool never moves
or deletes an existing tag. Successful reruns require the exact same tag
and commit.

Push mode requires one identical fetch/push destination for the named remote.
It verifies the remote application tags and selected predecessor before
creating a tag. Missing or stale local tags require an ordinary fetch outside
the tool followed by a new plan. A failed push preserves the local tag and
reports its state, allowing an explicit retry.

JSON success output goes to stdout. Errors go to stderr; `--json` makes them
JSON objects with an `error` field and, where applicable, partial operation
state. Exit codes: 0 success, 1 operation failure, 2 invalid usage.
Git subprocesses have finite deadlines and honor cancellation.

## Integration and development

See [the consumer example](examples/app/main.go), [workflow examples](docs/workflows.md)
and [Gitea-to-GitHub copying](docs/mirroring.md). Application presentation,
localization, packaging and release notes remain with the consuming application.

```sh
bash scripts/validate.sh
# Select an exact minimum compiler on PATH, then test consumer compatibility.
bash scripts/validate.sh --go-version=1.27.3
bash scripts/validate.sh --race
```

On Windows use `./scripts/validate.ps1` and optionally `-Race` with an existing
C compiler. Current builds use Go 1.27.4; consumer compatibility remains Go 1.27.3.
Select that exact compiler on PATH and pass `-GoVersion 1.27.3` for the minimum
check. Scripts set `GOTOOLCHAIN=local` so automatic switching cannot hide which
compiler ran. Both exact versions are tested in Gitea and the GitHub OS matrix.

Build output defaults to the project's external V: build namespace on maintained
Windows/WSL hosts, or the hosted runner's temporary build directory. Set
`GO_VERSIONING_BUILD_DIR` (or PowerShell `-OutputDirectory`) for another external
destination. Scope Go caches and temporary directories to the owning project's
external storage before running tools. Historical Go 1.27.3 installation evidence
remains unchanged.
The default VS Code build task creates a local snapshot and does not publish.

Licensed under [UPL 1.0](LICENSE).
