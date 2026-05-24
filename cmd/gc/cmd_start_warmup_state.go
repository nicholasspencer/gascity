package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gastownhall/gascity/internal/doctor"
	"github.com/gastownhall/gascity/internal/fsys"
)

// WarmupSuppressionState is the on-disk snapshot of the last warm-up alert emission.
type WarmupSuppressionState struct {
	Version        int                 `json:"version"`
	LastEmissionAt time.Time           `json:"last_emission_at"`
	FailureSetHash string              `json:"failure_set_hash"`
	LastSubject    string              `json:"last_subject,omitempty"`
	LastFailures   []SuppressedFailure `json:"last_failures,omitempty"`
}

// SuppressedFailure identifies a previously failing warm-up check.
type SuppressedFailure struct {
	Scope    string             `json:"scope"`
	Check    string             `json:"check"`
	Severity doctor.CheckStatus `json:"severity"`
}

func defaultWarmupStatePath(cityPath string) string {
	return filepath.Join(cityPath, ".gc", "runtime", "warmup-last.json")
}

func readWarmupState(path string) (*WarmupSuppressionState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		return nil, nil
	}
	var state WarmupSuppressionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, nil
	}
	if state.Version != 1 {
		return nil, nil
	}
	sortSuppressedFailures(state.LastFailures)
	return &state, nil
}

func writeWarmupState(path string, state *WarmupSuppressionState) error {
	if state == nil {
		state = &WarmupSuppressionState{}
	}
	copyState := *state
	copyState.Version = 1
	copyState.LastEmissionAt = copyState.LastEmissionAt.UTC()
	copyState.LastFailures = append([]SuppressedFailure(nil), copyState.LastFailures...)
	sortSuppressedFailures(copyState.LastFailures)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(copyState, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsys.WriteFileAtomic(fsys.OSFS{}, path, data, 0o644)
}

func sortSuppressedFailures(failures []SuppressedFailure) {
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].Scope != failures[j].Scope {
			return failures[i].Scope < failures[j].Scope
		}
		return failures[i].Check < failures[j].Check
	})
}
