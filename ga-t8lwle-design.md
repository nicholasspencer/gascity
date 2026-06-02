# Design: SQLiteStore — modernc Pure-Go Implementation (ga-t8lwle)

**Bead:** ga-t8lwle  
**Date:** 2026-06-02  
**Architect source:** ga-ml09dv  
**Diagram:** https://excalidraw.com/#json=dxLK-lZOGSYtP591coEkX,TUIo6zqsuIlwBnl_ikUZYg

---

## Summary

Implement `SQLiteStore` in `internal/beads/sqlite_store.go` as a drop-in replacement for `SQLiteCGOStore`. Uses `modernc.org/sqlite` (pure-Go, already in `go.mod`). No build tag — compiles unconditionally with `CGO_ENABLED=0`.

---

## Struct Layout

```
SQLiteStore
├── db          *sql.DB              — write connection (MaxOpenConns=1, serializes mutations)
├── readDB      *sql.DB              — read pool (MaxOpenConns=8, MaxIdleConns=8)
├── path        string
├── prefix      string
├── retentionPeriod    time.Duration
├── retentionStop      context.CancelFunc   — cancel to stop sweeper
├── retentionDone      chan struct{}          — signal sweeper exited
├── seq         atomic.Int64          — no DB round-trip per Create
├── closeOnce   sync.Once             — idempotent CloseStore()
```

Key structural differences from `SQLiteCGOStore`:

| Field | CGO store | New store |
|-------|-----------|-----------|
| Driver import | `_ "github.com/mattn/go-sqlite3"` | `_ "modernc.org/sqlite"` |
| Single DB | One `*sql.DB` for all ops | `writeDB` (1 conn) + `readDB` (pool=8) |
| Sweeper stop | `retentionStop chan struct{}` | `context.CancelFunc` + `retentionDone chan struct{}` |
| ID sequence | DB round-trip via KV table | `atomic.Int64` recovered on Open |
| Build tag | `//go:build cgo && sqlite_cgo` | **NONE** |

---

## Open Lifecycle

```
OpenSQLiteStore(dir, opts)
  → open writeDB (driver="sqlite", MaxConns=1)
  → apply WAL pragmas + CREATE TABLE IF NOT EXISTS (beads, labels, metadata, deps, kv)
  → recoverSequence: SELECT MAX(id) → seq.Store(maxNumericSuffix)
  → open readDB (driver="sqlite", MaxConns=8, MaxIdle=8)
  → startRetentionSweep(ctx)  — goroutine: select { ticker.C | ctx.Done }
  → return Store
```

---

## CloseStore() Contract

```go
func (s *SQLiteStore) CloseStore() error {
    var err error
    s.closeOnce.Do(func() {
        if s.retentionStop != nil { s.retentionStop() }
        if s.retentionDone != nil { <-s.retentionDone }
        if s.readDB != nil { if e := s.readDB.Close(); e != nil { err = e } }
        if s.db != nil { if e := s.db.Close(); e != nil && err == nil { err = e } }
    })
    return err
}
```

Detected dynamically via `interface{ CloseStore() error }` — NOT part of `beads.Store` interface.

---

## Schema

**Production schema** (same as CGO store — do NOT use benchmark adapter schema):

```sql
CREATE TABLE IF NOT EXISTS beads (
    id TEXT PRIMARY KEY, tier TEXT NOT NULL, title TEXT NOT NULL,
    status TEXT NOT NULL, issue_type TEXT NOT NULL, priority INTEGER,
    created_at INTEGER, updated_at INTEGER, assignee TEXT, from_agent TEXT,
    parent_id TEXT, ref TEXT, description TEXT, bead_json TEXT
);
CREATE TABLE IF NOT EXISTS labels (bead_id TEXT, label TEXT, PRIMARY KEY (bead_id, label), FOREIGN KEY (bead_id) REFERENCES beads(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS metadata (bead_id TEXT, meta_key TEXT, meta_value TEXT, PRIMARY KEY (bead_id, meta_key), FOREIGN KEY (bead_id) REFERENCES beads(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS deps (issue_id TEXT, depends_on_id TEXT, dep_type TEXT, PRIMARY KEY (issue_id, depends_on_id, dep_type));
CREATE TABLE IF NOT EXISTS kv (key TEXT PRIMARY KEY, value TEXT);
```

WAL pragmas:
```sql
PRAGMA journal_mode=WAL;
PRAGMA synchronous=FULL;
PRAGMA wal_autocheckpoint=1000;
PRAGMA busy_timeout=5000;
PRAGMA foreign_keys=ON;
```

---

## Public API (callers unchanged)

```go
func OpenSQLiteStore(dir string, opts ...SQLiteStoreOption) (Store, error)
func WithSQLiteStoreIDPrefix(prefix string) SQLiteStoreOption
func WithSQLiteStoreRetention(period, sweepInterval time.Duration) SQLiteStoreOption
```

---

## Test Requirements

File: `internal/beads/sqlite_store_test.go`

| Test | Assert |
|------|--------|
| `TestSQLiteStoreCreatesAndGets` | Create + Get round-trip |
| `TestSQLiteStoreReady` | Unblocked beads returned; blocked excluded |
| `TestSQLiteStoreCloseStore` | Open + CloseStore → goroutine count returns to baseline |
| `TestSQLiteStoreNoLeakOnDiscard` | Open N stores without CloseStore → goroutines elevate; with CloseStore → count stable |

Port leak test from `investigate/ga-qsvwe1-coordstore-leak @1ea16a7a3`. Remove `cgo && sqlite_cgo` build tag. Use `runtime.GC()` + `time.Sleep(50ms)` settle pattern.

---

## Critical Guardrails

1. **No build tag** — `sqlite_store.go` compiles unconditionally
2. **Schema = beads table** — not `records`/`ephemeral` (benchmark adapter schema)
3. **`CGO_ENABLED=0 go build ./internal/beads/...` must pass**
4. **`closeOnce` is mandatory** — callers call CloseStore multiple times in tests
5. **Context-based sweeper** — sweeper selects on `ctx.Done()`, not just ticker; no deadlock on close
