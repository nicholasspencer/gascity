package coordstore

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// Target describes a single performance requirement from discovery.md.
type Target struct {
	// Op is the operation name (matches OperationResult.Op).
	Op string
	// Name is a human-readable description.
	Name string
	// P99 is the p99 latency requirement. Zero means no p99 target.
	P99 time.Duration
	// Max is an absolute maximum (used for single-invocation operations like
	// PrimeScan). Zero means no max target.
	Max time.Duration
	// MinThroughput is the minimum sustained throughput in ops/sec.
	// Zero means no throughput target.
	MinThroughput float64
}

// DiscoveryTargets are the performance requirements from discovery.md.
// Source: docs/coordination-store/discovery.md §Targets.
var DiscoveryTargets = []Target{
	{
		Op:   "Get",
		Name: "point read (FR-3)",
		P99:  1 * time.Millisecond,
	},
	{
		Op:   "FilterScan",
		Name: "filter scan main tier (FR-2)",
		P99:  10 * time.Millisecond,
	},
	{
		Op:   "EphemeralFilterScan",
		Name: "filter scan ephemeral tier / mail poll (FR-8)",
		P99:  10 * time.Millisecond,
	},
	{
		Op:   "BatchGet",
		Name: "batch-by-id-set fetch (FR-4)",
		P99:  5 * time.Millisecond,
	},
	{
		Op:   "Create",
		Name: "per-record create (FR-1)",
		P99:  5 * time.Millisecond,
	},
	{
		Op:   "Update",
		Name: "per-record update (FR-1)",
		P99:  5 * time.Millisecond,
	},
	{
		Op:   "SetMetadataBatch",
		Name: "intra-record multi-key atomic write (FR-5)",
		P99:  5 * time.Millisecond,
	},
	{
		Op:   "Ready",
		Name: "ready semantics scan (FR-9)",
		P99:  10 * time.Millisecond,
	},
	{
		Op:   "PrimeScan",
		Name: "background prime at 10k rows (FR-15)",
		Max:  5 * time.Second,
	},
	{
		Op:            "MailPoll",
		Name:          "mail-poll read throughput",
		MinThroughput: 150,
	},
}

// ScorecardResult is the outcome of a single target check.
type ScorecardResult struct {
	Target Target
	// Actual values measured.
	ActualP99        time.Duration
	ActualMax        time.Duration
	ActualThroughput float64
	// Pass is true if the backend meets the target.
	Pass bool
	// Reason explains why the target was not met, or is empty on pass.
	Reason string
	// Measured is false if no samples were collected for this operation.
	Measured bool
}

// Scorecard aggregates the pass/fail results for a backend+workload run.
type Scorecard struct {
	Backend  string
	Workload string
	Results  []ScorecardResult
	// Duration is the wall-clock time the workload ran.
	Duration time.Duration
	// TotalOps is the total number of operations issued.
	TotalOps int
	// Errors is the total number of operation errors.
	Errors int
}

// Passed returns true if all measured targets passed.
func (s *Scorecard) Passed() bool {
	for _, r := range s.Results {
		if r.Measured && !r.Pass {
			return false
		}
	}
	return true
}

// PassCount returns the number of measured targets that passed.
func (s *Scorecard) PassCount() int {
	n := 0
	for _, r := range s.Results {
		if r.Measured && r.Pass {
			n++
		}
	}
	return n
}

// TotalTargets returns the number of measured targets.
func (s *Scorecard) TotalTargets() int {
	n := 0
	for _, r := range s.Results {
		if r.Measured {
			n++
		}
	}
	return n
}

// Score evaluates operation results against the discovery.md targets.
// results maps operation name → OperationResult.
// throughput maps operation name → ops/sec.
func Score(backend, workload string, dur time.Duration, totalOps, totalErrors int,
	results map[string]*OperationResult, throughput map[string]float64,
) Scorecard {
	sc := Scorecard{
		Backend:  backend,
		Workload: workload,
		Duration: dur,
		TotalOps: totalOps,
		Errors:   totalErrors,
	}

	for _, t := range DiscoveryTargets {
		r := ScorecardResult{Target: t}
		op, ok := results[t.Op]
		if !ok || op == nil || op.Samples == 0 {
			r.Measured = false
			sc.Results = append(sc.Results, r)
			continue
		}
		r.Measured = true
		r.ActualP99 = op.H.P99()
		r.ActualMax = op.H.Max()
		r.ActualThroughput = throughput[t.Op]

		var reasons []string
		if t.P99 > 0 && r.ActualP99 > t.P99 {
			reasons = append(reasons, fmt.Sprintf("p99 %s > target %s",
				FormatDuration(r.ActualP99), FormatDuration(t.P99)))
		}
		if t.Max > 0 && r.ActualMax > t.Max {
			reasons = append(reasons, fmt.Sprintf("max %s > target %s",
				FormatDuration(r.ActualMax), FormatDuration(t.Max)))
		}
		if t.MinThroughput > 0 && r.ActualThroughput < t.MinThroughput {
			reasons = append(reasons, fmt.Sprintf("throughput %.0f/s < target %.0f/s",
				r.ActualThroughput, t.MinThroughput))
		}
		r.Pass = len(reasons) == 0
		r.Reason = strings.Join(reasons, "; ")
		sc.Results = append(sc.Results, r)
	}

	return sc
}

// PrintTable writes the scorecard as a human-readable table to w.
func (s *Scorecard) PrintTable(w io.Writer) {
	status := "PASS"
	if !s.Passed() {
		status = "FAIL"
	}
	fmt.Fprintf(w, "\n=== Scorecard: %s / %s — %s ===\n", s.Backend, s.Workload, status) //nolint:errcheck
	fmt.Fprintf(w, "  duration=%s  ops=%d  errors=%d  targets=%d/%d passed\n\n",         //nolint:errcheck
		FormatDuration(s.Duration), s.TotalOps, s.Errors, s.PassCount(), s.TotalTargets())

	const colW = 38
	fmt.Fprintf(w, "  %-*s  %-12s  %-12s  %-12s  %s\n", //nolint:errcheck
		colW, "Target", "P99", "Max", "Throughput", "Result")
	fmt.Fprintf(w, "  %s\n", strings.Repeat("-", colW+50)) //nolint:errcheck

	for _, r := range s.Results {
		result := "skip"
		p99 := "-"
		maxVal := "-"
		tput := "-"

		if r.Measured {
			if r.Pass {
				result = "PASS"
			} else {
				result = "FAIL  ← " + r.Reason
			}
			if r.Target.P99 > 0 {
				p99 = FormatDuration(r.ActualP99)
			}
			if r.Target.Max > 0 {
				maxVal = FormatDuration(r.ActualMax)
			}
			if r.Target.MinThroughput > 0 {
				tput = fmt.Sprintf("%.0f/s", r.ActualThroughput)
			}
		}

		fmt.Fprintf(w, "  %-*s  %-12s  %-12s  %-12s  %s\n", //nolint:errcheck
			colW, r.Target.Name, p99, maxVal, tput, result)
	}
	fmt.Fprintln(w) //nolint:errcheck
}
