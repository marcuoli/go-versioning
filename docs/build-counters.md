# Persistent build-attempt counters

An overall build number is a reservation for one logical build invocation in one
explicitly initialized application authority. It is independent of the complete
`CORE+YYYYMMDD.PR` release identity. PR numbers and dependency versions are never overall build numbers. Before adoption,
investigate Git history, legacy VERSION/build files, hooks and build/release scripts.
Preserve a verified existing sequence/value and record its computation and semantics
(including counters previously incremented on commits). Only when no existing method
or value exists, use the explicitly approved one-time count of all unique commits
reachable from the canonical integration HEAD in complete, non-shallow history.
Record the exact HEAD, count, command, method and check time. This is a continuity
baseline, not evidence of historical build attempts. Never use `--all`, a first-parent
count or recompute commit counts during normal builds. A deliberately new sequence
may still initialize at zero, with first reservation 1, for API compatibility.

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

New authorities use schema2 and `VERSION`: one canonical decimal line followed by
LF, representing the current overall number. This file lives inside the single
external authority, never in independent checkout/cloud-sync replicas. Its view is
validated against the complete history and replaced under the same lock using a
synced adjacent staging file and rename. Supported filesystems must provide atomic
replacement/visibility; verify the actual filesystem. Direct edits are forbidden.

The store contains authority metadata, an append-only `issued.log`,
immutable numbered JSON reservations, and a temporary exclusive directory lock.
All cooperating allocators use the same lock and validate the complete sequence
and ledger before allocation. Issuance is synced before the reservation file;
the complete reservation file is synced before success is acknowledged. This
separate sequence detects a missing reservation tail rather than reusing a number.
Canonical JSON rejects duplicate/unknown fields and ambiguous edits. Schema1
authorities remain readable/writable without automatic migration. Explicit migration
upgrades their header to schema2; older library/CLI versions reject schema2 rather
than ignoring VERSION or a baseline. Upgrade all builders before migration.
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

Allocate once before invoking the project-owned artifact build: compilation or an
explicit release package/archive invocation. Imports and downstream dependency
compilation do not allocate this project's number. Ordinary tests, startup,
planning/version queries and tagging alone remain read-only.

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

## Explicit seed and migration

Use `InitializeSeed(ctx, path, app, Seed)` for a new seeded authority, or
`store.Migrate(ctx, Seed)` for an existing authority. A Seed contains `number`
(uint64), `method` (`legacy` or `git-commits`), full `commit`, `command`, investigation
`evidence` and RFC3339 `checked_time`. Evidence must establish the previous method
and value, or its absence before choosing the Git fallback. API callers verify
that evidence; the allocator cannot determine whether their investigation is true.
`Repository.CommitCount(ctx, branch)` collects the Git fallback evidence, requires
HEAD to match the designated canonical local integration branch and rejects shallow,
grafted or untraversable history. The operator verifies that branch against its
actual integration source; this command does not fetch or authenticate a host.

Migration records an immutable baseline receipt, its SHA256 in the authority header,
and the exact number of already tracked attempts observed under the authority lock.
Current VERSION becomes `baseline + all tracked attempts`, including builds that
arrived after preparation. For a synthetic baseline 70 and two existing attempts,
VERSION becomes 72; the next new reservation is 73. Earlier receipt bytes, numbers,
timestamps, request IDs and built artifacts remain unchanged. Retrying an old request
returns its original metadata, even though that historical number differs from the
new high-water. Do not recompile under an old allocation request ID.

Only one baseline is accepted. Retrying the same seed returns current status without
adding it again, including after later builds. Rechecking identical evidence with a
later `checked_time` retains the original receipt/time; every other seed field must
match. Status includes the original baseline evidence and tracked-attempt count.
Overflow, missing VERSION/receipt, a changed receipt, conflicting evidence or an
interrupted publication fails closed. Seeded initialization publishes its header only after the receipt and VERSION.
Migration publishes a schema2 header binding the expected receipt first, so old
and new writers reject any incomplete transaction. A crash between files is
retained for deliberate recovery, never
automatically adopted or repaired. Preserve the entire authority and stop/verify all
writers before operator recovery. Never delete a receipt or reset a sequence.

An existing VERSION containing a full release identity is not a numeric counter.
Inspect and preserve it and its consumers deliberately before adoption. This CLI
never overwrites a source checkout's VERSION. For a schema1 authority it admits an
absent VERSION or the exact previous numeric high-water only; a conflicting number,
release string or symlink fails without overwriting that file. A separate preserved
release-identity file may be needed in the owning project's migration workflow.

For a verified legacy baseline, create a reviewed seed JSON (synthetic values):

```json
{"number":70,"method":"legacy","commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","command":"inspect legacy VERSION history and increment hook","evidence":"verified value 70; hook incremented on commits","checked_time":"2026-11-05T20:00:00Z"}
```

```sh
# Existing authority: accounts for every reservation under lock.
go-versioning counter-migrate --repo /src/example-app \
  --store /srv/build-data/example-app/counter --app example-app \
  --seed-file /srv/build-data/example-app/legacy-seed.json --json

# New authority: only after investigation found no existing method/value.
go-versioning counter-init --repo /src/new-app \
  --store /srv/build-data/new-app/counter --app new-app \
  --seed-commits --branch main \
  --evidence "history, VERSION/build files, scripts and hooks inspected; no prior sequence" --json

go-versioning counter-status --repo /src/example-app \
  --store /srv/build-data/example-app/counter --app example-app --json
```

`--seed-file` is the explicit legacy-evidence path. `--seed-commits` collects and
persists the canonical HEAD/count/command/check time itself, requires investigation
`--evidence`, and is also supported by `counter-migrate`. The two options are
mutually exclusive. Save the returned status/receipt before preparing a retry.
If HEAD or other seed evidence has changed, stop and inspect the existing baseline;
never seed again from a newer count. All subsequent builds use only `build`/`Reserve`.

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
