package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/doctor"
)

type runTargetRoutedToBackfillCheck struct {
	cfg      *config.City
	cityPath string
	newStore func(string) (beads.Store, error)
}

func newRunTargetRoutedToBackfillCheck(cfg *config.City, cityPath string, newStore func(string) (beads.Store, error)) *runTargetRoutedToBackfillCheck {
	return &runTargetRoutedToBackfillCheck{cfg: cfg, cityPath: cityPath, newStore: newStore}
}

func (c *runTargetRoutedToBackfillCheck) Name() string { return "run-target-routed-to-backfill" }

func (c *runTargetRoutedToBackfillCheck) CanFix() bool { return true }

func (c *runTargetRoutedToBackfillCheck) WarmupEligible() bool { return false }

type runTargetBackfillTarget struct {
	label     string
	store     beads.Store
	beadID    string
	runTarget string
}

func (c *runTargetRoutedToBackfillCheck) collect() (targets []runTargetBackfillTarget, skipped []string) {
	scopes := []struct{ label, path string }{{"city", c.cityPath}}
	if c.cfg != nil {
		for _, rig := range c.cfg.Rigs {
			if rig.Suspended || strings.TrimSpace(rig.Path) == "" {
				continue
			}
			scopes = append(scopes, struct{ label, path string }{"rig " + rig.Name, rig.Path})
		}
	}
	for _, sc := range scopes {
		if c.newStore == nil || strings.TrimSpace(sc.path) == "" {
			continue
		}
		store, err := c.newStore(sc.path)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s skipped: opening bead store: %v", sc.label, err))
			continue
		}
		items, err := store.List(beads.ListQuery{Metadata: map[string]string{"gc.kind": "workflow"}})
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s skipped: listing beads: %v", sc.label, err))
			continue
		}
		for _, b := range items {
			runTarget := strings.TrimSpace(b.Metadata["gc.run_target"])
			if runTarget == "" || strings.TrimSpace(b.Metadata["gc.routed_to"]) != "" {
				continue
			}
			targets = append(targets, runTargetBackfillTarget{label: sc.label, store: store, beadID: b.ID, runTarget: runTarget})
		}
	}
	return targets, skipped
}

func (c *runTargetRoutedToBackfillCheck) Run(_ *doctor.CheckContext) *doctor.CheckResult {
	targets, skipped := c.collect()
	if len(targets) == 0 && len(skipped) == 0 {
		return okCheck(c.Name(), "no workflow roots need gc.routed_to backfill")
	}
	details := make([]string, 0, len(targets)+len(skipped))
	for _, target := range targets {
		details = append(details, fmt.Sprintf("%s bead %s has gc.run_target=%q with empty gc.routed_to", target.label, target.beadID, target.runTarget))
	}
	details = append(details, skipped...)
	sort.Strings(details)
	if len(targets) == 0 {
		return warnCheck(c.Name(),
			fmt.Sprintf("gc.routed_to backfill skipped %d scope(s)", len(skipped)),
			"fix bead store access, then rerun gc doctor",
			details)
	}
	return warnCheck(c.Name(),
		fmt.Sprintf("%d workflow root(s) carry gc.run_target without gc.routed_to", len(targets)),
		"run gc doctor --fix to backfill gc.routed_to from gc.run_target",
		details)
}

func (c *runTargetRoutedToBackfillCheck) Fix(_ *doctor.CheckContext) error {
	targets, _ := c.collect()
	for _, target := range targets {
		if err := target.store.SetMetadata(target.beadID, "gc.routed_to", target.runTarget); err != nil {
			return fmt.Errorf("%s bead %s: backfill gc.routed_to: %w", target.label, target.beadID, err)
		}
	}
	return nil
}
