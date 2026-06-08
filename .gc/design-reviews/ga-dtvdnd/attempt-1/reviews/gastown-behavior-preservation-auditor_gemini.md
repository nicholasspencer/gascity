# Oleg Marchetti — DeepSeek V4 Flash Perspective Independent Review (Iteration 1 / Attempt 1)

**Verdict:** approve-with-risks

**Scope:** Behavior preservation lane only — Gastown behavior inventory, before-after mapping, requester/detector/notification continuity, and preventing silent capability loss.

This review evaluates the Iteration 1 / Attempt 1 draft of the Core/Gastown Split design document against the approved requirements and current codebase behaviors. The design presents a highly mature, rigorous strategy for de-roling the SDK while preserving Gastown orchestration. In particular, the introduction of a machine-readable **Source-Derived Behavior Manifest (§88–120)** and a **Strict Behavior Witness Floor (§1393–1415)** provides an exceptionally strong defense against silent regressions.

However, from the strict, empirical perspective of **Behavior Preservation Auditing**, several critical risks, gaps, and edge cases must be addressed before the design is finalized.

---

## Executive Summary

The design outlines a systematic, staged rollout plan (§2723–2809) that decouples risky changes, enforces intermediate test states, and establishes a source-derived behavior manifest. This manifest ensures that every generalized asset's old trigger conditions and notification targets are exhaustively cataloged and validated against the new public Gastown pack via `test/packcompat`.

To ensure 100% behavioral equivalence and prevent silent operational failures, we identify three critical areas requiring refinement:
1. **The `dog` Worker Omission and Rename Fallback Gap:** Go-side SDK functions must survive when the Core maintenance worker `dog` is renamed or omitted, yet active provider formulas (like `mol-dog-backup` under Dolt) assume a valid maintenance worker is configured.
2. **Silent Mail Failures and Empty Recipient Misroutes in Shell Scripts:** Generalizing scripts (e.g., `reaper.sh` and `jsonl-export.sh`) to accept recipient parameters dynamically introduces a risk of invalid CLI executions (such as `gc mail send /`) or silent failures when no recipient is configured.
3. **Behavior Witness triggers on Simulated Failures:** The current test plan verifies happy path equivalence, but does not explicitly mandate verifying that error/warning/escalation paths trigger correctly.

---

## Top Strengths

- **Behavior Evidence Witness Floor (§1393–1400):** Mandating that any asset with historical execution-level test coverage cannot be downgraded to static path-presence or count-only validation is an outstanding quality gate.
- **Source-Derived, Machine-Readable Manifest (§88–120):** Transitioning away from hand-curated Excel sheets or markdown tables to a compiler-verifiable `behavior-manifest.generated.yaml` prevents human oversight from letting role leakage slip through.
- **Strict Staged Rollout Slices (§2723–2809):** Dividing the migration into seven highly specified slices (from candidate public Gastown branch to final source deletion) with explicit entry/exit gates minimizes rollout risk.
- **Retired-Source Containment API (§1085–1100):** Creating a single centralized classifier API to handle legacy paths prevents duplicate-active-definition crashes across intermediate and rollback states.

---

## Critical Risks & Gaps

### 1. Fallback Behavior for Omitted/Renamed Maintenance Workers
- **The Risk:** The design specifies that `dog` is merely configurable pack data and that "Go must continue to work when the Core maintenance worker is renamed or omitted" (§738–741). However, provider-pack formulas (such as `dolt`'s `mol-dog-backup` and related doctor checks) are highly dependent on having a configured worker to execute tasks.
- **The Gap:** If an operator defines a Core-only city and explicitly omits `core.maintenance_worker`, the design does not specify whether the SDK falls back to a safe, non-agent controller execution mode or crashes during formula compilation.
- **Recommendation:** Mandate that if `core.maintenance_worker` is omitted, the SDK falls back gracefully to standard controller-driven execution where applicable, or fails compiling the specific dependent formulas with a clear, localized config error rather than an opaque TOML parsing panic.

### 2. Guarding Against Malformed/Empty Dynamic Notification Recipients in Scripts
- **The Risk:** Generalized scripts like `reaper.sh` and `jsonl-export.sh` will consume recipients from formula or order metadata dynamically (§1377–1381).
- **The Gap:** The design states that "required recipient fields fail preflight if empty or `/`". However, for optional notification recipients, an empty or unconfigured value can lead to malformed CLI calls (e.g., executing `gc mail send ""` or `gc mail send /`) inside the shell scripts, causing unhandled script crashes.
- **Recommendation:** Mandate that all generalized shell scripts perform preflight validation of their recipient parameters. If the recipient variable is empty or resolves to `/`, the script must log a warning to `stderr` and skip mail execution (exiting with code `0`) rather than executing a malformed command.

### 3. Verification of Warning and Escalation Pathways
- **The Risk:** The behavior manifest and `test/packcompat` ensure that Gastown workflows trigger under standard conditions (§1412–1415).
- **The Gap:** Verifying only the happy path of a workflow (e.g., successful backup or wisp compaction) does not prove that error-handling, warnings, and escalation pathways (which represent the highest-risk operational logic) are preserved.
- **Recommendation:** Explicitly require that `test/packcompat` includes **behavioral-trigger fixtures** that simulate edge cases, such as mock failures, timeouts, and network disconnects, to force scripts and formulas down their warning and escalation paths.

---

## Evaluation of the Three Key Questions

### 1. Does every generalized Core asset have a corresponding external Gastown home for stripped role-specific behavior?
- **Auditor Finding:** **Yes.** The "Existing Asset Migration Map" (documented in requirements and mirrored in design) explicitly lists the destination and rationale for every asset. For example, Gastown-specific prompt files, `prune-branches.sh`, and workflow formulas (such as `mol-deacon-patrol` and `mol-witness-patrol`) are moved cleanly to `gascity-packs/gastown`, while generalized operational cleanup moves to Core.

### 2. Does the before-and-after inventory cover formulas, orders, scripts, prompts, template variables, and notification paths rather than only file moves?
- **Auditor Finding:** **Yes.** The "Source-Derived Behavior Manifest" (§88) specifically encompasses trigger conditions, requester actions, detector routines, route metadata, mail/nudge targets, prompt fragments, and script branches. This guarantees that side-effecting behaviors are tracked at a logical level rather than merely as file paths on disk.

### 3. What artifact proves supported Gastown workflows still resolve and trigger after the split?
- **Auditor Finding:** The canonical machine-readable **`plans/core-gastown-pack-migration/behavior-manifest.generated.yaml`** (and its companion public Gastown manifest `gastown/docs/behavior-manifest.generated.yaml`) represents the definitive, auditable proof artifact. Under §115–120, CI is configured to fail if any row is missing or lacks verified witnesses.

---

## Required Changes for Finalization

1. **Script Recipient Preflight Validation:** Amend §1379–1381 to mandate that generalized shell scripts explicitly check dynamic recipient variables and skip execution with code `0` on empty/slash targets.
2. **Define Maintenance Worker Omission Policy:** Detail the exact SDK behavior and error-handling flow when the operator configures a city with no `core.maintenance_worker`.
3. **Mandate Behavioral-Trigger Fixtures in `test/packcompat`:** Update §1412–1415 to require testing of simulated failure/escalation paths rather than just successful executions.
