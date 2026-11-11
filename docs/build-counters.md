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
`InitializeAt`/`InitializeSeedAt` explicitly create a new store bound to main VERSION;
`Open` never creates/resets state. Legacy initialization APIs remain compatible.
The CLI also requires an explicit repository and rejects source-local stores.
Store permissions and access are the operator's responsibility; only trusted,
cooperating builders may write it.

New CLI authorities use schema3: one numeric `VERSION` at the permanent canonical
main checkout root (I: in the maintained Windows layout), with adjacent
`VERSION.authority.json` ownership. Journals/lock/receipts remain external on V:.
All builders reach that one host-native main file, never checkout/cloud copies.
The value is validated against full history and atomically replaced under the same
lock. Read [location migration and host reachability](counter-location.md) before
adoption. `counter-relocate` moves an existing authority's file without allocating,
reseeding or changing IDs/baselines/receipts/retries; the former file is retired.

The store contains authority metadata, an append-only `issued.log`,
immutable numbered JSON reservations, and a temporary exclusive directory lock.
All cooperating allocators use the same lock and validate the complete sequence
and ledger before allocation. Issuance is synced before the reservation file;
the complete reservation file is synced before success is acknowledged. This
separate sequence detects a missing reservation tail rather than reusing a number.
Canonical JSON rejects duplicate/unknown fields and ambiguous edits. Schema1/2
authorities remain compatible until explicit relocation. Schema3 binds the original
journal and permanent main file; older binaries reject it. Upgrade all builders
before migration. Baseline migration preserves an existing schema3 binding.
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
compilation do not allocate this project's number. Disposable validation-only CI
executables and automatic hosting source archives do not allocate. Ordinary tests, startup,
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

Use `InitializeSeedAt(ctx, path, app, mainVersionFile, Seed)` for a new seeded authority, or
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
Baseline migration publishes a header binding the expected receipt first, preserving
schema3 location when present, so old
and new writers reject any incomplete transaction. A crash between files is
retained for deliberate recovery, never
automatically adopted or repaired. Preserve the entire authority and stop/verify all
writers before operator recovery. Never delete a receipt or reset a sequence.

An existing VERSION containing a full release identity is not a numeric counter.
Inspect and preserve it and its consumers deliberately before adoption. This CLI
rejects a source VERSION with conflicting content; it only manages the designated
main numeric file under the ownership contract. For a schema1 authority it admits an
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
go-versioning counter-init --repo /src/new-app --main-repo /src/new-app \
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

go-versioning counter-init --repo /src/example-app --main-repo /src/example-app \
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
    return err // InitializeSeedAt/InitializeAt is a separate explicit one-time operation.
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

A reviewed project-owned library compilation or explicit release-source-package
workflow may reserve once at its defined logical artifact-production boundary.
Automatic hosting archives remain excluded. Preserve all already-issued attempts,
receipts and baseline continuity; this classification never erases/reseeds history.

## Recovering a lagging primary VERSION

If a restored primary checkout has an older numeric `VERSION`, stop all builders
and non-cooperating writers (including cloud synchronization) and preserve the
complete authority and original file first. A filesystem lock cannot fence a
cloud client. Do not copy a conflict file over the authority, reseed, or truncate
history.

Use `counter-recover` with the independently verified counter ID, exact current
number and complete journal high-water mark. Preview is the default:

```text
go-versioning counter-recover --repo APP --main-repo MAIN --store JOURNAL --app example --counter-id ID --expected-current 7 --expected-last 9 --json
```

After inspecting the plan, repeat with `--apply`. The CLI verifies the designated
primary main and ignored/untracked operational files. The schema-3 API validates
ownership, relocation evidence, the baseline, all reservations and the independent
issued sequence under the normal authority lock. It accepts only a strictly
lagging, canonical decimal file. Incomplete/corrupt history, a different identity
or location, an ahead value or changed expectations remain blockers.

Before replacing `VERSION`, apply writes an immutable
`version-recovery-7-9.json` receipt containing the original bytes, identity,
location, expected numbers and recovery time. Preserve this receipt with the
journal. An exact retry can resume after receipt publication or report an already
completed repair; it fails if allocation has since advanced the expected ledger.
No number is allocated, and allocation receipts, baseline, counter identity and
issued sequence stay unchanged. Normal allocation and status still reject a
rollback until explicit recovery completes. Run `counter-check` afterwards.

The public `Store.RecoverVersion` API has the same preview/apply semantics. Its
caller must verify the permanent main root and stop external writers; the CLI
supplies the Git checks. Recovery trusts a complete existing authority, not an
independently reconstructed history or proof against wholesale rollback of both
journal and file.
