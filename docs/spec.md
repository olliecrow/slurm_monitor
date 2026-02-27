# Product Spec

## Goal
Build a resilient CLI/TUI monitor for Slurm that can run:
- locally on a host with Slurm CLI access, or
- remotely over SSH with robust recovery from network drops and SSH failures.

The tool should run for long periods with minimal operator interaction and provide a clear live view of cluster node usage and queue state.

## Scope

### In scope
- Full-screen terminal UI that adapts to terminal resize.
- Live polling and rendering loop with bounded CPU overhead.
- Local mode (default when no SSH target is provided).
- Remote mode via SSH target (alias from SSH config or `user@host` style target).
- Recovery behavior for transient SSH/network failures.
- Three primary data views:
  - node summary view (per-node rows + aggregate totals)
  - queue summary view (cluster-level running/pending counts)
  - user view (per-user running/pending counts)
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
- `--once`: collect one snapshot and print text summary.
- `--duration <duration>`: optional auto-exit timer for TUI runs.

## Startup Behavior

### Mode selection
- If no target is provided, run local checks and start local mode.
- If target is provided, run remote checks and start remote mode.

### Capability checks (must pass before entering TUI loop)
- Required commands: `sinfo`, `squeue`, `scontrol`.
- Local mode:
  - if required commands are missing locally, exit with a clear error.
- Remote mode:
  - run capability check remotely via SSH.
  - if required commands are missing on remote, exit with a clear error.

### Failure semantics
- Non-recoverable startup failures are fatal:
  - missing required Slurm commands in selected context
  - invalid CLI argument combinations
- Transient startup failures (SSH/network/timeouts) are retried automatically with backoff.
- Retry behavior is unbounded by default and continues until operator quit; when `--duration` is set, retries stop at the configured deadline.
- Runtime poll failures are non-fatal unless operator chooses to quit; stale data remains visible with staleness and connectivity markers.

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

## Runtime Data Contract

### 1) Node summary view
Per-node fields:
- node name
- node state (preserve full Slurm composite state including qualifiers such as `+DRAIN` and `+DOWN`)
- CPU allocation (`allocated/total`)
- CPU utilization (if available from Slurm-reported metrics; else display `n/a`)
- memory allocation (`allocated/total`)
- memory utilization (if available; else `n/a`)
- GPU allocation (`allocated/total`)
- GPU utilization (if available; else `n/a`)
- partition(s)
- explicit node-health alert line in the node summary panel when any node is `DOWN` or `DRAIN`

Aggregate row:
- totals across visible nodes for allocation/usage signals where mathematically valid.

### 2) Queue summary view
Fields:
- running jobs count
- pending jobs count
- other jobs count
- aligned label/count rows for running/pending/other/total
- counts include Slurm job arrays at array-task granularity (each array task counts as one job).

### 3) User view
Per-user fields:
- user
- running count
- pending count
- pending CPU-job count
- pending GPU-job count
- pending CPU-job count + pending GPU-job count equals pending count for each user.
- job-type counts include Slurm job arrays at array-task granularity.

## TUI Behavior
- Full-screen layout.
- Dynamic resize handling for width/height changes.
- Live updates without requiring restart.
- Non-interactive display: no in-app controls or navigation; monitor-only rendering.
- Header includes a heartbeat clock and refresh age.
- Header includes a status spinner so refresh/liveness is visible even when metrics are stable.
- Header intentionally omits node-health alert badges; `DOWN`/`DRAIN` alerts are shown directly in the node summary panel.
- Body renders two vertically stacked panels in fixed order:
  - node summary
  - combined queue panel (queue summary section + user view section)
- Compact terminals reduce row/detail density but keep the same two-panel vertical order.
- Node and user tables are height-bounded and width-bounded from current terminal dimensions to avoid wrap/scroll drift on large clusters.
- Row budgets are computed from per-panel content height (not just global terminal height) so mandatory lines remain visible under tight layouts.
- When rows are clipped, section headers must show deterministic truncation metadata (for example `top X/Y, +N hidden`).
- When no rows fit in a panel budget, headers should still show hidden-row metadata without `top 0/...` phrasing (for example `+N hidden`).
- Node summary must always include node-alert line (when applicable) and `TOTAL` aggregate row, even when per-node rows are clipped.
- In worst-case global viewport clipping, the final visible row must show `... output clipped to terminal height ...`.
- Connectivity indicator states:
  - loading
  - connected
  - reconnecting
  - disconnected (recovering)
- Connectivity panel also shows:
  - age of last successful update
  - next retry countdown when reconnecting
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
- Shell assumptions should remain POSIX-compatible where possible.

## Security Constraints
- Never commit secrets, credentials, tokens, private keys, or passwords.
- Avoid CLI flags that expose secrets in shell history/process list.
- Log output must redact sensitive target material if it may contain credentials.

## Safety Constraint
- The monitor must never submit mutating operations to Slurm.
- Runtime command allowlist is read-only Slurm queries (`sinfo`, `squeue`, `scontrol` reads).

## Non-functional acceptance criteria
- Can run continuously for long periods without manual reconnect intervention.
- Handles poor network conditions without crashing or wedging UI.
- Produces explicit, actionable error messages for startup capability failures.
