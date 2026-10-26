# Go versioning implementation plan

> **For agentic workers:** Use test-driven implementation and independent final review.

**Goal:** Deliver a reusable public Go module, CLI and immutable tag-allocation workflow.
**Architecture:** Root policy/metadata package, explicit Git adapter, thin standard-library CLI.
**Tech Stack:** Go 1.27.3+, standard library and installed Git.
**Spec:** [Design](../specs/2026-10-26-go-versioning-design.md)

## Global constraints
- Module `github.com/marcuoli/go-versioning`; package `versioning`; Go 1.27.3; no dependencies.
- Application tags `vMAJOR.MINOR.PATCH+YYYYMMDD.PR`; snapshots insert `-snapshot`.
- America/Sao_Paulo committer-date policy; immutable tags; explicit repository and push.
- New clean public-safe history. Generic documentation and synthetic fixtures only.
- Keep credentials, internal hosts, local paths and private source out of tracked files.

## Review focus
- Missing/shallow history must never silently bootstrap the wrong version.
- An unrelated current directory or Git environment must not select another repository.
- Existing conflicting tags and failed pushes must retain accurate local/remote state.
- Partial metadata must not combine different commit identities.
- Git errors and publication history must not expose credentials or internal context.

## Task 1: Version policy and build metadata
Files: `version.go`, `metadata.go`, matching tests and `doc.go`.
Produces: Core{Major,Minor,Patch uint64}; Bump with BumpInherit/BumpMajor/BumpMinor/BumpPatch;
ParseCore(string)(Core,error); Core.String(); Core.Bump(Bump)(Core,error).
Version{Core Core, Date time.Time, PR uint64, Snapshot bool}; Parse(string)(Version,error);
ParseTag(string)(Version,bool,error), bool means legacy; Version.String();
Version.Tag()(string,error); Next(Core,time.Time,uint64,Bump)(Version,error).
Info{Version,Commit,BuildTime string} with snake_case JSON fields;
Resolve(Info,Info)(Info,error); ReadBuildInfo() Info; Current(Info)(Info,error);
UnknownVersion constant.
- [ ] Write behavioral tests first and observe missing API failure.
- [ ] Implement parsing, overflow checks, real dates, timezone, snapshot/tag behavior.
- [ ] Implement metadata precedence and embedded main-module discovery.
- [ ] Run `go test .`, `go vet .`, record results.

## Task 2: Git inspection and allocation
Files: `gitversion/repository.go`, `gitversion/plan.go`, `gitversion/tag.go`, tests.
Consumes Task 1 policy. Produces New(string)(*Repository,error);
Repository.Inspect(context.Context)(versioning.Info,error);
Repository.Next(context.Context,PlanOptions)(Plan,error);
Repository.Tag(context.Context,PlanOptions,string)(TagResult,error), empty remote = local only.
PlanOptions{Commit,Branch,InitialCore string; PR uint64; Bump versioning.Bump}.
Plan{Version,Tag,Commit,PreviousTag,Branch,Verification string}, JSON snake_case.
TagResult{Plan Plan; Local,Remote,Created bool}, JSON snake_case.
- [ ] Write temporary-repository tests and observe missing API failure.
- [ ] Implement explicit repository, finite deadlines, first-parent/canonical priority,
      complete history, bootstrap and exact tag/commit validation.
- [ ] Test local bare remotes, immutable reruns, annotated refs, conflicts, failure/retry.
- [ ] Implement argv-based Git and sanitized errors; isolate repository-changing Git env.
- [ ] Run `go test ./...` and record results.

## Task 3: CLI, examples and documentation
Files: `cmd/go-versioning/*.go`, tests, `examples/app/main.go`, README,
LICENSE, CHANGELOG, `.github/workflows/ci.yml` and generic workflow examples.
Consumes Task 1/2 APIs.
- [ ] Write CLI behavior tests first: usage=2, runtime=1, success=0,
      human/JSON output, dry-run flags, no implicit repo/push, canceled context.
- [ ] Implement inspect/next/tag; main handles signals and exit status.
- [ ] Document linker injection, independent library module tags and explicit source resolution.
- [ ] Add consumer example, Go 1.27.3/current CI and Gitea/GitHub tag-allocation examples.
- [ ] Run full suite and consumer example.

## Task 4: Verify, audit and publish
- [ ] Run tests, vet, race, minimum toolchain and cross-platform CGO-disabled builds.
- [ ] Independently review code and address material findings with regression tests.
- [ ] Audit every tracked file and commit metadata for private/sensitive material.
- [ ] Commit clean public-safe history, publish Gitea and public GitHub at identical commits.
- [ ] Configure one-way Gitea-to-GitHub copy using repo-limited authentication; verify a new commit copies.
- [ ] Publish immutable module v0.1.0; independently fetch and test consumer.
- [ ] Verify remote branch/tag identities, repository visibility, mirror status and clean local checkout.
