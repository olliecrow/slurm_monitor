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

Collapsed job-array rows undercount schedulable work. `squeue -r` expands tasks so queue and user job counts reflect task granularity.

CPU/GPU resource totals come from `tres-alloc`, which is the Slurm field for allocated or requested TRES. Some pending rows omit TRES detail, so the collector may use cached `scontrol show job` data for the affected root job. That fallback is limited to four probes per collection and one shared command-timeout budget so missing detail cannot create an unbounded poll.

Enforcement: the collector command includes `squeue -r` and `tres-alloc`; tests lock the command shape, fallback conditions, and probe limit.

References: `internal/slurm/collector.go`, `internal/slurm/collector_test.go`, `internal/slurm/parse.go`.

## Preserve source metric semantics

Node CPU load and free-memory values come directly from Slurm. They are not smoothed or interpolated because synthetic movement would misrepresent scheduler data.

GPU percentage is allocation saturation (`GPUAlloc/GPUTotal`), not live device activity. The UI labels it `gpu alloc%`.

Enforcement: parsers retain raw Slurm-derived values and availability flags; UI labels and tests distinguish allocation from utilization.

References: `internal/slurm/parse.go`, `internal/tui/model.go`, `docs/spec.md`.

## Keep the TUI focused, terminal-bounded, and non-interactive

The display has three data views: node summary, queue summary, and per-user summary. It uses two stacked panels so these views remain visible without navigation.

Panel-content budgets determine visible rows. Hidden-row metadata is explicit, and node alerts plus the aggregate `TOTAL` row take priority over per-node rows. This prevents large clusters or small terminals from causing wrapping, scrolling, or silent loss of critical health information.

The header distinguishes initial loading, connected operation, transient recovery, and permanent disconnection. A clock, refresh age, and status spinner show liveness even when cluster metrics are unchanged.

Enforcement: one budget-aware render path handles normal and compact layouts; viewport tests cover resizing, clipping metadata, alerts, totals, and footer placement.

References: `internal/tui/model.go`, `internal/tui/model_test.go`, `docs/spec.md`.

## Show CPU-job and GPU-job splits directly

Running and pending counts are separated into CPU-job and GPU-job columns. Explicit job wording prevents the counts from being mistaken for CPU or GPU resource totals.

Per-user ordering favors current GPU and CPU holders, then current job counts, pending GPU/CPU demand, pending job counts, pending memory, and username. This keeps active large holders visible when the terminal clips rows while retaining deterministic tie-breakers.

Held CPU/GPU totals remain in `--once` output but not in the TUI user table, where they would make the layout too wide.

Enforcement: the parser stores the four canonical job counts; aggregate totals are derived from them rather than stored separately.

References: `internal/slurm/types.go`, `internal/slurm/user_sort.go`, `internal/tui/model.go`, `internal/app/app.go`.

## Keep help and preflight behavior available in the CLI

Operators may need to understand behavior without opening repository documentation. `--help` therefore describes modes, flags, retry behavior, authentication, and examples.

`doctor` performs one non-mutating capability pass. `dry-run` prints the resolved execution plan without invoking local or remote commands. `completion` emits static Bash or Zsh completion text.

Enforcement: command parsing and output tests keep helper behavior aligned with the main CLI.

References: `cmd/slurm-monitor/main.go`, `internal/config/config.go`, `internal/app/preflight.go`, `docs/spec.md`.
