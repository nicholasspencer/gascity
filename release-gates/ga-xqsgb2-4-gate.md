# Release Gate: ga-xqsgb2-4

Status: FAIL

Source bead: ga-xqsgb2.4
Deploy bead: ga-xqsgb2.4.1
Branch: builder/ga-xqsgb2-4
Commit: 40dab6eaa0f9b7926d7fb9cc69c40681ed69a3f4
Base checked: origin/main at ef7fb4f1e22ff696086c96033e66dc003ef7b9c9

`docs/PROJECT_MANIFEST.md` is not present in this worktree, so this gate uses
the deployer role's release criteria table plus the repo testing policy in
`TESTING.md`.

## Criteria

| # | Criterion | Result | Evidence |
|---|-----------|--------|----------|
| 1 | Review PASS present | PASS | `bd show ga-xqsgb2.4.1` contains `VERDICT: pass` for branch `builder/ga-xqsgb2-4`. |
| 2 | Acceptance criteria met | PASS | Review notes confirm close/reopen lifecycle write pairs use Store.Tx, rollback keeps explicit session_name clearing outside the close transaction, and focused tests cover the behavior. |
| 3 | Tests pass | FAIL | Release-gate tests were not run because criterion 6 failed before a clean final branch could be evaluated. Builder/reviewer notes report prior `go test ./...` and `go vet ./...` passed on the stale branch. |
| 4 | No high-severity review findings open | PASS | Review notes list LOW/INFO findings only; no HIGH or CRITICAL findings are present. |
| 5 | Final branch is clean | PASS | `git status --short --branch` was clean before writing this gate file; this gate file is committed as the only deployer change on the feature branch. |
| 6 | Branch diverges cleanly from main | FAIL | `git merge-tree origin/main origin/builder/ga-xqsgb2-4` reported content conflicts in `internal/beads/beadstest/conformance.go` and `internal/beads/caching_store_writes.go`. |

## Failure Diagnosis

The prior blocker PR #2309 has merged into `origin/main`, but this downstream
branch still carries the older transactional-write stack. The deployer must not
resolve content conflicts or rebase release branches, so this bead is routed
back to builder for a rebuild on current `origin/main`.
