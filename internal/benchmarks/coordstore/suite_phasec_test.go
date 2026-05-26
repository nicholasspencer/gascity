package coordstore_test

import (
	"testing"
	"time"
)

func TestFullMatrixAdaptersSelectsTargetBackendsInStableOrder(t *testing.T) {
	adapters := []adapterFactory{
		{name: "sqlite"},
		{name: "badger"},
		{name: "hqstore"},
		{name: "authorcore"},
		{name: "sqlite-cgo"},
		{name: "bbolt"},
	}

	got := fullMatrixAdapters(adapters)
	var names []string
	for _, adapter := range got {
		names = append(names, adapter.name)
	}
	want := []string{"hqstore", "bbolt", "sqlite-cgo", "badger"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}
}

func TestSoakConfigFromEnvParsesSeparateChaosDuration(t *testing.T) {
	t.Setenv("COORDSTORE_SOAK_DURATION", "6h")
	t.Setenv("COORDSTORE_CHAOS_DURATION", "1h")

	cfg := soakConfigFromEnv(t, 4*time.Hour)
	if cfg.SoakDuration != 6*time.Hour {
		t.Fatalf("SoakDuration = %s, want 6h", cfg.SoakDuration)
	}
	if cfg.ChaosDuration != time.Hour {
		t.Fatalf("ChaosDuration = %s, want 1h", cfg.ChaosDuration)
	}
}
