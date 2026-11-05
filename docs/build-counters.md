# Persistent build-attempt counters

An overall build number is a reservation for one logical build invocation in one
explicitly initialized application authority. It is independent of the complete
`CORE+YYYYMMDD.PR` release identity. No PR number, commit count, dependency version
or inferred historical baseline is used. The first reservation is 1: the first
tracked attempt in this authority, not a claim about earlier application builds.

`Info.OverallBuildNumber` is uint64; zero means unknown.
`Info.BuildCounterID` identifies the authority/epoch. A positive number requires
that 32-digit authority ID, a full commit, an explicit version (including
`0.0.0+unknown`) and an RFC3339 build-start timestamp. Runtime readers validate
those fields but do not authenticate an allocation against a live store.

## Authority and storage

Use the public `buildcounter` package only in build tooling. Supply a stable
application ID and an absolute external store path. The parent must already exist.
`Initialize` exclusively creates a new store; `Open` never creates/reset state.
The CLI also requires an explicit repository and rejects source-local stores.
Store permissions and access are the operator's responsibility; only trusted,
cooperating builders may write it.

The store contains immutable authority metadata, an append-only `issued.log`,
immutable numbered JSON reservations, and a temporary exclusive directory lock.
All cooperating allocators use the same lock and validate the complete sequence
and ledger before allocation. Issuance is synced before the reservation file;
the complete reservation file is synced before success is acknowledged. This
separate sequence detects a missing reservation tail rather than reusing a number.
Canonical schema1 JSON rejects duplicate/unknown fields and ambiguous edits.
Records are bounded; the ledger is never silently truncated to a prefix.

Concurrency is supported across processes/checkouts using the **same live store**
on a filesystem with atomic directory creation, exclusive file creation and the
required sync/visibility semantics. Local Windows/Linux validation is documented
with releases; a particular network filesystem requires its own verification.
Independent stores, ephemeral runner disks and cloud-sync replicas do not share
one order. CI must mount the authority used by its other builders, or designate
one allocation job with access to it and pass the immutable result to build jobs.
A CI run number can label a unique request, but is never the build number itself.

Copying or consistently rolling back the entire store can create duplicate
sequences and cannot be detected without an independent authority/high-water
record. Never clone/reset/rewind the live authority. This is a trusted-filesystem
allocator, not a network allocation service, signed receipt or tamper-proof log.
File sync requests durability; ultimate power-loss behavior depends on the OS,
filesystem and hardware. Preserve the whole store and external build journals.

## Attempts, failures and retries

Allocate once before invoking the compiler. All targets/packages belonging to
that logical invocation can embed the same reservation. This counts reserved
attempts, not successfully built binaries. A failed compiler/package/upload still
consumes its reservation. Gaps among successful artifacts are expected.

The builder supplies a fresh bounded request ID for a new invocation, even when
source/version is unchanged. Retrying a lost allocation response uses the same
ID: matching version/commit returns the original number and timestamp. A changed
version/commit with the same ID fails. Recompiling is a new invocation and needs
a new ID; the allocator cannot infer the intent of a reused ID or fingerprint all
dirty source, flags and tools. A source commit identifies HEAD, not arbitrary
uncommitted inputs; use a fixed source tree for reproducibility.

BuildTime is the actual original reservation/build-start time in UTC, not commit
time or completion time. Any allocator/output error stops compilation. An error
can include uncertain reservation state; preserve it and retry the original
request after investigation. Never continue with a guessed number or decrement.

An interrupted writer can leave a lock, partial issuance or partial reservation.
Future allocations fail closed, including context/deadline expiration while
waiting for a lock. No timeout/PID-based lock stealing or automatic journal
repair occurs. Stop/verify all writers and preserve the full store before any
operator recovery. Remove an abandoned lock only after proving no writer owns it.
An incomplete ledger requires reconstruction from trustworthy reservation evidence
or a deliberate new authority with its discontinuity disclosed; never delete an
uncertain tail or restore an older active sequence merely to make allocation pass.

## CLI and shared metadata injection

Use an explicit external parent and source root. These are synthetic paths:

```sh
mkdir -p /srv/build-data/example-app

go-versioning counter-init --repo /src/example-app \
  --store /srv/build-data/example-app/counter --app example-app --json

# Persist this ID for retries of this allocation; choose a new ID per invocation.
request_id=ci-job-208-attempt-2
flags=$(go-versioning build --repo /src/example-app \
  --store /srv/build-data/example-app/counter --app example-app \
  --request-id "$request_id" --format ldflags) || exit

CGO_ENABLED=0 go -C /src/example-app build -trimpath -buildvcs=false -ldflags "$flags" \
  -o /srv/build-data/example-app/app ./cmd/app
```

`--json` returns the flat typed `Info` for other build-script integrations.
`--format ldflags` targets strings owned by go-versioning; applications can simply
call `versioning.CurrentBuild()`. They need no private allocator, linker variable
convention or numeric parser. `ParseBuildNumber` is available for an existing
custom injection adapter and rejects signs/leading zeroes/overflow.

```go
info, err := versioning.CurrentBuild() // read-only; no Git, store or allocation
if err != nil {
    return err
}
// Map info into your own presentation, API or go-banner BuildDetails.
```

An untagged repository remains `0.0.0+unknown` while receiving a genuine known
counter. Source versions otherwise remain snapshots. `build --release --version
FULL --branch main` requires a clean source tree, matching existing application
tag and HEAD, and existing first-parent/date/core rules. Supply a reviewed
`--bump major|minor|patch` or first `--initial-core` when needed. The caller still
verifies the hosting PR/merge association and remote tag identity. The command
never creates application tags or releases. Only its counter reservation changes.

`CurrentBuild` without injection preserves unknown counters and unknown actual
build time; it never substitutes VCS time for compilation. Existing `Current`
and custom `Info` injection remain supported with their established fallback
contract. `Info.LinkerFlags()` validates a complete identity and formats the same
shared flags used by the CLI. See [the metadata example](../examples/metadata/main.go).

`Info` JSON uses a uint64 number. Browser-facing APIs should encode the known
number as a decimal string before JavaScript parsing to preserve all 64 bits.
Presentation must preserve unknown zero and the counter ID as provenance. A banner
renders metadata; it does not allocate or validate a historical baseline.

## Go API for build tools

```go
store, err := buildcounter.Open(absoluteStore, "example-app")
if err != nil {
    return err // Initialize is a separate explicit one-time operation.
}
reservation, err := store.Reserve(ctx, buildcounter.Request{
    ID: attemptID, Version: sourceInfo.Version, Commit: sourceInfo.Commit,
})
if err != nil {
    return err
}
flags, err := reservation.Info.LinkerFlags()
```

Cancellation is checked while acquiring/validating state. Once issuance starts,
the small write/sync sequence completes or returns its actual I/O error; context
cancellation cannot interrupt an OS file sync. The default CLI deadline is 30s.
The ledger is fully validated on each reservation, so cost grows with history;
no rows are discarded for speed. Capacity/storage migrations need their own
reviewed preservation procedure.
