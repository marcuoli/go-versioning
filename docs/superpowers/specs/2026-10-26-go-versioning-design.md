# Application versioning design

Build a reusable, standard-library-only Go module and CLI for immutable application versions.

Module: `github.com/marcuoli/go-versioning`. Root package: `versioning`.
Minimum Go: 1.27.3. Library dependency releases use ordinary tags such as `v0.1.0`.

## Selected approach
Share version/build metadata plus Git tag-allocation tooling. A metadata-only
library would leave each application maintaining its own allocation policy.

## Policy
- Releases: `MAJOR.MINOR.PATCH+YYYYMMDD.PR`; canonical Git tags prefix that with `v`.
- Source builds: `MAJOR.MINOR.PATCH-snapshot+YYYYMMDD.PR`; unknown: `0.0.0+unknown`.
- Valid calendar dates, unsigned core components without leading zeroes, positive PR numbers.
- Inherit the core by default; major/minor/patch bumps reset lower components.
- Allocation uses the final commit's committer date in America/Sao_Paulo.
- Canonical tags on first-parent history take priority; legacy `build/` tags are read-only fallback.
- Build metadata identifies application builds and does not change SemVer precedence.

## Ownership
Root owns parsing, formatting, core calculation, and injected/embedded metadata
resolution. `gitversion` owns explicit-repository Git inspection, planning and
immutable allocation. `cmd/go-versioning` owns flags and output.
The runtime library never probes the working directory or a dependency source path.

Complete valid injected metadata wins. Partial injection may fill missing
fields from the main binary's embedded metadata only when commit identities
are compatible. Requested failed Git operations and explicit malformed inputs
return errors. No fabricated timestamp or repository identity.

## CLI and safety
`inspect` reports checkout metadata; `next` calculates a local read-only plan;
`tag` allocates locally and pushes only when explicitly enabled with a named remote.
Allocation requires a full target commit, PR, target branch and complete
first-parent history. First allocation requires an explicit initial core.
Reruns require the exact expected tag/commit. Never replace existing tags.
A push verifies remote application tags and predecessor identity first;
failure preserves any created local tag and reports partial state.
Use finite deadlines, argv-based Git calls, credential-free diagnostics and
human or stable JSON output. Credentials stay in Git's existing facilities.
Examples support Gitea and GitHub event metadata.

## Publication
The source repository is hosted on Gitea and copied one-way to a public GitHub
repository. All tracked content and Git history must be public-safe. Documentation
uses synthetic examples and generic forge endpoints. Use repository-limited
mirror credentials. Application adoption and packaging remain separate work.

## Acceptance
- Parsing, real dates, overflow, bumps, bootstrap and metadata precedence tests.
- Temporary Git repositories and local bare remotes prove ancestry, canonical
  priority, shallow rejection, conflicts, idempotence and push-failure recovery.
- CLI tests prove flags, JSON, exit codes, dry runs and cancellation.
- Full tests, vet, race where available, minimum-Go checks and CGO-disabled builds.
- Standalone consumer example, cross-platform CI, README and public-content audit.
