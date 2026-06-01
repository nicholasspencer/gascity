# Release Gate: ga-yinjm2.1 HQStore Assignees Index Union

Date: 2026-06-01
Bead: ga-yinjm2.1
Source review bead: ga-kbehsy
Source commit: 8a6de11d9
Release branch: release/ga-yinjm2-1-hqstore-assignees
Release feature commit before this gate: d670cb69d
Base checked: origin/main b2b659d421ace115fcc47575b202c0a4541ad75a

`docs/PROJECT_MANIFEST.md` is not present in this worktree, so this gate uses the deployer prompt criteria and the bead acceptance criteria.

## Scope

The release diff from current `origin/main` contains the HQStore Assignees index-union slice only:

| Path | Status | Purpose |
|---|---:|---|
| `internal/beads/hqstore_core.go` | M | Union per-assignee HQStore index buckets for `ListQuery.Assignees`. |
| `internal/beads/hqstore_core_internal_test.go` | A | Cover multi-assignee OR semantics and unknown-assignee empty union behavior. |
| `release-gates/ga-yinjm2-1-hqstore-assignees-gate.md` | A | Release gate evidence. |

## Gate Checklist

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Review PASS present | PASS | Review bead `ga-kbehsy` is closed with close reason `pass`; notes contain `PASS (reviewer: gascity/reviewer)` for source commit `8a6de11d9`. |
| 2 | Acceptance criteria met | PASS | Dependency PR #2865 is recorded merged at `2026-06-01T05:16:20Z`; current release diff excludes the coordstore/SQLite PR #2738 stack; behavior from `ga-kbehsy` is preserved by code at `internal/beads/hqstore_core.go` and tests in `internal/beads/hqstore_core_internal_test.go`. |
| 3 | Tests pass | PASS | `go test ./internal/beads -run 'TestHQTierIndexCandidateIDs(AssigneesEmptyUnionStaysEmpty\|UnionsAssignees)' -count=1` PASS; `go test ./internal/beads -count=1` PASS; `go vet ./...` PASS; `make test` PASS with observable log `/tmp/gascity-test.jsonl.i3gzST`. |
| 4 | No high-severity review findings open | PASS | Review notes report no security concerns and no unresolved HIGH findings; unresolved HIGH count is 0. |
| 5 | Final branch is clean | PASS | `git status --short --branch` before gate write showed no uncommitted feature changes; final clean status is verified after committing this gate. |
| 6 | Branch diverges cleanly from main | PASS | `git fetch origin main` refreshed current base; cherry-pick onto `origin/main` was clean; `git merge-tree origin/main HEAD` exited 0; `git diff --check origin/main...HEAD` exited 0. |
| 7 | Single feature theme | PASS | Commit set touches one subsystem, `internal/beads`, and implements one behavior: indexed candidate selection for multi-assignee HQStore list queries. |

## Acceptance Notes

- `ListQuery.Assignees` contract dependency is on `origin/main` via PR #2865 before this slice ships.
- Per-assignee index buckets are OR-unioned when `ListQuery.Assignees` is set.
- Unknown assignees produce an empty candidate set, including when combined with status filters and when closed beads are included.
- Singular `Assignee` behavior is unchanged; `ListQuery.Validate()` keeps singular and plural assignee forms mutually exclusive.

Gate result: PASS.
