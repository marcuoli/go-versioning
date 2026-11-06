# VERSION at permanent main

The authoritative numeric `VERSION` belongs in the application's permanent canonical
main checkout root. On the maintained Windows layout that is on I:. Journal metadata,
the exclusive lock, baseline, issuance log and immutable reservation receipts remain
in the external project store on V:. A temporary implementation/release worktree or
CI source checkout never receives a live copy. Full `CORE+YYYYMMDD.PR` application
identity remains separate.

Schema3 records the authority-host-native main file and original journal path.
`VERSION.authority.json` beside VERSION binds that file to its application, authority
ID, journal path and location-time number. It is operational ownership metadata, not
a new counter. Another store cannot claim it. A copied completed or pending journal
fails its original-path/ownership check. Do not hand-edit either file or its header.
This is still a trusted-filesystem allocator: consistent whole-authority rollback or
replication cannot be made safe by these checks alone.

## Existing authority: location only

Upgrade and stop/verify all old builders, preserve the authority and inspect any
existing root VERSION. Run on the ONE designated allocation host:

```sh
go-versioning counter-status --repo /src/example-main \
  --store /srv/build-data/example/counter --app example --json

go-versioning counter-check --repo /src/example-main \
  --main-repo /src/example-main --store /srv/build-data/example/counter --app example --json
# A legacy-layout diagnostic means relocation is required, not manual file creation.

go-versioning counter-relocate --repo /src/example-main \
  --store /srv/build-data/example/counter --app example \
  --main-repo /src/example-main --json
```

Then repeat `counter-check` with the same arguments; it must succeed before production
allocation/publication. A missing canonical-root VERSION is expected before relocation
of an existing authority. The command creates/binds it at the preserved current value
(e.g. 27), without a placeholder, seed or reservation.

This allocates nothing and changes no number, authority ID, baseline, issued row,
receipt bytes, request ID or retry timestamp. It counts/validates all existing history
under the original lock, publishes pending schema3 intent, creates matching main
ownership/VERSION, preserves the old store file as `VERSION.retired`, removes the
former live store VERSION and completes the header. The retired evidence remains
required and matches the number at relocation, even after later builds increment.

On Windows, directory handoff can transiently report access denied while a removed
lock is delete-pending. Acquisition retries that exact result at most three times;
persistent failures are still errors, no ACL is changed and no lock is stolen.

The same target can be retried: known completed steps are checked, never overwritten.
Normal status/allocation is blocked during a pending transition. Conflicting, partial,
missing or corrupt evidence fails closed for operator investigation; this command is
not generic repair. A surviving abandoned lock still requires proving no writer owns
it before operator recovery. Another target or changed journal host/path is rejected.
Old CLI/library releases reject schema3 instead of continuing the V: sequence.

Existing root VERSION must be absent or contain the exact current decimal number
with LF. A full release identity, mismatching number, symlink or another owner is
preserved and rejected. Resolve that conflict through the owning project's reviewed
migration, preserving its old identity/evidence; do not reset/reseed an authority or
blindly overwrite a tracked release file. `counter-migrate` is the separate one-time
baseline operation and is not needed for a location correction. Do not pass a seed
or invoke `build` merely to relocate.

## New authority and ongoing builds

`counter-init` and `counter-relocate` require explicit `--main-repo` before any
write. `counter-init` puts VERSION in the primary permanent main checkout, using the
existing explicit legacy seed or investigated one-time canonical commit fallback.
`--main-branch` defaults to `main`. Primary main must be checked out on that branch;
linked worktrees/bare repositories are not designated main roots. `--repo` can be a
linked source worktree; Git's primary main remains the location. `--main-repo` is always explicit for initialization/relocation and also designates
real main when builds use a separate source/CI checkout on that same host.
The operator must verify permanence, actual mount identity and source project: Git
branch designation alone does not authenticate them.

```go
store, err := buildcounter.InitializeSeedAt(ctx, journalPath, appID,
    filepath.Join(permanentMainRoot, "VERSION"), verifiedSeed)
// For an existing store, use store.RelocateVersion(ctx, mainVersionFile).
// Open, Reserve, Status and exact allocation retries keep their established APIs.
```

`InitializeAt` is the deliberately unseeded counterpart. These filesystem APIs
require an explicit absolute main VERSION and external journal directory; the caller
verifies its main designation. `BoundVersionFile` is a readonly location query.
Legacy `Initialize`/`InitializeSeed` and schema1/2 are retained for API compatibility,
not the prescribed layout for new adoption. Explicit relocation upgrades them.

VERSION and its ownership record are mutable operational files. The owning project
must review ignore rules such as `/VERSION` and `/VERSION.authority.json`, keeping
release-identity files separate. The shared `Repository.CheckCounterFiles` rejects tracked files and requires both
files to be ignored. The CLI applies it before initialization/relocation, `build`
and `counter-check`; it never silently edits `.gitignore`, untracks a legacy file or
weakens release cleanliness. `CheckLocation` validates the expected path, ownership
and full history without allocation. `counter-check` combines these checks with Git
primary-main designation and is the common adoption/production gate. `counter-status`
is readonly preflight history/location information, including the bound-file snapshot;
its filesystem compliance flag alone does not authenticate Git policy or a mount. Atomic updates stage a synced file beside main VERSION and
rename it under the original journal lock; this necessary destination-local staging
is separate from compiler/cache/log scratch, which remains on V:.

## Windows, WSL, SSH and CI reachability

Persisted paths belong to the designated allocation host. No Windows/Linux alias
conversion, host migration, writable cloud copy or duplicate checkout counter is
inferred. WSL may invoke the Windows CLI by interop using Windows source/journal/main
paths and pass returned metadata to its Linux compiler. CI can call one allocation
job on that authority host and fan out the immutable result to build targets.

A Linux journal host reached over SSH can stay the authority ONLY if it has coherent
read/write access to the actual permanent Windows main root, for example one trusted
filesystem mount of that exact root. SSH itself does not share a file. Run relocation
on Linux with its native mounted main path; Windows callers continue to invoke that
same Linux allocator. For a separate source checkout use, for example:

```sh
go-versioning build --repo /srv/source-at-commit \
  --main-repo /mnt/real-canonical-main --store /var/lib/example/counter \
  --app example --request-id "$attempt_id" --format ldflags
```

Verify that mount names the real canonical root and that source/version/commit belong
to the intended project. A runner clone or synchronized checkout is not a substitute.
If the host cannot reach the real main file, location cutover is blocked pending
qualified filesystem access. Moving journals/authority hosts is a separate preservation
migration, not supported by this location-only command. Do not copy/edit bindings.
Filesystem visibility/atomic replacement and power-loss behavior need qualification
on the actual mount; local tests do not certify arbitrary network filesystems.

Readonly libraries, imports, planning/startup/tagging, tests, disposable CI showcase
executables and automatic source archives do not allocate. Only a deliberately defined
artifact-production entry point for output intended for use/distribution reserves once
per logical build/matrix. A source-only module may have no authority or VERSION until
that boundary exists; do not create unused state to satisfy a layout example.
## Fixture safety and repeatable adoption proof

Disposable counter tests MUST create a standalone isolated Git primary-main fixture.
A linked worktree of the real application still resolves the live primary main and
is not sufficient isolation. Never pair a temporary journal with real application
--repo/--main-repo in an allocation/migration test. The explicit-main write guard
rejects missing designation before creating any root file/store, but cannot infer
that an explicitly named real root was intended as a test. Root ownership blocks a
second authority after real adoption. Readonly metadata tests may use real source.

Consumers should record official module/source identity, designated primary main and
allocator host/journal, reviewed tracking/ignore policy, and before/after status.
Use shared counter-check -> counter-relocate -> counter-check for legacy location
adoption (first check's legacy diagnostic is expected); retain the same number, ID,
baseline and receipt hashes. All builders then call the shared build/Reserve APIs;
new CLI build rejects legacy V: placement. Preserve existing root release-file
semantics in an owning-project PR before cutover; a conflict is actionable, never
permission to create a replacement counter. Source-only/no-authority is explicitly
N/A for location checks and must not fabricate VERSION merely to pass a checklist.
