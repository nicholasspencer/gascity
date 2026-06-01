# Release Gate: ga-i6r4qd restart marker clear errors

Date: 2026-06-01
Outcome: FAIL

## Scope

- Deploy bead: ga-i6r4qd
- Source bead: ga-8x3f5g.5
- Review bead: ga-tgjsk6
- Feature branch: builder/ga-8x3f5g.5-clear-restart-errors
- Reviewed commit: e6f53f657aaf2b3ca24c65501542a641defbadce
- Base checked: origin/main at 882955678

## Gate Criteria

| # | Criterion | Result | Evidence |
|---|-----------|--------|----------|
| 1 | Review PASS present | PASS | ga-tgjsk6 is closed with `pass`; notes say the reviewer passed ga-i6r4qd and found no blocking issues. |
| 2 | Acceptance criteria met | PASS | Reviewed commit e6f53f657 surfaces non-gone `clearRestartRequested` errors with session/bead context, suppresses gone-session races, adds `runtime.Fake.RemoveMetaErrors`, and adds focused coverage for the reset-pending tick-2 wake path. |
| 3 | Tests pass | FAIL | Not run for final release branch because the branch cannot be cleanly merged onto current origin/main. Builder notes report focused reset tests, `make test-fast-parallel`, `go vet ./...`, and `make dashboard-check` passed before handoff. |
| 4 | No high-severity review findings open | PASS | ga-tgjsk6 notes record no security concerns or blocking findings. |
| 5 | Final branch is clean | FAIL | The branch cannot be used as the PR branch in its current form; it is stacked on many unrelated commits. Local worktrees also have an unrelated unstaged deletion of `schemas/convoy/target/result.schema.json`, which was not touched by this gate. |
| 6 | Branch diverges cleanly from main | FAIL | `git merge-tree $(git merge-base origin/main e6f53f657) origin/main e6f53f657` reports multiple `changed in both` conflicts. |
| 7 | Single feature theme | FAIL | `git log --ancestry-path $(git merge-base origin/main e6f53f657)..e6f53f657` reports 43 commits, and `git diff --name-only $(git merge-base origin/main e6f53f657)..e6f53f657` reports 92 paths across session reset, beads, coordstore, docs, generated API/dashboard files, schemas, CI, and release gates. The reviewed commit is a coherent session/reconciler slice, but the branch is not a single-feature PR. |

## Command Evidence

```text
$ git merge-base origin/main e6f53f657
4b19290c8ea2713c250c9cf9f073ea64236e9cc5

$ git log --oneline --ancestry-path 4b19290c8ea2713c250c9cf9f073ea64236e9cc5..e6f53f657 | wc -l
43

$ git diff --name-only 4b19290c8ea2713c250c9cf9f073ea64236e9cc5..e6f53f657 | wc -l
92

$ git diff --name-only e6f53f657^..e6f53f657
cmd/gc/session_reconciler.go
cmd/gc/session_reconciler_restart_request_test.go
internal/runtime/fake.go
internal/runtime/fake_test.go

$ git merge-tree $(git merge-base origin/main e6f53f657) origin/main e6f53f657 | rg -n '^(<<<<<<<|=======|>>>>>>>)|changed in both|added in both|removed in'
multiple `changed in both` conflicts reported
```

## Diagnosis

The reviewed implementation appears to be a valid single-feature session/reconciler slice, but the deploy branch is stacked on unrelated work and does not merge cleanly with current `origin/main`. Deployer did not resolve these conflicts and did not open a PR.

Builder should prepare a clean branch from current `origin/main` containing only e6f53f657's reviewed session/reconciler diff plus this release-gate artifact, rerun the required validation, and reroute for review/deploy.
