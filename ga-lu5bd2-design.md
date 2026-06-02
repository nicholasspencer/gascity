# Design: CGO Store Cleanup (ga-lu5bd2)

**Bead:** ga-lu5bd2  
**Date:** 2026-06-02  
**Architect source:** ga-ml09dv  
**Depends on:** ga-t8lwle + ga-ayn8o1 (both must be merged first)  
**Diagram:** https://excalidraw.com/#json=C35TqM62SnbPFHRwIxIM3,QCsuPbsflp399wqSCvIywQ

---

## Summary

Once `openCoordStoreAt` calls `OpenSQLiteStore`, the CGO store files are dead code. This slice deletes them and removes the `mattn/go-sqlite3` dependency.

**Gate:** Do NOT run this until ga-t8lwle and ga-ayn8o1 are merged to main and `openCoordStoreAt` is confirmed wired to `OpenSQLiteStore`.

---

## Files to Delete

```bash
rm internal/beads/sqlite_cgo_store.go
rm internal/beads/sqlite_cgo_store_stub.go
# If present on main (was on investigate/ga-qsvwe1 branch — check first):
rm internal/beads/sqlite_cgo_store_leak_test.go
```

---

## Dependency Verification

Before removing, confirm no other production code imports the cgo driver:

```bash
grep -rn 'mattn/go-sqlite3\|SQLiteCGO\|OpenSQLiteCGO\|sqlite_cgo' \
    /home/jaword/projects/gascity --include='*.go' | grep -v '_test.go'
```

Expected: zero results. If results appear, investigate before proceeding.

---

## go.mod Cleanup

```bash
go mod tidy
```

Verify `go.mod` no longer contains `mattn/go-sqlite3`.  
Verify `go.sum` no longer contains mattn entries.

---

## Build Tag Cleanup

Remove `-tags sqlite_cgo` from any build system reference:

```bash
grep -rn 'sqlite_cgo' /home/jaword/projects/gascity Makefile .github/
```

Remove every hit from Makefile targets and `.github/workflows/*.yml` CI jobs.

---

## Acceptance Criteria

| Check | Command | Expected |
|-------|---------|----------|
| Pure-Go build | `CGO_ENABLED=0 go build ./cmd/gc/...` | EXIT 0 |
| Full build clean | `CGO_ENABLED=0 go build ./...` | EXIT 0 |
| No mattn dep | `grep mattn go.mod` | no output |
| Tests pass | `make test-fast-parallel` | GREEN |
| Vet clean | `go vet ./...` | no output |
| No build tags | `grep -r sqlite_cgo .` | no output |
| Leak test passes | `TestSQLiteStoreNoLeakOnDiscard` | PASS |
