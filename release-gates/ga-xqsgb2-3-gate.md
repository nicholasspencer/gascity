# Release Gate: ga-xqsgb2-3

Status: FAIL

Source bead: ga-xqsgb2.3
Deploy bead: ga-340dma
Branch: builder/ga-xqsgb2-3
Commit: ee2173a88f576091119e1a7aa573b40a5cc1d31a
Base checked: origin/main at ef7fb4f1e22ff696086c96033e66dc003ef7b9c9

`docs/PROJECT_MANIFEST.md` is not present in this worktree, so this gate uses
the deployer role's release criteria table plus the repo testing policy in
`TESTING.md`.

## Criteria

| # | Criterion | Result | Evidence |
|---|-----------|--------|----------|
| 1 | Review PASS present | PASS | `bd show ga-340dma` contains `VERDICT: pass` for branch `builder/ga-xqsgb2-3` at `ee2173a88f576091119e1a7aa573b40a5cc1d31a`. |
| 2 | Acceptance criteria met | PASS | Review notes confirm CachingStore.Tx tracks mutated bead IDs, delegates to backing Tx without holding the cache mutex, evicts touched bead/dependency/dirty/deleted cache entries after successful commit, leaves cache entries unchanged on errors, and covers touched-ID invalidation, error propagation, and zero-touch behavior. |
| 3 | Tests pass | FAIL | Release-gate tests were not run because criterion 6 failed before a clean final branch could be evaluated. Builder/reviewer notes report prior focused tests, `make test-fast-parallel`, and `go vet ./...` passed on the stale branch. |
| 4 | No high-severity review findings open | PASS | Review notes list no blocking findings and no HIGH or CRITICAL findings. |
| 5 | Final branch is clean | PASS | `git status --short --branch` was clean before writing this gate file; this gate file is committed as the only deployer change on the feature branch. |
| 6 | Branch diverges cleanly from main | FAIL | `git merge-tree origin/main fork/builder/ga-xqsgb2-3` reported content conflicts in `internal/beads/beadstest/conformance.go` and `internal/beads/caching_store_writes.go`. |

## Failure Diagnosis

The prior blocker PR #2309 has merged into `origin/main`, but this downstream
branch still carries an older transactional-write stack. It no longer merges
cleanly with current `origin/main`.

The deployer must not resolve content conflicts or rebase release branches.
Route this bead back to builder so the branch can be rebuilt on current
`origin/main` and re-reviewed if the resulting diff changes materially.
