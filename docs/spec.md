# Product Spec

## Goal
Build a resilient CLI/TUI monitor for Slurm that can run:
- locally on a host with Slurm CLI access, or
- remotely over SSH with robust recovery from network drops and SSH failures.

The tool should run for long periods with minimal operator interaction and provide a clear live view of scheduler activity and queue pressure.

## Scope

### In scope
- Full-screen terminal UI that adapts to terminal resize.
- Live polling and rendering loop with bounded CPU overhead.
- Local mode (default when no SSH target is provided).
- Remote mode via SSH target (alias from SSH config or `user@host` style target).
- Recovery behavior for transient SSH/network failures.
- Four primary data views:
  - scheduler summary view (cluster-level job counts and CPU/GPU resource totals)
  - partition view (per-partition job counts and CPU/GPU resource totals)
  - user view (per-user job counts and CPU/GPU resource totals)
  - pending-reason view (scheduler reasons with affected task, CPU, and GPU demand)
- Queue labels must make it clear these are job or array-task counts, not held CPU or GPU resource totals.
- Clear connectivity status indicators in the UI.

### Out of scope
- Mutating Slurm state (cancel/requeue/hold/release).
- Embedding credentials in files or CLI history.
- Replacing OpenSSH with a custom SSH stack.

## CLI Contract

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
- Invalid argument combinations or unknown flags must return actionable errors.
- Parse errors must direct users to `slurm-monitor --help`.

### Core flags
- `--refresh <duration>`: poll interval (default `2s`).
- `--connect-timeout <duration>`: SSH command connect timeout.
- `--command-timeout <duration>`: per poll command timeout.
- `--ssh-config <path>`: optional custom SSH config file.
- `--identity-file <path>`: optional SSH identity file.
- `--port <int>`: optional SSH port override.
- `--no-color`: disable colored UI output.
- `--compact`: compact layout for small terminal dimensions.
- `--once`: collect one snapshot and print a text summary with queue job and resource totals, and top partition, user, pending-reason, and grouped-job rows.
- `--duration <duration>`: optional auto-exit timer for TUI runs.

## Startup Behavior

### Mode selection
- If no target is provided, run local checks and start local mode.
- If target is provided, run remote checks and start remote mode.

### Capability checks (must pass before entering TUI loop)
- Required commands: `squeue`, `scontrol`.
- Command execution uses POSIX `sh -lc` in both local and remote modes.
- Local mode:
  - if `sh` or required Slurm commands are missing locally, exit with a clear error.
- Remote mode:
  - run capability check remotely via SSH through `sh -lc`.
  - if the remote shell contract or required Slurm commands are missing, exit with a clear error.

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

## Helper Command Behavior

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

## Runtime Data Contract

### 1) Scheduler summary view
Fields:
- running CPU-job count
- running GPU-job count
- pending CPU-job count
- pending GPU-job count
- other jobs count
- running CPU and GPU resource totals
- pending CPU and GPU resource demand
- concise running and pending activity lines that distinguish CPU-only jobs, GPU jobs, CPU resources, and GPU resources
- counts include Slurm job arrays at array-task granularity (each array task counts as one job).

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
- default user ordering should keep the biggest current holders near the top, with pending demand used as a tie-breaker.

### 3) Partition view
Per-partition fields:
- partition name
- running CPU-job count
- running GPU-job count
- pending CPU-job count
- pending GPU-job count
- counts and resource totals use the same task-granular rules as the scheduler summary.
- default ordering surfaces pending GPU pressure, pending CPU pressure, then current GPU and CPU load.

### 4) Pending-reason view
Per-reason fields:
- pending reason reported by Slurm
- affected array-task count
- total pending CPU demand
- total pending GPU demand
- blank or unavailable reasons appear as `<unknown>`.
- default ordering surfaces GPU demand, CPU demand, then affected task count.

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
- default ordering surfaces pending GPU work, running GPU work, pending CPU work, then running CPU work; larger grouped jobs sort first within each class.

## TUI Behavior
- Full-screen layout.
- Dynamic resize handling for width/height changes.
- Live updates without requiring restart.
- Non-interactive display: no in-app controls or navigation; monitor-only rendering.
- Header includes a heartbeat clock and update age.
- Header includes a status spinner so refresh/liveness is visible even when metrics are stable.
- When the header is narrow, it removes lower-priority fields as complete units instead of cutting labels or values mid-word.
- Body renders one scheduler-insights panel with summary, partition, user, and pending-reason sections.
- The queue summary uses one activity line on wide terminals and separate running and pending lines when the combined line does not fit.
- Wide partition and user tables group plain-language running jobs, pending jobs, resources in use, and requested resources. Values identify CPU-only jobs, GPU jobs, CPUs, and GPUs explicitly.
- Wide pending-reason tables give spare terminal width to reason text before truncating it.
- Wide tables use the minimum width needed for their current labels and values instead of stretching columns across the terminal.
- Compact detail sections omit table headers and use self-describing rows so more data fits. Partition and user section titles identify their values as job counts, and each row labels running and pending CPU-only/GPU values; pending-reason rows include task, CPU, and GPU units. Data is truncated only when the terminal width requires it.
- At 72x20 and larger, compact terminals preserve at least one data row from every active detail section. Smaller terminals preserve every active section title when space permits.
- Partition, user, and pending-reason tables are height-bounded and width-bounded from current terminal dimensions to avoid wrap/scroll drift on large clusters.
- Row budgets are computed from panel content height and shared fairly across active detail sections.
- The panel separates sections with blank lines when every active detail can still show at least one data row. Tight terminals remove spacing before data.
- When rows are clipped, section headers must show deterministic plain-language metadata (for example `X shown · N hidden`).
- When no rows fit in a panel budget, headers should still show the hidden-row count without `0 shown` phrasing (for example `N hidden`).
- In worst-case global viewport clipping, the final visible row must show `... output clipped to terminal height ...`.
- The insights panel fills the body above the footer, including when all data rows fit with height to spare.
- Connectivity indicator states:
  - loading
  - connected
  - reconnecting
  - disconnected
  - disconnected (recovering)
- Connectivity panel also shows:
  - age of last successful update
  - next retry countdown when reconnecting or recovering
- Graceful quit with standard terminal restoration.

## Remote Resilience Contract
- Remote polling must tolerate transient errors and automatically retry.
- Startup capability probes in remote mode must also retry on transient transport failures.
- Reconnect loop uses bounded exponential backoff with jitter.
- Transport must respect SSH config features including `ProxyJump`, host aliases, and identity directives.
- No secrets or credentials are persisted by the tool.
- Existing SSH mechanisms (agent, keys, config) are preferred.
- Passwords are not accepted as CLI flags.

## Performance Targets
- TUI remains responsive under frequent updates.
- Poll/render loop does not block terminal input handling.
- Data collection and rendering pipelines are decoupled so slow network polls do not freeze UI.

## Platform Support
- Primary: macOS.
- Secondary: Linux.
- Unsupported: Windows.
- Shell assumptions should remain POSIX-compatible where possible.

## Security Constraints
- Never commit secrets, credentials, tokens, private keys, or passwords.
- Avoid CLI flags that expose secrets in shell history/process list.
- Transport errors must omit executed commands and captured stdout.
- Locally displayed SSH targets and stderr must be treated as potentially sensitive when sharing diagnostics.

## Safety Constraint
- The monitor must never submit mutating operations to Slurm.
- Runtime command allowlist is read-only Slurm queries (`squeue` and `scontrol` reads).

## Non-functional acceptance criteria
- Can run continuously for long periods without manual reconnect intervention.
- Handles poor network conditions without crashing or wedging UI.
- Produces explicit, actionable error messages for startup capability failures.
