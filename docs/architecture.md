# Architecture

## High-level design

`slurm_monitor` is split into four runtime layers:
1. CLI/bootstrap
2. transport and collectors
3. domain model + aggregation
4. TUI renderer and interaction loop

```text
CLI -> Mode Resolver -> Transport -> Slurm Collectors -> Snapshot Model -> TUI ViewModel -> Render
                             ^                |
                             |                v
                     Reconnect Controller <- Errors/Timeouts
```

## Components

## 1) CLI/bootstrap
Responsibilities:
- parse flags and target
- provide contextual help/usage output (`-h`/`--help`)
- choose local vs remote mode
- run capability checks (`sinfo`, `squeue`, `scontrol`)
- initialize TUI and background pollers

## 2) Transport abstraction
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

## 3) Collector pipeline
Collectors produce typed data for a `Snapshot`:
- `[]Node`
- `QueueSummary`
- `[]UserSummary`

Design principles:
- minimal round trips per poll tick (single combined command for node + queue collection using `squeue -r` plus `tres-alloc` for requested/allocated job resources, with cached per-root `scontrol show job` probes as a fallback when pending GPU request details are still missing)
- clear parsers with defensive handling for missing optional metrics
- deterministic parse errors with useful context
- preserve scheduler-critical composite node state qualifiers (`+DRAIN`, `+DOWN`) during parsing; only cosmetic state markers are stripped

## 4) Snapshot aggregation
Responsibilities:
- compute totals and derived percentages
- preserve raw values + display values (`n/a` where unavailable)
- track freshness timestamps
- aggregate queue and user job splits for CPU jobs and GPU jobs in running and pending states

## 5) TUI runtime
Responsibilities:
- state store (`latest snapshot`, `connection state`, `error banner`, `staleness age`)
- resize-aware layout selection
- height-aware section budgeting so compact terminals keep critical sections visible
- high-frequency render loop independent from poll cadence

Recommended stack:
- `bubbletea` for event loop and rendering
- `lipgloss` for table and status styling

## Remote resilience model

## Connection state machine
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

## Retry policy
- bounded exponential backoff with jitter
- immediate short retry on first failure
- cap maximum backoff to preserve liveliness
- reset backoff after a successful poll

## Capability detection
- startup probe checks required commands in selected context
- startup probe uses `sh -lc` in both local and remote modes.
- startup probe retries transient transport failures with backoff; missing-command and permanent SSH/config/shell failures remain fatal.
- `doctor` runs a single non-mutating probe pass and reports pass/fail without entering the monitor loop.
- `dry-run` prints planned stages and executes no transport commands.
- optional capability map can track optional metrics availability (for utilization fields)

## Data collection strategy

## Preferred command plan
Use read-only Slurm commands with stable parse contracts:
- node and allocation data from `scontrol show node -o`
- queue job counts and resource totals from `squeue -h -r -O ... tres-alloc ...` so job arrays are counted at task granularity and CPU/GPU totals come from Slurm's documented TRES data

Optional metrics:
- CPU/memory/GPU utilization depends on cluster/slurm configuration.
- CPU/memory utilization values come from Slurm node fields (`CPULoad`, `FreeMem`) and may refresh slowly depending on cluster update cadence.
- GPU percentage shown in the TUI is allocation ratio (`GPUAlloc/GPUTotal`), labeled explicitly as allocation percentage because true device activity is not available from the current collector contract.
- if GPU totals are unavailable, keep the column but show `n/a`.

## Rendering layout plan

All terminals:
- top: connection header + last update/staleness
- middle: node summary panel
- bottom: combined queue panel (queue summary section + user summary section)
- when any node is `DOWN` or `DRAIN`, render a node-health alert line at the top of the node summary panel
- keep node-health alerts out of the header to reduce top-line noise and keep clock/status readability

Compact terminals:
- maintain the same vertical panel order while reducing visible row/detail counts to fit height

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
- redact potentially sensitive command/target details in logs/errors where needed
