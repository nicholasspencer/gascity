package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/events"
)

func withCommandBeadEvents(result beads.StoreOpenResult, recorder events.Recorder) beads.StoreOpenResult {
	if result.Store == nil || recorder == nil {
		return result
	}
	if result.Diagnostic.Store != beads.BeadsStoreNameNativeDoltStore {
		return result
	}
	result.Store = beads.NewNotifyingStore(result.Store, func(eventType, beadID string, payload json.RawMessage) {
		recorder.Record(events.Event{
			Type:    eventType,
			Actor:   "gc",
			Subject: beadID,
			Payload: payload,
		})
	})
	return result
}

func openCommandBeadEventRecorder(cityPath string) events.Provider {
	if cityPath == "" {
		return nil
	}
	eventsCfg := config.EventsConfig{}
	if cfg, err := loadCityConfig(cityPath, io.Discard); err == nil {
		eventsCfg = cfg.Events
	}
	if v := os.Getenv("GC_EVENTS"); v != "" {
		eventsCfg.Provider = v
	}
	eventsPath := filepath.Join(cityPath, ".gc", "events.jsonl")
	if eventsCfg.Provider == "fake" || eventsCfg.Provider == "fail" || strings.HasPrefix(eventsCfg.Provider, "exec:") {
		eventsPath = ""
	}
	provider, err := newEventsProviderForNameWithConfig(eventsCfg.Provider, eventsPath, io.Discard, eventsCfg)
	if err != nil {
		return nil
	}
	return provider
}
