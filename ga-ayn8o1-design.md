# Design: Wire SQLiteStore + Fix dispatch() Store Leak (ga-ayn8o1)

**Bead:** ga-ayn8o1  
**Date:** 2026-06-02  
**Architect source:** ga-ml09dv  
**Depends on:** ga-t8lwle (SQLiteStore must exist before this compiles)  
**Diagram:** https://excalidraw.com/#json=i9i4uodZUKMOlettP9q60,Zl-Qud5KKLJeY6q12wm5rQ

---

## Summary

Two targeted changes in `cmd/gc/`, both small:

1. Swap driver in `openCoordStoreAt` (one call site)
2. Add `defer` close loop in `dispatch()` to fix per-tick store leak

---

## Change 1: cmd/gc/main.go — driver swap

Single substitution in `openCoordStoreAt`:

```go
// BEFORE
return beads.OpenSQLiteCGOStore(
    storeDir,
    beads.WithSQLiteCGOStoreIDPrefix(issuePrefixForScope(scopeRoot, cityPath, cfg)),
    beads.WithSQLiteCGOStoreRetention(4*time.Hour, 30*time.Second),
)

// AFTER
return beads.OpenSQLiteStore(
    storeDir,
    beads.WithSQLiteStoreIDPrefix(issuePrefixForScope(scopeRoot, cityPath, cfg)),
    beads.WithSQLiteStoreRetention(4*time.Hour, 30*time.Second),
)
```

Provider strings `"sqlite"` and `"sqlite-cgo"` in the `openStoreAtForCity` switch remain **unchanged** — they are user-visible config values, not code paths.

---

## Change 2: cmd/gc/order_dispatch.go — leak fix

**The bug:** `dispatch()` opens stores per tick into a local `stores` map via `m.storeFn`. Each `OpenSQLiteStore` call starts a retention goroutine. The map goes out of scope on return without ever calling `CloseStore()`. After N reconcile ticks: N leaked goroutines + N leaked DB connections.

**The fix:** Register a deferred cleanup immediately after `stores` is declared.

```go
func (m *memoryOrderDispatcher) dispatch(ctx context.Context, cityPath string, now time.Time) {
    if m.cfg != nil && citySuspended(m.cfg) {
        return
    }

    stores := make(map[string]beads.Store)
    defer func() {                          // ADD: close all stores opened this tick
        for _, s := range stores {
            if c, ok := s.(interface{ CloseStore() error }); ok {
                if err := c.CloseStore(); err != nil {
                    logDispatchError(m.stderr, "gc: order dispatch: closing store: %v", err)
                }
            }
        }
    }()

    // rest of function unchanged
```

**Why `defer`:** Fires on every return path — normal return, early return (suspended city, error), and any future returns added by later patches.

**Why interface assertion:** `CloseStore()` is not part of `beads.Store`. Stores without it (MemStore, filestore, bd exec store) are silently skipped, matching the existing `closeBeadStoreHandle` detection pattern. Driver-agnostic.

---

## Goroutine lifecycle (after fix)

```
tick N:
  dispatch() called
  stores map created
  defer registered
  N stores opened (each starts retention goroutine)
  order logic runs
  dispatch() returns
  defer fires: CloseStore() on all N stores
    → retentionStop() cancels each context
    → <-retentionDone waits sweeper exit
    → readDB.Close() + writeDB.Close()
  net goroutine delta: 0
```

---

## Test Requirement

File: `cmd/gc/order_dispatch_test.go`

**`TestDispatchClosesPerTickStores`:**
- Create spy store wrapping `beads.NewMemStore()` with `CloseStore()` that records calls
- Run N `dispatch()` ticks via the dispatcher's `storeFn`
- Assert `CloseStore` was called once per tick
- Assert no stores remain open after dispatch returns

---

## Critical Guardrails

1. **Do NOT change provider strings** (`"sqlite"`, `"sqlite-cgo"`) — user-visible config
2. **`defer` placement is mandatory** — must be immediately after `stores :=` so all paths are covered
3. **Log CloseStore errors** via `logDispatchError` — do not silently discard
4. **Depends on ga-t8lwle** — must compile against new `OpenSQLiteStore` function
