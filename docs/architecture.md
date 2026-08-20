# Architecture

## High-level design

`slurm_monitor` is split into five runtime layers:
1. CLI/bootstrap
2. transport
3. collectors
4. domain model and aggregation
5. TUI renderer and interaction loop

```text
CLI -> Mode Resolver -> Transport -> Slurm Collectors -> Snapshot Model -> TUI ViewModel -> Render
                             ^                |
                             |                v
                     Reconnect Controller <- Errors/Timeouts
```

## Components

### 1) CLI/bootstrap
Responsibilities:
- parse flags and target
- provide contextual help/usage output (`-h`/`--help`)
- choose local vs remote mode
- run capability checks (`squeue`, `scontrol`)
- initialize TUI and background pollers

### 2) Transport abstraction
Interface:
- `Run(ctx, cmd) -> stdout/stderr/error`

Implementations:
- `LocalTransport`: executes commands directly.
- `SSHTransport`: executes via system `ssh`.

`SSHTransport` requirements:
- supports alias and `user@host` targets
- supports custom ssh config and identity file flags
- applies connect timeout and command timeout
- classifies failures into retryable vs fatal
- uses OpenSSH connection multiplexing (`ControlMaster`/`ControlPersist`) to reduce poll latency and improve live-update cadence.
- uses OpenSSH keepalive/retry options (`ServerAlive*`, `TCPKeepAlive`, `ConnectionAttempts`) for better behavior on flaky networks.
- uses POSIX `sh -lc` on the target rather than assuming `bash`.

### 3) Collector pipeline
Collectors produce typed data for a `Snapshot`:
- `QueueSummary`
- `[]PartitionSummary`
- `[]UserSummary`
- `[]PendingReasonSummary`
- `[]JobSummary`

Design principles:
- one `squeue -r` command per poll tick, using `tres-alloc` and pending reason data
- at most four cached `scontrol show job` fallback probes per tick, within one command-timeout budget, and only when a pending row omits TRES details
- clear parsers with defensive handling for missing optional metrics
- deterministic parse errors with useful context

### 4) Snapshot aggregation
Responsibilities:
- compute queue totals and resource demand
- track freshness timestamps
- aggregate queue, partition, and user job splits and CPU/GPU resources in running and pending states
- aggregate pending reasons by affected task and CPU/GPU demand
- group matching array tasks by root job, user, partition, state, and pending reason for bounded job insights

### 5) TUI runtime
Responsibilities:
- state store (`latest snapshot`, `connection state`, `error banner`, `staleness age`)
- resize-aware layout selection
- height-aware section budgeting so compact terminals keep critical sections visible
- high-frequency render loop independent from poll cadence

Runtime stack:
- `bubbletea` for event loop and rendering
- `lipgloss` for table and status styling

## Remote resilience model

### Connection state machine
States:
- `Connected`
- `Disconnected`
- `Reconnecting`
- `DisconnectedRecovering`

Transitions:
- poll success -> `Connected`
- non-retryable failure -> `Disconnected`
- retryable transport failure -> `Reconnecting`
- repeated failure above threshold -> `DisconnectedRecovering`
- next success from recovery states -> `Connected`

Behavior:
- keep last known snapshot visible during non-connected states
- show error + age since last successful update
- continue retry loop only for retryable failures
- stop retrying after permanent configuration/auth/parser-contract failures and leave the UI disconnected until quit

### Retry policy
- bounded exponential backoff with jitter
- immediate short retry on first failure
- cap maximum backoff to preserve liveliness
- reset backoff after a successful poll

### Capability detection
- startup probe checks required commands in selected context
- startup probe uses `sh -lc` in both local and remote modes.
- startup probe retries transient transport failures with backoff; missing-command and permanent SSH/config/shell failures remain fatal.
- `doctor` runs a single non-mutating probe pass and reports pass/fail without entering the monitor loop.
- `dry-run` prints planned stages and executes no transport commands.

## Data collection strategy

### Command plan
Use read-only Slurm commands with stable parse contracts:
- queue job counts, resource totals, and pending reasons from `squeue -h -r -O ... tres-alloc ... Reason ...` so job arrays are counted at task granularity and CPU/GPU totals come from Slurm's documented TRES data
- bounded `scontrol show job -o` fallback probes only when pending rows omit TRES details

Optional metrics:
- pending GPU demand can require the bounded job-detail fallback when a cluster omits TRES data from pending `squeue` rows.

## Rendering layout

All terminals:
- top: connection header + last update/staleness
- middle: scheduler-insights panel with a compact queue activity summary, then partition, user, and pending-reason sections
- use the stabilized terminal width without an extra right margin
- share available detail height fairly across active sections

Compact terminals:
- keep the same section order and use self-describing rows without table headers
- spend saved header space on additional visible data rows

## Error model
- fatal startup errors:
  - missing Slurm commands in target context
  - invalid CLI argument combinations
  - permanent SSH/auth/configuration/shell-contract failures
- non-fatal startup errors:
  - transient SSH/network/timeout failures during capability probe (retry loop continues)
- non-fatal runtime errors:
  - SSH timeout/drop
  - transient command failures
- parser errors:
  - surfaced in status panel and treated as non-retryable so the operator sees a disconnected terminal instead of an infinite retry loop

## Security model
- no credential persistence
- no password flags
- rely on standard SSH auth flows (agent, key, config)
- omit executed commands and captured stdout from transport errors
- treat locally displayed SSH targets and stderr as potentially sensitive when sharing diagnostics
