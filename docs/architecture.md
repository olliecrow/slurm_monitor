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

## 3) Collector pipeline
Collectors produce typed snapshots:
- `NodeSnapshot`
- `QueueSummarySnapshot`
- `UserQueueSnapshot`

Design principles:
- minimal round trips per poll tick (single combined remote command per poll for node + queue collection)
- clear parsers with defensive handling for missing optional metrics
- deterministic parse errors with useful context

## 4) Snapshot aggregation
Responsibilities:
- compute totals and derived percentages
- preserve raw values + display values (`n/a` where unavailable)
- track freshness timestamps
- aggregate user queue pressure with pending CPU-job and GPU-job counts for triage-focused views

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
- `Reconnecting`
- `DisconnectedRecovering`

Transitions:
- poll success -> `Connected`
- retryable transport failure -> `Reconnecting`
- repeated failure above threshold -> `DisconnectedRecovering`
- next success from recovery states -> `Connected`

Behavior:
- keep last known snapshot visible during non-connected states
- show error + age since last successful update
- continue retry loop until quit

## Retry policy
- bounded exponential backoff with jitter
- immediate short retry on first failure
- cap maximum backoff to preserve liveliness
- reset backoff after a successful poll

## Capability detection
- startup probe checks required commands in selected context
- startup probe retries transient transport failures with backoff; missing-command failures remain fatal.
- optional capability map can track optional metrics availability (for utilization fields)

## Data collection strategy

## Preferred command plan
Use read-only Slurm commands with stable parse contracts:
- node and allocation data from `scontrol show node -o`
- queue and per-user counts from `squeue -h -r` so job arrays are counted at task granularity

Optional metrics:
- CPU/memory/GPU utilization depends on cluster/slurm configuration.
- CPU/memory utilization values come from Slurm node fields (`CPULoad`, `FreeMem`) and may refresh slowly depending on cluster update cadence.
- GPU utilization is represented as allocation ratio (`GPUAlloc/GPUTotal`) when direct GPU activity metrics are unavailable, so it changes only when allocations change.
- if not available via standard commands, keep column but show `n/a`.

## Rendering layout plan

All terminals:
- top: connection header + last update/staleness
- middle: node summary panel
- bottom: combined queue panel (queue summary section + user summary section)

Compact terminals:
- maintain the same vertical panel order while reducing visible row/detail counts to fit height

## Error model
- fatal startup errors:
  - missing Slurm commands in target context
  - invalid CLI argument combinations
- non-fatal startup errors:
  - transient SSH/network/timeout failures during capability probe (retry loop continues)
- non-fatal runtime errors:
  - SSH timeout/drop
  - transient command failures
- parser errors:
  - surfaced in status panel; poll loop continues

## Security model
- no credential persistence
- no password flags
- rely on standard SSH auth flows (agent, key, config)
- redact potentially sensitive command/target details in logs/errors where needed
