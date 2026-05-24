package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/doctor"
)

func TestWarmupStateFileAtomicWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "warmup-last.json")
	want := &WarmupSuppressionState{
		Version:        1,
		LastEmissionAt: time.Date(2026, 5, 24, 3, 0, 0, 0, time.UTC),
		FailureSetHash: "abc123",
		LastSubject:    "core-pg:auth alert during city warm-up",
		LastFailures: []SuppressedFailure{
			{Scope: "city", Check: "core-pg:auth", Severity: doctor.StatusError},
		},
	}

	if err := writeWarmupState(path, want); err != nil {
		t.Fatalf("writeWarmupState: %v", err)
	}
	got, err := readWarmupState(path)
	if err != nil {
		t.Fatalf("readWarmupState: %v", err)
	}
	assertWarmupStateEqual(t, got, want)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat state file: %v", err)
	}
	if gotMode, wantMode := info.Mode().Perm(), os.FileMode(0o644); gotMode != wantMode {
		t.Fatalf("mode = %v, want %v", gotMode, wantMode)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read state dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("state dir entries = %d, want 1: %+v", len(entries), entries)
	}
}

func TestWarmupStateFile_CreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gc", "runtime", "warmup-last.json")
	want := &WarmupSuppressionState{
		Version:        1,
		LastEmissionAt: time.Date(2026, 5, 24, 3, 30, 0, 0, time.UTC),
		FailureSetHash: "def456",
	}

	if err := writeWarmupState(path, want); err != nil {
		t.Fatalf("writeWarmupState: %v", err)
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat parent dir: %v", err)
	}
	if gotMode, wantMode := info.Mode().Perm(), os.FileMode(0o755); gotMode != wantMode {
		t.Fatalf("parent mode = %v, want %v", gotMode, wantMode)
	}
	got, err := readWarmupState(path)
	if err != nil {
		t.Fatalf("readWarmupState: %v", err)
	}
	assertWarmupStateEqual(t, got, want)
}

func TestWarmupStateFile_ParseErrorTreatedAsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "warmup-last.json")
	if err := os.WriteFile(path, []byte("not valid json{{"), 0o644); err != nil {
		t.Fatalf("write bad state: %v", err)
	}

	got, err := readWarmupState(path)
	if err != nil {
		t.Fatalf("readWarmupState: %v", err)
	}
	if got != nil {
		t.Fatalf("state = %+v, want nil", got)
	}

	want := &WarmupSuppressionState{Version: 1, FailureSetHash: "ok"}
	if err := writeWarmupState(path, want); err != nil {
		t.Fatalf("writeWarmupState after parse error: %v", err)
	}
	roundTrip, err := readWarmupState(path)
	if err != nil {
		t.Fatalf("readWarmupState after overwrite: %v", err)
	}
	assertWarmupStateEqual(t, roundTrip, want)
}

func TestWarmupStateFile_UnknownVersionTreatedAsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "warmup-last.json")
	if err := os.WriteFile(path, []byte(`{"version":999,"failure_set_hash":"future"}`), 0o644); err != nil {
		t.Fatalf("write future state: %v", err)
	}

	got, err := readWarmupState(path)
	if err != nil {
		t.Fatalf("readWarmupState: %v", err)
	}
	if got != nil {
		t.Fatalf("state = %+v, want nil", got)
	}
}

func TestWarmupStateFile_AbsentFileTreatedAsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "warmup-last.json")

	got, err := readWarmupState(path)
	if err != nil {
		t.Fatalf("readWarmupState: %v", err)
	}
	if got != nil {
		t.Fatalf("state = %+v, want nil", got)
	}
}

func assertWarmupStateEqual(t *testing.T, got, want *WarmupSuppressionState) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("state = %+v, want %+v", got, want)
		}
		return
	}
	if got.Version != want.Version ||
		!got.LastEmissionAt.Equal(want.LastEmissionAt) ||
		got.FailureSetHash != want.FailureSetHash ||
		got.LastSubject != want.LastSubject ||
		len(got.LastFailures) != len(want.LastFailures) {
		t.Fatalf("state = %+v, want %+v", got, want)
	}
	for i := range got.LastFailures {
		if got.LastFailures[i] != want.LastFailures[i] {
			t.Fatalf("LastFailures[%d] = %+v, want %+v", i, got.LastFailures[i], want.LastFailures[i])
		}
	}
}
