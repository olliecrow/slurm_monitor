# Decision Capture Policy

This document defines how to record fixes and important decisions so future work does not re-litigate the same questions. It is written to stay accurate over time; avoid time-specific language.

## When to record
- Any fix for a confirmed bug, regression, or safety issue.
- Any deliberate behavior choice that differs from intuitive defaults.
- Any trade-off decision that affects modeling or behavior.
- Any change that affects external behavior, invariants, or public APIs.

## Where to record
Use the smallest, most local place that makes the decision obvious:
- **Code comments** near the behavior when the rationale is not obvious.
- **Tests** with names/assertions that encode the invariant.
- **Docs** (this file or another focused doc) when the decision is cross-cutting.

Prefer updating an existing note over creating a new file.

## What to record
Keep entries short and focused:
- **Decision**: what was chosen.
- **Context**: what problem or risk it addresses.
- **Rationale**: why this choice was made.
- **Trade-offs**: what we are not doing.
- **Enforcement**: which tests or code paths lock it in.
- **References** (optional): file paths, tests, or PRs that embody the decision.

## Template
```
Decision:
Context:
Rationale:
Trade-offs:
Enforcement:
References:
```

## Decision Records

Decision:
Use Go as the implementation language.
Context:
The project needs a fast cross-platform terminal app (macOS primary, Linux support), stable static binaries, and robust process/SSH orchestration.
Rationale:
Go offers a strong CLI/TUI ecosystem (`cobra`, `bubbletea`), good operational ergonomics, and straightforward subprocess/network concurrency.
Trade-offs:
Less strict type-level guarantees than Rust; parser/data modeling discipline must come from tests and code review.
Enforcement:
Initialize Go module and keep implementation in Go packages.
References:
`docs/spec.md`, `docs/architecture.md`, `docs/implementation-plan.md`.

Decision:
Use the system `ssh` client for remote transport instead of implementing SSH protocol directly in-process.
Context:
The tool must support aliases, ProxyJump/bastions, user@host shorthand, and existing SSH config/auth flows.
Rationale:
System `ssh` already handles host key checking, config resolution, jump hosts, key agents, and auth prompts reliably across environments.
Trade-offs:
Depends on external binary behavior and command-line invocation discipline; requires careful stderr parsing and timeout handling.
Enforcement:
Remote transport layer shells out to `ssh` with explicit options/timeouts and reconnection policy.
References:
`docs/spec.md` (remote mode), `docs/architecture.md` (transport manager).

Decision:
Startup is fail-fast only for non-recoverable capability/argument failures; transient transport failures retry automatically.
Context:
User wants explicit errors for wrong host selection (or local mode without Slurm) rather than silent degraded behavior.
Rationale:
Fail-fast for missing Slurm tools reduces false confidence and avoids rendering stale/empty data as if monitoring were active. Retrying transient transport failures improves resilience on unreliable networks.
Trade-offs:
Partially configured hosts still fail immediately; transient startup transport failures can delay startup for a long time unless operator quits or sets `--duration`.
Enforcement:
Startup capability checks for required commands; immediate process exit for missing-command/argument failures; retry loop for transient startup transport failures.
References:
`docs/spec.md` (startup checks), `docs/implementation-plan.md` (phase 1 acceptance checks).

Decision:
The monitor remains read-only and does not mutate Slurm state.
Context:
The requested scope is monitoring reliability and visibility; action controls increase risk and complexity.
Rationale:
Read-only design reduces blast radius and keeps operator trust high.
Trade-offs:
No in-TUI cancel/requeue/hold actions.
Enforcement:
Collector command allowlist includes read-only Slurm commands only (`sinfo`, `squeue`, `scontrol` reads).
References:
`docs/spec.md` (non-goals), `docs/security.md` (safety posture).

Decision:
The TUI exposes three primary data views: node summary, queue summary, and per-user view.
Context:
The monitoring objective requires high-level cluster state plus clear per-user queue attribution without requiring job mutation controls.
Rationale:
Keeping these three views visible keeps the display focused and readable while covering operator needs.
Trade-offs:
Job-level drill-down is not part of the default panel set.
Enforcement:
TUI layout includes a node panel plus a combined queue panel containing queue and user sections, with corresponding collectors/aggregators.
References:
`docs/spec.md` (Runtime Data Contract), `docs/implementation-plan.md` (Phase 3 and Phase 5).

Decision:
SSH authentication follows standard SSH mechanisms and excludes password CLI flags.
Context:
The tool must support aliases, `user@host`, and bastion paths while preserving secret safety.
Rationale:
Using existing SSH config/agent/key flows preserves compatibility and avoids credential leakage risks.
Trade-offs:
Users without configured SSH auth must configure SSH externally instead of passing passwords to the tool.
Enforcement:
CLI target handling accepts alias or `user@host`; authentication is delegated to OpenSSH config/agent/keys only.
References:
`docs/spec.md` (CLI Contract, Remote Resilience Contract), `docs/security.md` (SSH/auth policy).

Decision:
Queue and per-user counts use Slurm job-array task granularity.
Context:
Collapsed array rows in default `squeue` output under-count pending/running jobs and pending CPU/GPU demand for array-heavy workloads.
Rationale:
Using `squeue -r` expands array tasks so each task is counted as one job and pending demand reflects the real schedulable workload.
Trade-offs:
Array-expanded output is larger and can slightly increase parse/load costs.
Enforcement:
Collector command uses `squeue -h -r`; parsing/aggregation sums per-line task demand; regression test guards presence of `-r`.
References:
`internal/slurm/collector.go`, `internal/slurm/parse.go`, `internal/slurm/collector_test.go`, `docs/spec.md` (Runtime Data Contract).

Decision:
Node CPU/memory utilization reflects raw Slurm node metrics without synthetic smoothing/interpolation.
Context:
On some clusters these fields refresh slowly, making percentages appear static over short windows even while polling is healthy.
Rationale:
Displaying source-of-truth values avoids inventing activity that Slurm does not report and keeps monitor semantics transparent.
Trade-offs:
Short-term visual movement can be low during stable periods; users may perceive slower change despite active refresh.
Enforcement:
UI refresh indicators (heartbeat clock, last-update age, status spinner) show liveness independently of metric movement; node utilization comes directly from parsed Slurm node fields.
References:
`internal/slurm/parse.go`, `internal/tui/model.go`, `docs/architecture.md` (Optional metrics), `docs/spec.md` (TUI Behavior).

Decision:
CLI help is first-class and self-contained.
Context:
Operators often launch the tool from terminals where docs may not be open; usage and failure guidance must be discoverable in-command.
Rationale:
Rich `--help` output and parse-error hints reduce onboarding friction and prevent avoidable misconfiguration loops.
Trade-offs:
Help text requires maintenance when flags/behavior change.
Enforcement:
Argument parser exposes a dedicated help path; main handles help with zero-exit output; parse errors include a direct `--help` hint.
References:
`internal/config/config.go`, `internal/config/config_test.go`, `cmd/slurm-monitor/main.go`, `docs/spec.md` (CLI Contract).

Decision:
Add explicit `doctor` and `dry-run` commands alongside monitor mode.
Context:
Operators need a fast way to validate setup and understand execution flow before starting a long live monitor session.
Rationale:
Dedicated helper commands keep onboarding simple, improve troubleshooting, and avoid accidental long-running sessions when users only need validation or preview.
Trade-offs:
CLI surface area is slightly larger and requires docs/tests to stay aligned.
Enforcement:
Argument parsing supports `doctor` and `dry-run`; `doctor` reports pass/fail from non-mutating preflight checks; `dry-run` prints planned stages and executes no transport commands.
References:
`internal/config/config.go`, `internal/app/preflight.go`, `internal/app/preflight_test.go`, `docs/spec.md` (Helper Command Behavior).

Decision:
Preserve composite node state qualifiers and prioritize node-summary health alerts for `DOWN`/`DRAIN`.
Context:
Collapsing node states to base values (for example showing `MIXED` instead of `MIXED+DRAIN`) hid scheduler-critical conditions and could mislead operators when jobs were blocked by maintenance/drain status.
Rationale:
Displaying full composite state makes scheduler constraints explicit, and placing a red health alert in the node summary keeps the warning next to node evidence while keeping the header focused on liveness.
Trade-offs:
Composite states consume more horizontal space; wide-mode state column was widened to reduce truncation. Header omits duplicate alerts by design.
Enforcement:
Node-state parsing preserves `+DRAIN`/`+DOWN` qualifiers (only cosmetic `*` is stripped). Node summary renders a `node alert` line when any nodes are `DOWN` or `DRAIN`. Header does not render node alert badges.
References:
`internal/slurm/parse.go`, `internal/slurm/parse_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `docs/spec.md`, `docs/architecture.md`.

Decision:
TUI mode is read-only and non-interactive by design.
Context:
The monitor is intended as an observability surface, not an in-terminal control plane.
Rationale:
Keeping TUI read-only and non-interactive reduces operator risk and keeps the interface simple.
Trade-offs:
No action controls from the TUI.
Enforcement:
- TUI does not expose mutating commands.
- Bottom status line shows `Ctrl+C to exit` as the only interaction hint.

Decision:
Constrain TUI row density and frame width to terminal-aware bounds, and surface explicit hidden-row indicators for large node/user tables.
Context:
Large clusters (many nodes/users) can produce oversized tables that risk terminal wrapping/scroll artifacts and unstable rendering in full-screen TUIs.
Rationale:
Deterministic row caps plus explicit `top X/Y, +N hidden` labels keep the display stable across different cluster sizes and terminal dimensions while preserving operator context.
Trade-offs:
Not all rows are shown at once in constrained terminals; operators must widen/resize terminal to view more rows.
Enforcement:
- Node/user row rendering applies terminal-aware upper bounds.
- Section titles include hidden-row metadata when clipping occurs.
- Node summary always retains alert + `TOTAL` lines.
- If final viewport clipping still occurs, last visible row is replaced with `... output clipped to terminal height ...`.
- Tests cover capped node/user labels and stable frame width behavior.
References:
`internal/tui/model.go`, `internal/tui/model_test.go`, `docs/spec.md`.

Decision:
Apply per-panel line budgeting so node alert and `TOTAL` remain visible even in tight terminal layouts.
Context:
Global height caps alone can still clip required node-summary lines when panel borders/padding and long labels consume extra rows.
Rationale:
Budgeting from panel content height keeps rendering deterministic across varying terminal sizes while preserving critical operator signals.
Trade-offs:
Very small terminals may show zero per-node rows while still showing section title, alert, and aggregate totals.
Enforcement:
- Node panel computes mandatory-line budget (`title`, optional `node alert`, `TOTAL`) before allocating per-node rows.
- Queue panel computes remaining line budget for user rows and always shows hidden-count metadata when rows are dropped.
- When panel budget allows zero visible rows, user-view header uses hidden-only labeling (for example `+N hidden`) instead of `top 0/N`.
- Generated table lines are width-fitted to panel content width to prevent wrap-driven vertical drift.
- Tests cover tight-height behavior for node alert + `TOTAL` preservation and hidden-user indicators.
References:
`internal/tui/model.go`, `internal/tui/model_test.go`, `docs/spec.md`.

Decision:
Add a top-level completion command and include it in canonical help text.
Context:
Operators repeatedly run `slurm-monitor` in shells where tab completion and direct in-command guidance reduce setup friction.
Rationale:
`slurm-monitor completion [bash|zsh]` provides practical tab-completion installation without changing monitor/doctor/dry-run behavior.
Trade-offs:
Completion scripts are static and require updates if command/flag surfaces change.
Enforcement:
- `cmd/slurm-monitor/main.go` intercepts `completion` and prints shell script output or completion help.
- `internal/config.HelpText()` documents completion usage and examples.
- Tests cover completion script generation and help-text inclusion.
References:
`cmd/slurm-monitor/main.go`, `cmd/slurm-monitor/main_test.go`, `internal/config/config.go`, `internal/config/config_test.go`, `README.md`
