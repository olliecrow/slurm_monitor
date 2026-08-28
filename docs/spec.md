# Product specification

## Goal

Build a CLI/TUI monitor for Slurm that can run:
- locally on a host with Slurm CLI access, or
- remotely over SSH, with automatic recovery from transient network and SSH failures.

The tool runs until the operator stops it or its configured duration expires. It shows scheduler activity and queue pressure without changing Slurm state.

## Scope

### In scope
- Full-screen terminal UI that adapts to terminal resize.
- Live polling and rendering loop with bounded CPU overhead.
- Local mode (default when no SSH target is provided).
- Remote mode via SSH target (alias from SSH config or `user@host` style target).
- Recovery behavior for transient SSH/network failures.
- Four primary data views:
  - scheduler summary view (cluster-level job counts, CPU/GPU resource totals, and current availability)
  - partition view (per-partition job counts and CPU/GPU resource totals)
  - user view (per-user job counts and CPU/GPU resource totals)
  - pending-reason view (scheduler reasons with affected task, CPU, and GPU demand)
- Queue labels must identify job or array-task counts and distinguish them from CPU and GPU resource totals.
- Connectivity state and retry timing in the UI.

### Out of scope
- Mutating Slurm state (cancel/requeue/hold/release).
- Embedding credentials in files or CLI history.
- Replacing OpenSSH with a custom SSH stack.

## CLI contract

### Invocation
- `slurm-monitor`
  - local mode; requires Slurm CLI available locally.
- `slurm-monitor <ssh-target>`
  - remote mode; `<ssh-target>` supports SSH config alias or `user@host`.
- `slurm-monitor doctor [<ssh-target>]`
  - runs non-mutating preflight checks and exits with pass/fail status.
- `slurm-monitor dry-run [<ssh-target>]`
  - prints planned execution order and exits without running commands.
- `slurm-monitor completion [bash|zsh]`
  - prints shell completion script output and exits.
- `slurm-monitor --help` (or `-h`)
  - prints a self-contained usage guide with mode behavior, retry semantics, auth model, flags, and examples.

### Argument errors
- Invalid argument combinations or unknown flags must state the violated argument rule.
- Parse errors must direct users to `slurm-monitor --help`.

### Main flags
- `--refresh <duration>`: poll interval (default `2s`).
- `--connect-timeout <duration>`: SSH command connect timeout.
- `--command-timeout <duration>`: per poll command timeout.
- `--ssh-config <path>`: optional custom SSH config file.
- `--identity-file <path>`: optional SSH identity file.
- `--port <int>`: optional SSH port override.
- `--no-color`: disable colored UI output.
- `--compact`: compact layout for small terminal dimensions.
- `--once`: collect one snapshot and print a text summary with queue job and resource totals, current CPU/GPU availability, and top partition, user, pending-reason, and grouped-job rows.
- `--duration <duration>`: optional clean auto-exit timer for TUI runs, including while transient startup preflight retries are active.

## Startup behavior

### Mode selection
- If no target is provided, run local checks and start local mode.
- If target is provided, run remote checks and start remote mode.

### Capability checks (must pass before entering TUI loop)
- Required commands: `squeue`, `scontrol`.
- Command execution uses POSIX `sh -lc` in both local and remote modes.
- Local mode:
  - if `sh` or a required Slurm command is missing locally, name the missing command and exit.
- Remote mode:
  - run capability check remotely via SSH through `sh -lc`.
  - if the remote shell contract fails or a required Slurm command is missing, identify the failed capability and exit.

### Failure semantics
- Non-recoverable startup failures are fatal:
  - missing required Slurm commands in selected context
  - invalid CLI argument combinations
  - permanent SSH/auth/configuration/shell-contract failures
- Transient startup failures (SSH/network/timeouts) are retried automatically with backoff.
- Retry behavior is unbounded by default and continues until operator quit; when `--duration` is set, retries stop at the configured deadline.
- Runtime poll failures keep the last good snapshot visible.
  - transient transport failures continue retrying with staleness and retry markers.
  - permanent transport/parser-contract failures stop retrying and leave the UI disconnected until operator quit.

## Helper command behavior

### `doctor`
- Runs one preflight pass and exits.
- Never enters the TUI loop.
- Checks required local tooling and selected-mode Slurm capability.
- Exits non-zero when any check fails.

### `dry-run`
- Prints resolved mode, target, runtime options, and planned stage order.
- Does not execute local or remote Slurm commands.
- Always remains read-only and exits after printing the plan.

### `completion`
- Prints shell completion script text for `bash` or `zsh`.
- Does not execute local or remote Slurm commands.
- Exits non-zero on unsupported shells or invalid argument counts.

## Runtime data contract

### 1) Scheduler summary view
Fields:
- running CPU-job count
- running GPU-job count
- pending CPU-job count
- pending GPU-job count
- other jobs count
- running CPU and GPU resource totals
- pending CPU and GPU resource demand
- currently available CPU cores and GPUs
- queue totals appear in the aggregate partition grid when the full wide partition grid fits, and in a separate resource summary on compact or short wide terminals
- CPU-core totals include CPU cores assigned to both CPU-only and GPU jobs
- counts include Slurm job arrays at array-task granularity (each array task counts as one job).
- available resources are unallocated CPU and GPU capacity on deduplicated nodes in schedulable `IDLE` or `MIXED` states without blocking flags
- available CPU uses Slurm's effective CPU total when present; available GPU uses configured and allocated node TRES, with node GRES fields as a fallback
- availability is cluster-level capacity, not a guarantee that a specific job can start; partition, reservation, topology, feature, memory, and policy constraints can still prevent placement.

### 2) User view
Per-user fields:
- user
- running CPU-job count
- running GPU-job count
- pending CPU-job count
- pending GPU-job count
- running CPU and GPU resource totals
- pending CPU and GPU resource demand
- running CPU-job count + running GPU-job count equals running count for each user.
- pending CPU-job count + pending GPU-job count equals pending count for each user.
- job-type counts include Slurm job arrays at array-task granularity.
- default user ordering puts the largest running GPU and CPU allocations first, then uses running job counts and pending demand as tie-breakers.

### 3) Partition view
Per-partition fields:
- partition name
- running CPU-job count
- running GPU-job count
- pending CPU-job count
- pending GPU-job count
- counts and resource totals use the same task-granular rules as the scheduler summary.
- default ordering sorts by pending GPU pressure, pending CPU pressure, then current GPU and CPU load.

### 4) Pending-reason view
Per-reason fields:
- pending reason reported by Slurm
- affected array-task count
- total pending CPU demand
- total pending GPU demand
- blank or unavailable reasons appear as `<unknown>`.
- default ordering sorts by GPU demand, CPU demand, then affected task count.

### Grouped job details (`--once` only)
Per grouped-job fields:
- root job ID
- user
- partition
- state
- pending reason (blank for non-pending work)
- matching array-task count
- total requested or allocated CPU and GPU resources across those tasks
- array tasks are grouped only when root job, user, partition, state, and pending reason match.
- default ordering sorts pending GPU work, running GPU work, pending CPU work, then running CPU work; larger grouped jobs sort first within each class.

## TUI behavior
- Full-screen layout.
- Dynamic resize handling for width/height changes.
- Live updates without requiring restart.
- Non-interactive display: no in-app controls or navigation; monitor-only rendering.
- Header includes a heartbeat clock and update age.
- Header includes a status spinner so refresh/liveness is visible even when metrics are stable.
- When the header is narrow, it removes lower-priority fields as complete units instead of cutting labels or values mid-word.
- Body renders one content-height framed dashboard with queue, partition, user, and pending-reason sections. The frame uses balanced horizontal padding, and unused terminal height stays blank outside it above the pinned footer.
- The queue summary states total, running, pending, and non-zero other job counts. It always shows currently available CPU cores and GPUs. Wide terminals put detailed queue totals in a bold `All partitions` row when the full grid fits vertically. Compact terminals and short wide terminals combine in-use, requested, and free values in a complete CPU-core and GPU resource summary because their detail rows show job counts only.
- Wide partition and user tables use distinct CPU-only job, GPU job, CPU-core, and GPU groups. Two header rows state `Running` and `Pending` once for job counts and `In use` and `Requested` once for resources. Data rows contain right-aligned numbers only.
- CPU-core totals include cores assigned to CPU-only and GPU jobs.
- Wide pending-reason tables align their numeric right edge with the aggregate grids and give remaining width to reason text before truncating it.
- The TUI replaces stable common Slurm pending-reason codes with plain-language labels and preserves unknown reason text. Long prose reasons clip at a word boundary when space permits. The `--once` report keeps raw Slurm reason values for diagnostics.
- Wide tables use their natural width instead of stretching across the terminal. Group widths stay stable through normal count growth, and thousands separators make large values easier to scan.
- Compact partition and user sections identify their values as `running / pending` job counts, omit table headers and resource columns, and keep aligned CPU-only and GPU pairs. The compact queue summary retains CPU-core and GPU resource totals. A long partition or user name shortens before complete metrics.
- At 72x20 and larger, compact terminals preserve at least one data row from every active detail section. Smaller terminals preserve every active section title when space permits.
- Tables must fit within the current terminal width and height. They must not wrap or scroll as queue size grows.
- Allocate available detail rows to active sections in round-robin order.
- Connected horizontal frame dividers separate sections when every active detail can still show data. Tight terminals omit dividers and use that space for data first.
- When rows are clipped, show `X shown · N hidden` in the section header.
- When no rows fit in a section budget, headers should still show the hidden-row count without `0 shown` phrasing (for example `N hidden`).
- In worst-case global viewport clipping, the final visible row must show `... output clipped to terminal height ...`.
- Connectivity indicator states:
  - loading
  - connected
  - reconnecting
  - disconnected
  - `disconnected, recovering`
- The header also shows:
  - age of last successful update
  - next retry countdown when reconnecting or recovering
- Ctrl+C exits and restores the terminal.

## Remote resilience contract
- Remote polling must tolerate transient errors and automatically retry.
- Startup capability probes in remote mode must also retry on transient transport failures.
- Reconnect loop uses bounded exponential backoff with jitter.
- Transport must respect SSH config features including `ProxyJump`, host aliases, and identity directives.
- No secrets or credentials are persisted by the tool.
- Existing SSH mechanisms (agent, keys, config) are preferred.
- Passwords are not accepted as CLI flags.

## Performance targets
- TUI remains responsive under frequent updates.
- Poll/render loop does not block terminal input handling.
- Data collection and rendering pipelines are decoupled so slow network polls do not freeze UI.

## Platform support
- Primary: macOS.
- Secondary: Linux.
- Unsupported: Windows.
- Shell assumptions should remain POSIX-compatible where possible.

## Security constraints
- Never commit secrets, credentials, tokens, private keys, or passwords.
- Avoid CLI flags that expose secrets in shell history/process list.
- Transport errors must omit executed commands and captured stdout.
- Locally displayed SSH targets and stderr must be treated as potentially sensitive when sharing diagnostics.

## Safety constraint
- The monitor must never submit mutating operations to Slurm.
- Runtime command allowlist is read-only Slurm queries (`squeue` and `scontrol` reads).

## Non-functional acceptance criteria
- Can run continuously for long periods without manual reconnect intervention.
- Handles poor network conditions without crashing or wedging UI.
- Names the failed startup capability or transport error so the operator can act.
