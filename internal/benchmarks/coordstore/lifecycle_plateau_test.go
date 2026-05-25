package coordstore_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/benchmarks/coordstore"
	"github.com/gastownhall/gascity/internal/benchmarks/coordstore/adapters/authorcore"
)

// TestLifecycleWorkloadPlateaus is the ga-w08fz regression guard. With the
// lifecycle ops (Complete/Archive/PurgeExpired) the working set PLATEAUS near
// the seeded steady-state size under heavy create load; with them disabled
// (the legacy net-create-only workload) the set grows without bound. A soak run
// against an in-memory backend would otherwise just benchmark each backend's
// compression of an ever-growing dataset rather than real steady-state work.
func TestLifecycleWorkloadPlateaus(t *testing.T) {
	if testing.Short() {
		t.Skip("timed workload; skipped in -short")
	}

	base := coordstore.WorkloadConfig{
		Name:            "plateau-test",
		MainOpenCount:   50,
		MainClosedCount: 100,
		WispOpenCount:   50,
		// Heavy create pressure so net-create-only growth is unmistakable.
		MailPollRate:  10,
		PointReadRate: 10,
		CreateRate:    100,
		UpdateRate:    5,
		Duration:      2 * time.Second,
		Concurrency:   8,
	}

	run := func(lifecycle bool) int64 {
		wl := base
		if lifecycle {
			// Balance the create rate (≈50/s main + ≈50/s wisp).
			wl.CompleteRate = 50
			wl.ArchiveRate = 50
			wl.PurgeRate = 5
		}
		ctx := context.Background()
		a := authorcore.New()
		if err := a.Open(ctx, coordstore.Config{}); err != nil {
			t.Fatalf("open: %v", err)
		}
		seed, err := coordstore.NewSeeder(1).Seed(ctx, a, wl)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		if _, err := coordstore.NewRunner(a, wl, seed).Run(ctx, io.Discard); err != nil {
			t.Fatalf("run: %v", err)
		}
		stats := a.Stats(ctx)
		return stats["main_records"] + stats["ephemeral_records"]
	}

	const seededTotal = 50 + 100 + 50 // open main + closed main + open wisp

	withLifecycle := run(true)
	createOnly := run(false)
	t.Logf("seeded=%d  lifecycle-ON total=%d  create-only total=%d",
		seededTotal, withLifecycle, createOnly)

	// Plateau: lifecycle keeps the working set bounded near the seeded size.
	if withLifecycle > seededTotal*3 {
		t.Errorf("lifecycle workload did not plateau: total=%d, want <= %d (seeded %d)",
			withLifecycle, seededTotal*3, seededTotal)
	}
	// Sanity: the create-only workload must visibly grow, else the test is not
	// generating enough create pressure to be a meaningful contrast.
	if createOnly < seededTotal*4 {
		t.Errorf("create-only workload did not grow enough to be meaningful: total=%d (seeded %d)",
			createOnly, seededTotal)
	}
	// The whole point: lifecycle is dramatically smaller than create-only.
	if withLifecycle*2 >= createOnly {
		t.Errorf("lifecycle total (%d) not meaningfully smaller than create-only (%d)",
			withLifecycle, createOnly)
	}
}
