# Architecture

## High-level design

`slurm_monitor` has five runtime layers:

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
The CLI layer:

- parse flags and target
- provide contextual help/usage output (`-h`/`--help`)
- choose local vs remote mode
- run capability checks (`squeue`, `scontrol`)
- initialize TUI and background pollers

### 2) Transport abstraction
The transport interface is:

- `Run(ctx, cmd) -> stdout/stderr/error`

Two implementations satisfy it:

- `LocalTransport`: executes commands directly.
- `SSHTransport`: executes via system `ssh`.

`SSHTransport`:

- supports alias and `user@host` targets
- supports custom ssh config and identity file flags
- applies connect timeout and command timeout
- classifies failures into retryable vs fatal
- uses OpenSSH connection multiplexing (`ControlMaster` and `ControlPersist`) to reduce poll latency
- uses `ServerAlive*` and `TCPKeepAlive` to detect broken connections and `ConnectionAttempts` to retry connection setup
- uses POSIX `sh -lc` on the target rather than assuming `bash`.

### 3) Collector pipeline
Collectors build these typed parts of a `Snapshot`:

- `QueueSummary`
- `AvailableResources`
- `[]PartitionSummary`
- `[]UserSummary`
- `[]PendingReasonSummary`
- `[]JobSummary`

Each poll:

- one combined read-only `scontrol show nodes --oneliner` and `squeue -r` command per poll tick, using node state/capacity plus queue `tres-alloc` and pending reason data
- at most four cached `scontrol show job` fallback probes per tick, within one command-timeout budget, and only when a pending row omits TRES details
- parsers accept missing optional metrics
- parse errors identify the row and failed field

### 4) Snapshot aggregation
Snapshot aggregation:

- compute queue totals, resource demand, and current cluster availability
- track freshness timestamps
- aggregate queue, partition, and user job splits and CPU/GPU resources in running and pending states
- aggregate pending reasons by affected task and CPU/GPU demand
- group matching array tasks by root job, user, partition, state, and pending reason so the `--once` job list has one row per group

### 5) TUI runtime
The TUI runtime:

- stores the latest snapshot, connection state, error text, and staleness age
- selects a layout for the current terminal size
- allocates section rows from the terminal height in round-robin order
- updates the clock and status once per second, independently of Slurm polling

The runtime uses:

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
- show the error and age since the last successful update
- continue retry loop only for retryable failures
- stop retrying after permanent configuration/auth/parser-contract failures and leave the UI disconnected until quit

### Retry policy
- bounded exponential backoff with jitter
- immediate short retry on first failure
- cap the retry delay
- reset backoff after a successful poll

### Capability detection
- startup probe checks required commands in selected context
- startup probe uses `sh -lc` in both local and remote modes.
- startup probe retries transient transport failures with backoff; missing-command and permanent SSH/config/shell failures remain fatal.
- `doctor` runs a single non-mutating probe pass and reports pass/fail without entering the monitor loop.
- `dry-run` prints planned stages and executes no transport commands.

## Data collection strategy

### Command plan
The collector runs these read-only Slurm queries:

- available CPU/GPU capacity from deduplicated `scontrol show nodes --oneliner` rows; count unallocated capacity only on schedulable `IDLE` or `MIXED` nodes without blocking flags
- queue job counts, resource totals, and pending reasons from `squeue -h -r -O ... tres-alloc ... Reason ...` so job arrays are counted at task granularity and CPU/GPU totals come from Slurm's documented TRES data
- bounded `scontrol show job -o` fallback probes only when pending rows omit TRES details

Optional metrics:
- pending GPU demand can require the bounded job-detail fallback when a cluster omits TRES data from pending `squeue` rows.
- node GPU capacity uses `CfgTRES` and `AllocTRES`, with `Gres` and `GresUsed` as compatibility fallbacks.

## Rendering layout

All terminals:

- top: connection header + last update/staleness
- middle: a content-height frame with equal left and right padding
- dashboard order: queue activity and available capacity, partitions, users, then pending reasons
- use the stabilized terminal width without an extra right margin
- allocate detail rows to active sections in round-robin order
- replace available separator rows with horizontal rules connected to the outer frame

Compact terminals:

- keep the same section order and use self-describing rows without table headers
- spend saved header space on additional visible data rows

Short wide terminals:

- retain queue-wide CPU-core and GPU in-use, requested, and available totals in the summary when the available height cannot fit the full partition grid

## Error model
- fatal startup errors:
  - missing Slurm commands in target context
  - invalid CLI argument combinations
  - permanent SSH/auth/configuration/shell-contract failures
- non-fatal startup errors:
  - transient SSH, network, and timeout failures during capability probing; the retry loop continues
- non-fatal runtime errors:
  - SSH timeout/drop
  - transient command failures
- parser errors:
  - shown on the header's error line and treated as non-retryable so the operator sees a disconnected terminal instead of an infinite retry loop

## Security model
- no credential persistence
- no password flags
- rely on SSH agents, keys, and configuration
- omit executed commands and captured stdout from transport errors
- treat locally displayed SSH targets and stderr as potentially sensitive when sharing diagnostics
