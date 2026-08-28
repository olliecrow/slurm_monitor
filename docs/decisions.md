# Durable decisions

This file records cross-cutting rationale that is not clearer in code, tests, the specification, or the architecture document.

## Use Go on macOS and Linux

The project needs a responsive terminal application, direct process control, and single-file binaries. Go provides the CLI and TUI libraries, concurrency, and build output without another runtime.

Windows is unsupported. Tests cover parser and process boundaries that Go's type system cannot enforce.

Enforcement: keep runtime implementation in Go and preserve the macOS/Linux platform contract.

References: `go.mod`, `docs/spec.md`, `docs/architecture.md`.

## Use system OpenSSH and POSIX `sh`

Remote mode must honor existing host aliases, ProxyJump routes, host-key checks, agents, keys, and SSH configuration. The system `ssh` client already implements those contracts; an embedded SSH stack would duplicate them and risk behaving differently from an operator's normal connection.

Local and remote commands use `sh -lc`. The command strings are simple and POSIX-compatible, so requiring Bash would add an unnecessary dependency.

No password flag is provided. Authentication stays in standard OpenSSH configuration, agents, and keys.

Enforcement: local and SSH transports invoke `sh -lc`; SSH options preserve standard configuration while adding timeouts, keepalives, and connection reuse.

References: `internal/transport/`, `internal/config/config.go`, `docs/security.md`.

## Keep all Slurm access read-only

The product monitors a cluster. It does not control one. Queue mutation would require new authentication, confirmation, and recovery rules and would raise the cost of an operator mistake.

Enforcement: runtime commands are limited to `squeue` and read-only `scontrol` queries. The CLI and TUI expose no cancel, requeue, hold, release, or submit actions.

References: `internal/slurm/collector.go`, `docs/spec.md`, `docs/security.md`.

## Fail fast only when retry cannot help

Invalid arguments, missing capabilities, permanent SSH/auth/configuration failures, shell-contract failures, and parser-contract failures require operator or code changes. Retrying them indefinitely hides the real problem.

The monitor retries transient transport failures. During recovery, it keeps the last good snapshot visible and marks it stale. Startup and runtime retries continue until the operator quits or `--duration` expires.

Enforcement: typed missing-command errors and transport classification decide whether to retry. Retryable failures use bounded exponential backoff. Permanent runtime failures leave the TUI disconnected until exit.

References: `internal/app/app.go`, `internal/monitor/monitor.go`, `internal/transport/transport.go`, `docs/architecture.md`.

## Count array tasks and preserve Slurm resource semantics

Collapsed job-array rows undercount schedulable work. `squeue -r` expands tasks so queue, partition, and user job counts reflect task granularity.

CPU/GPU resource totals come from `tres-alloc`, which is the Slurm field for allocated or requested TRES. Some pending rows omit TRES detail, so the collector may use cached `scontrol show job` data for the affected root job. That fallback is limited to four probes per collection and one shared command-timeout budget so missing detail cannot create an unbounded poll.

Current availability comes from one node query, not queue subtraction. Nodes are deduplicated before aggregation. CPU availability uses `CPUEfctv` when present and otherwise `CPUTot`, less `CPUAlloc`. GPU availability uses configured TRES less allocated TRES, with node GRES fields as a compatibility fallback. Only `IDLE` and `MIXED` nodes without blocking state flags contribute. This prevents down, drained, reserved, powered-down, and other unavailable capacity from appearing free. These totals describe cluster-wide schedulable capacity, but placement constraints can still prevent a specific job from using it.

Enforcement: the collector command includes read-only node data, `squeue -r`, and `tres-alloc`; tests lock the command shape, node-state and resource parsing, fallback conditions, and probe limit.

References: `internal/slurm/collector.go`, `internal/slurm/collector_test.go`, `internal/slurm/parse.go`.

## Keep the TUI non-interactive and within the terminal viewport

The display has queue summary, partition, user, and pending-reason sections. A frame encloses only the dashboard content. Unused height stays blank above the pinned footer. Active detail sections take turns receiving each available row.

The interactive display omits individual jobs because those rows churn on busy clusters. Partition, user, and pending-reason totals show scheduler pressure. The `--once` report keeps grouped root jobs for detailed diagnostics. Partitions sort by pending pressure before current load. Pending reasons sort by GPU demand, CPU demand, then task count.

The queue summary states total, running, pending, and non-zero other job counts plus current CPU/GPU availability. On wide terminals with enough height, a bold `All partitions` row supplies detailed queue totals in the same grid as partition data. Compact terminals and short wide terminals use a separate resource summary with in-use, requested, and free values because their detail rows show job counts only. CPU-core totals cover both CPU-only and GPU jobs.

The TUI translates stable common Slurm pending-reason codes into plain language and preserves unknown reason text. Long prose reasons clip at a word boundary when space permits, without changing fixed table-label clipping. The `--once` report keeps raw reason values for diagnostics and downstream parsing. Compact pending-reason details use singular task and resource labels only for one.

Wide partition and user tables use one shared numeric grid. Two header rows name CPU-only jobs, GPU jobs, CPU cores, and GPUs, then state `Running` and `Pending` for jobs and `In use` and `Requested` for resources. Names align left; numbers align right; state words do not repeat in data cells. Paired columns use stable widths through normal count growth. Pending-reason totals use the same right edge. Compact rows keep aligned CPU-only and GPU `running / pending` pairs and shorten a long name before complete metrics.

Dashboard-height budgets determine visible rows. Connected horizontal rules separate sections when every active detail can still show data. Tight terminals use that row for data instead. At 72x20, every active section can show at least one data row. Common queues show several. Hidden-row metadata uses `X shown · N hidden`, and smaller terminals keep every active section title when space permits. The display does not wrap or scroll. Each rendered section reports its hidden-row count. A terminal too small to render every section can omit later sections.

The header distinguishes initial loading, connected operation, transient recovery, and permanent disconnection. A clock, update age, and status spinner show liveness even when cluster metrics are unchanged. Narrow headers remove the source and then the clock as complete fields when needed, rather than showing partial values.

Enforcement: one render path handles normal and compact layouts. Sort functions use fixed tie-breakers. Viewport tests cover resizing, round-robin detail row allocation, shown and hidden row counts, and footer placement.

References: `internal/slurm/summary_sort.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `docs/spec.md`.

## Show CPU-job and GPU-job splits directly

Running and pending counts are grouped into distinct CPU-only-job and GPU-job columns. CPU-core and GPU resource columns remain separate because GPU jobs also use CPU cores. Two-level headers prevent job counts from being mistaken for resource totals without repeating prose in every row.

Per-user ordering sorts by running GPU and CPU allocations, running job counts, pending GPU and CPU demand, pending job counts, pending memory, and username. Users with the largest running allocations remain visible when the terminal clips rows. The remaining fields give fixed tie-breakers.

Running and pending CPU/GPU totals appear in expanded partition and user tables. Compact partition and user rows retain all four job counts. `--once` prints all counts and resource totals.

Enforcement: the parser stores the four canonical job counts; aggregate totals are derived from them rather than stored separately.

References: `internal/slurm/types.go`, `internal/slurm/user_sort.go`, `internal/tui/model.go`, `internal/app/app.go`.

## Keep help and preflight behavior available in the CLI

Operators may need to understand behavior without opening repository documentation. `--help` therefore describes modes, flags, retry behavior, authentication, and examples.

`doctor` performs one non-mutating capability pass. `dry-run` prints the resolved execution plan without invoking local or remote commands. `completion` emits static Bash or Zsh completion text.

Enforcement: command parsing and output tests keep helper behavior aligned with the main CLI.

References: `cmd/slurm-monitor/main.go`, `internal/config/config.go`, `internal/app/preflight.go`, `docs/spec.md`.
