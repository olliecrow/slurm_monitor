# Requirement Alignment

This file maps stated requirements to planned behavior/docs and later test coverage.

## Requirement map

### R1: Local and remote monitoring modes
- Requirement:
  - run locally when no SSH target is provided
  - run remotely when a target is provided
- Planned enforcement:
  - mode resolver + startup capability checks
- References:
  - `docs/spec.md` (CLI Contract, Startup Behavior)
  - `docs/implementation-plan.md` (Phases 1-2)

### R2: Robust remote SSH monitoring under bad networks
- Requirement:
  - tolerate dropouts/timeouts and recover automatically
- Planned enforcement:
  - reconnect controller with bounded backoff/jitter and state machine
- References:
  - `docs/spec.md` (Remote Resilience Contract)
  - `docs/architecture.md` (Remote resilience model)
  - `docs/implementation-plan.md` (Phase 2, Phase 7)

### R3: Full-screen dynamic TUI
- Requirement:
  - full-window UI, resize-aware, live-updating
- Planned enforcement:
  - TUI runtime with resize events and decoupled render/poll loop
- References:
  - `docs/spec.md` (TUI Behavior)
  - `docs/architecture.md` (TUI runtime, rendering layout plan)
  - `docs/implementation-plan.md` (Phases 5-6)

### R4: Node-level metrics with aggregate totals
- Requirement:
  - node name/state/cpu/mem/gpu allocation+utilization/partition and totals row
- Planned enforcement:
  - node collector + aggregator + table renderer
- References:
  - `docs/spec.md` (Runtime Data Contract, Node monitoring class)
  - `docs/implementation-plan.md` (Phases 3-4)

### R5: Three data views in fixed layout
- Requirement:
  - node summary view with aggregate totals
  - queue summary view
  - pure per-user view
- Planned enforcement:
  - dedicated collectors and a fixed two-panel TUI where queue + user share a combined lower panel
- References:
  - `docs/spec.md` (Runtime Data Contract)
  - `docs/implementation-plan.md` (Phases 3 and 5)

### R12: Discoverable CLI help and argument guidance
- Requirement:
  - users can get complete usage/context from CLI help without reading source
  - invalid flags point users to help output
- Planned enforcement:
  - dedicated help text includes mode behavior, auth model, retry semantics, flags, and examples
  - parse errors include `--help` guidance
- References:
  - `docs/spec.md` (CLI Contract)
  - `internal/config/config.go`
  - `cmd/slurm-monitor/main.go`

### R11: Queue types and requested-resource visibility
- Requirement:
  - monitor not only job counts, but also job-state mix and requested resources by workload.
- Planned enforcement:
  - queue collector parses per-job state + requested CPU/memory/GPU with `squeue -r` so job arrays are counted at task granularity.
  - queue summary + user section expose running/pending/other counts plus per-user pending CPU-job/GPU-job split.
- References:
  - `docs/spec.md` (Queue summary view fields)
  - `internal/slurm/parse.go`
  - `internal/slurm/collector.go`
  - `internal/tui/model.go`

### R6: Wrong-target and missing-Slurm fail-fast behavior
- Requirement:
  - local mode without Slurm should error
  - remote target without Slurm should error
- Planned enforcement:
  - startup capability checks and explicit fatal errors
- References:
  - `docs/spec.md` (Startup Behavior)
  - `docs/decisions.md` (fail-fast startup decision)
  - `docs/implementation-plan.md` (Phase 1)

### R7: SSH aliases and bastion/proxy support
- Requirement:
  - support SSH config alias and relay through bastion where configured
- Planned enforcement:
  - system `ssh` transport honoring existing SSH config and ProxyJump
- References:
  - `docs/spec.md` (CLI Contract, Remote Resilience Contract)
  - `docs/architecture.md` (Transport abstraction)
  - `docs/decisions.md` (system `ssh` decision)

### R8: Credentials safety and open-source posture
- Requirement:
  - never commit or expose secrets/credentials
- Planned enforcement:
  - security policy + no password CLI flags + redaction guidance
- References:
  - `docs/security.md`
  - `AGENTS.md` (Open-Source Transition Posture)
  - `docs/decisions.md` (read-only and transport decisions)

### R10: Read-only monitoring only
- Requirement:
  - never submit mutating operations to Slurm
- Planned enforcement:
  - read-only command allowlist and no queue action controls in the UI
- References:
  - `docs/spec.md` (Safety Constraint)
  - `docs/security.md` (Runtime safety posture)
  - `docs/decisions.md` (read-only decision)

### R9: macOS primary, Linux support
- Requirement:
  - works on macOS and Linux
- Planned enforcement:
  - Go implementation and cross-platform command/path assumptions
- References:
  - `docs/spec.md` (Platform Support)
  - `docs/implementation-plan.md` (Phase 0 acceptance checks + Phase 7 matrix)
