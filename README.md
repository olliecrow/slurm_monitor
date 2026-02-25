# slurm_monitor

`slurm_monitor` is a terminal-first monitor for Slurm clusters with:
- local mode (run on a node that has Slurm CLI tools installed),
- remote mode over SSH (including SSH config aliases and bastion/proxy jumps),
- a full-screen live-updating TUI for node health and queue state.
- non-interactive display behavior (no in-app controls).
- queue summary with running/pending/other/total counts in a clean aligned list.
- explicit heartbeat clock/spinner in the header with last-update age and refresh cadence so live refresh is visible.
- adaptive compact layout that preserves the fixed vertical panel order.
- user-view pending job-type columns (pending CPU jobs / pending GPU jobs) so the split sums to per-user pending.
- two-panel stack: node summary panel, then a combined queue panel (queue summary at top + user view at bottom).
- queue and user counts account for Slurm job arrays at array-task granularity.

The monitor is strictly read-only and never submits mutating Slurm operations.
Transient SSH/network failures (including startup preflight) are retried automatically until recovery or process stop.

## Build
```bash
go build ./cmd/slurm-monitor
```

## Optional shell alias
If you run this tool often, add an alias in your shell rc file so you can start it with a short command.

If `slurm-monitor` is already in your `PATH`:
```bash
# ~/.bashrc or ~/.zshrc
alias slurm_monitor='slurm-monitor'
```

If you prefer running from this repo without installing a binary:
```bash
# ~/.bashrc or ~/.zshrc
alias slurm_monitor='go run /path/to/slurm_monitor/cmd/slurm-monitor'
```

Reload your shell config:
```bash
source ~/.bashrc   # bash
source ~/.zshrc    # zsh
```

## Run
Show CLI help (flags, behavior, auth model, examples):
```bash
go run ./cmd/slurm-monitor --help
```

Local mode (fails fast if Slurm tools are missing locally):
```bash
go run ./cmd/slurm-monitor
```

Remote mode using SSH alias or `user@host`:
```bash
go run ./cmd/slurm-monitor cluster_alias
go run ./cmd/slurm-monitor user@cluster.example.org
```

One-shot collection mode:
```bash
go run ./cmd/slurm-monitor --once cluster_alias
```

## Core flags
- `--refresh <duration>` (default `2s`)
- `--connect-timeout <duration>` (default `10s`)
- `--command-timeout <duration>` (default `15s`)
- `--ssh-config <path>`
- `--identity-file <path>`
- `--port <int>`
- `--compact`
- `--no-color`
- `--once`
- `--duration <duration>` (auto-exit timer for scripted TUI runs)

## Runtime stack
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) for TUI runtime.
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) for styling/layout.
- OpenSSH (`ssh`) for remote transport with config/alias/bastion support.

## Canonical docs
- [docs/spec.md](docs/spec.md): product/runtime behavior contract
- [docs/architecture.md](docs/architecture.md): system design and resilience model
- [docs/implementation-plan.md](docs/implementation-plan.md): phased build + verification plan
- [docs/open-questions.md](docs/open-questions.md): implementation clarifications and resolved defaults
- [docs/security.md](docs/security.md): secrets/auth/logging policy
- [docs/project-preferences.md](docs/project-preferences.md): durable project maintenance preferences
