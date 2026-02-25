# Implementation Plan

This plan defines an execution path from empty scaffold to a usable monitor.

Current status:
- core baseline is implemented (CLI, local/SSH modes, collectors, reconnect loop, and TUI).
- this plan remains the structured checklist for further hardening and enhancements.

## Phase 0: Project bootstrap
Goal:
- establish code layout and baseline tooling.

Tasks:
1. Initialize Go module and package layout:
   - `cmd/slurm-monitor/`
   - `internal/cli/`
   - `internal/transport/`
   - `internal/collectors/`
   - `internal/model/`
   - `internal/tui/`
2. Add minimal CLI entrypoint with version/help.
3. Add lint/test task stubs and CI-ready local commands.

Acceptance checks:
- `go test ./...` runs cleanly.
- `go run ./cmd/slurm-monitor --help` works.
- invalid flags print actionable errors and direct users to `--help`.

## Phase 1: Mode resolution and capability checks
Goal:
- enforce startup behavior and fail-fast checks.

Tasks:
1. Implement local/remote mode resolver from positional target.
2. Implement command capability checker (`sinfo`, `squeue`, `scontrol`) in selected transport context.
3. Implement actionable fatal error messages.

Acceptance checks:
- local mode exits with error if Slurm commands unavailable.
- remote mode exits with error if remote host lacks Slurm commands.
- transient startup SSH/network failures retry until recovery (or until `--duration` deadline when explicitly set).
- remote mode accepts SSH alias and `user@host`.

## Phase 2: Transport layer and reconnect controller
Goal:
- robust command execution in local and remote modes.

Tasks:
1. Implement `Transport` interface.
2. Implement `LocalTransport`.
3. Implement `SSHTransport` via system `ssh` with:
   - connect timeout
   - command timeout
   - custom config/identity/port support
4. Implement retry/backoff controller and connection state transitions.

Acceptance checks:
- transient SSH failures move state to reconnecting and recover automatically.
- tool keeps running with stale snapshot when disconnected.
- reconnect success returns state to connected without restart.

## Phase 3: Slurm collectors and parsers
Goal:
- produce typed snapshots for node summary, queue summary, and user views.

Tasks:
1. Implement node collector for required fields.
2. Implement queue summary collector (running/pending/other) with job-array task expansion.
3. Implement per-user queue collector.
4. Implement parser defensive behavior for missing optional utilization fields.

Acceptance checks:
- snapshots are produced on healthy cluster.
- queue and per-user counts match `squeue -r` verification for array-heavy workloads.
- missing optional metrics produce `n/a`, not crashes.
- parser failures are surfaced and loop continues.

## Phase 4: Domain model and aggregation
Goal:
- convert collector outputs into render-ready view model.

Tasks:
1. Implement model structs and derived metric calculators.
2. Implement totals row logic.
3. Implement freshness timestamp and staleness duration.

Acceptance checks:
- totals are mathematically correct for sample fixtures.
- freshness/staleness values update as expected.

## Phase 5: TUI foundation
Goal:
- deliver first full-screen live UI.

Tasks:
1. Implement event loop and screen lifecycle.
2. Add connection status header.
3. Add fixed vertical panel stack with two panels:
   - node summary (with aggregate totals),
   - combined queue panel (queue summary section + per-user section).
4. Add resize-aware layout switching.

Acceptance checks:
- UI fills terminal and restores terminal on exit.
- layout adapts after resize.
- snapshot updates are reflected without restart.

## Phase 6: Runtime hardening and UX polish
Goal:
- stabilize long-running operation and improve readability.

Tasks:
1. Add stale-data visual treatment and error banner area.
2. Add compact mode for narrow terminals.
3. Add optional text sparklines/history strips.
4. Add graceful shutdown behavior and signal handling.

Acceptance checks:
- long-running process remains responsive.
- stale/connected states are unambiguous.
- compact layout remains usable on small terminal sizes.

## Phase 7: Verification and battle testing (required)
Goal:
- prove resilience and correctness with realistic conditions.

### Functional matrix
- local mode with Slurm available
- local mode without Slurm
- remote mode to valid Slurm host
- remote mode to invalid/non-Slurm host
- remote mode via alias with ProxyJump/bastion path

### Resilience matrix
- temporary network drop then recovery
- repeated SSH timeout bursts
- command timeout without process crash
- terminal resize stress while reconnecting

### Data correctness checks
- node fields populated correctly against source commands
- queue summary counts match direct `squeue` checks
- per-user counts match grouped `squeue` outputs

### Performance checks
- poll cadence remains stable under degraded network
- UI input/quit remains responsive during reconnect loops

Exit criteria:
- all functional and resilience scenarios pass
- no crash in extended soak run
- known limitations documented in `README.md` and `docs/spec.md`

## Phase 8: Packaging and operator docs
Goal:
- make tool easy to run in real environments.

Tasks:
1. Add installation/build instructions.
2. Add usage examples for local, alias target, and `user@host`.
3. Document known limitations and troubleshooting.
4. Document read-only safety guarantee and non-mutating command allowlist.

Acceptance checks:
- fresh user can build and run from docs.
- troubleshooting covers common SSH/Slurm failures.

## Ongoing loops during implementation
- after each phase:
  - verify (`go test`, targeted manual checks)
  - promote durable findings to `docs/`
  - checkpoint commit when eligible
- keep `plan/current/` as disposable execution scratch.
