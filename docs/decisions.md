# Durable decisions

This file records cross-cutting rationale that is not clearer in code, tests, the specification, or the architecture document.

## Use Go on macOS and Linux

The project needs a responsive terminal application, straightforward process orchestration, and easily distributed binaries. Go provides suitable CLI/TUI libraries and simple concurrency without adding a more complex runtime or build toolchain.

Windows is intentionally unsupported. Parser and process boundaries remain covered by tests because Go provides fewer type-level guarantees than some alternatives.

Enforcement: keep runtime implementation in Go and preserve the macOS/Linux platform contract.

References: `go.mod`, `docs/spec.md`, `docs/architecture.md`.

## Use system OpenSSH and POSIX `sh`

Remote mode must honor existing host aliases, ProxyJump routes, host-key checks, agents, keys, and SSH configuration. The system `ssh` client already implements those contracts; an embedded SSH stack would duplicate them and risk behaving differently from an operator's normal connection.

Local and remote commands use `sh -lc`. The command strings are simple and POSIX-compatible, so requiring Bash would add an unnecessary dependency.

No password flag is provided. Authentication stays in standard OpenSSH configuration, agents, and keys.

Enforcement: local and SSH transports invoke `sh -lc`; SSH options preserve standard configuration while adding timeouts, keepalives, and connection reuse.

References: `internal/transport/`, `internal/config/config.go`, `docs/security.md`.

## Keep all Slurm access read-only

The product is an observability surface, not a cluster control plane. Queue mutation would increase operational risk and expand the authentication, confirmation, and recovery model.

Enforcement: runtime commands are limited to `squeue` and read-only `scontrol` queries. The CLI and TUI expose no cancel, requeue, hold, release, or submit actions.

References: `internal/slurm/collector.go`, `docs/spec.md`, `docs/security.md`.

## Fail fast only when retry cannot help

Invalid arguments, missing capabilities, permanent SSH/auth/configuration failures, shell-contract failures, and parser-contract failures require operator or code changes. Retrying them indefinitely hides the real problem.

Transient transport failures should recover automatically. During recovery, the last good snapshot remains visible with explicit stale and retry state. Startup and runtime retries are unbounded unless the operator quits or sets `--duration`.

Enforcement: typed missing-command errors and transport retry classification gate bounded exponential backoff; permanent runtime failures leave the TUI disconnected until exit.

References: `internal/app/app.go`, `internal/monitor/monitor.go`, `internal/transport/transport.go`, `docs/architecture.md`.

## Count array tasks and use Slurm TRES as the resource source

Collapsed job-array rows undercount schedulable work. `squeue -r` expands tasks so queue, partition, and user job counts reflect task granularity.

CPU/GPU resource totals come from `tres-alloc`, which is the Slurm field for allocated or requested TRES. Some pending rows omit TRES detail, so the collector may use cached `scontrol show job` data for the affected root job. That fallback is limited to four probes per collection and one shared command-timeout budget so missing detail cannot create an unbounded poll.

Enforcement: the collector command includes `squeue -r` and `tres-alloc`; tests lock the command shape, fallback conditions, and probe limit.

References: `internal/slurm/collector.go`, `internal/slurm/collector_test.go`, `internal/slurm/parse.go`.

## Keep the TUI focused, terminal-bounded, and non-interactive

The display has scheduler overview, partition, user, pending-reason, and job sections. It uses one full-height panel so scheduler insights remain visible without navigation. The panel shares its remaining height fairly across all active detail sections.

The job view groups array tasks only when root job, user, partition, state, and pending reason match. This keeps large arrays readable without changing task-granular queue totals. Partition ordering surfaces pending pressure before current load. Pending-reason ordering surfaces the largest GPU and CPU demand. Job ordering surfaces pending GPU, running GPU, pending CPU, and running CPU work, then favors larger grouped jobs.

Wide partition and user tables group running jobs, pending jobs, resources in use, and requested resources under plain-language labels instead of abbreviated metric names. Wide pending-reason and job tables give spare width to reason text. Compact tables keep full state names and use shorter plain labels only where width requires them.

Panel-content budgets determine visible rows. The compact five-line scheduler table leaves enough space for every active detail section to show at least one data row at 72x20. Hidden-row metadata uses `showing X of Y; N hidden`, and smaller terminals keep every active detail section title when space permits. This prevents large queues or small terminals from causing wrapping, scrolling, or silent loss of scheduler context.

The header distinguishes initial loading, connected operation, transient recovery, and permanent disconnection. A clock, refresh age, and status spinner show liveness even when cluster metrics are unchanged. Narrow headers remove the source and then the clock as complete fields when needed, rather than showing partial values.

Enforcement: one budget-aware render path handles normal and compact layouts; summary sorting is deterministic; viewport tests cover resizing, fair detail-section budgets, clipping metadata, and footer placement.

References: `internal/slurm/summary_sort.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `docs/spec.md`.

## Show CPU-job and GPU-job splits directly

Running and pending counts are separated into CPU-job and GPU-job columns. Explicit job wording prevents the counts from being mistaken for CPU or GPU resource totals.

Per-user ordering favors current GPU and CPU holders, then current job counts, pending GPU/CPU demand, pending job counts, pending memory, and username. This keeps active large holders visible when the terminal clips rows while retaining deterministic tie-breakers.

Running and pending CPU/GPU totals appear in expanded partition and user tables. Compact tables retain the four job-count columns. `--once` prints all counts and resource totals.

Enforcement: the parser stores the four canonical job counts; aggregate totals are derived from them rather than stored separately.

References: `internal/slurm/types.go`, `internal/slurm/user_sort.go`, `internal/tui/model.go`, `internal/app/app.go`.

## Keep help and preflight behavior available in the CLI

Operators may need to understand behavior without opening repository documentation. `--help` therefore describes modes, flags, retry behavior, authentication, and examples.

`doctor` performs one non-mutating capability pass. `dry-run` prints the resolved execution plan without invoking local or remote commands. `completion` emits static Bash or Zsh completion text.

Enforcement: command parsing and output tests keep helper behavior aligned with the main CLI.

References: `cmd/slurm-monitor/main.go`, `internal/config/config.go`, `internal/app/preflight.go`, `docs/spec.md`.
