# slurm_monitor

<img width="555" height="616" alt="image" src="https://github.com/user-attachments/assets/c7137a9a-b790-43fa-8986-070dde76ca2d" />


`slurm_monitor` is a terminal-first monitor for Slurm clusters.
It supports local mode and remote mode over SSH, and it stays read only.

## What this project is trying to achieve

Give you a clear live view of cluster health and queue state without running any mutating Slurm commands.

## What you experience as a user

1. Run the tool locally on a cluster node, or remotely over SSH.
2. See a live TUI with node summary and queue views.
3. Track queue counts, user pending counts, and array task counts.
4. Keep monitoring through transient SSH or network failures, with automatic retries.

## Quick start

Build the binary.

```bash
go build ./cmd/slurm-monitor
```

Show help.

```bash
go run ./cmd/slurm-monitor --help
```

Run doctor preflight checks.

```bash
go run ./cmd/slurm-monitor doctor
go run ./cmd/slurm-monitor doctor cluster_alias
```

Preview the execution plan without running commands.

```bash
go run ./cmd/slurm-monitor dry-run
go run ./cmd/slurm-monitor dry-run --once cluster_alias
```

Run local mode.

```bash
go run ./cmd/slurm-monitor
```

Run remote mode.

```bash
go run ./cmd/slurm-monitor cluster_alias
go run ./cmd/slurm-monitor user@cluster.example.org
```

Run one-shot collection.

```bash
go run ./cmd/slurm-monitor --once cluster_alias
```

## Doctor output example

```text
slurm-monitor doctor
mode: remote
target: cluster_alias

[ok] local tool ssh: /usr/bin/ssh
[ok] slurm preflight: required Slurm commands are reachable on ssh:cluster_alias

doctor result: PASS
```

## Dry-run output example

```text
slurm-monitor dry-run
mode: remote
target: cluster_alias
refresh: 2s
connect-timeout: 10s
command-timeout: 15s
duration: unbounded
once: false
compact: false
no-color: false

planned sequence:
1. Parse flags and build the configured transport.
2. Connect over OpenSSH to the target and validate sinfo, squeue, and scontrol remotely.
3. Start the polling loop and render the live TUI until interrupted or duration is reached.
4. Exit without mutating any Slurm queue or cluster state.

dry-run only: no local or remote commands were executed.
```

## Helpful options

- `--refresh <duration>`, default `2s`
- `--connect-timeout <duration>`, default `10s`
- `--command-timeout <duration>`, default `15s`
- `--ssh-config <path>`
- `--identity-file <path>`
- `--port <int>`
- `--compact`
- `--no-color`
- `--once`
- `--duration <duration>`

## Optional shell alias

If `slurm-monitor` is in your `PATH`.

```bash
alias slurm_monitor='slurm-monitor'
```

If you prefer to run from this repo.

```bash
alias slurm_monitor='go run /path/to/slurm_monitor/cmd/slurm-monitor'
```

Reload your shell config.

```bash
source ~/.bashrc
source ~/.zshrc
```

## Runtime stack

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the TUI runtime
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) for styling and layout
- OpenSSH `ssh` for remote transport and SSH config support

## Canonical docs

- [docs/spec.md](docs/spec.md): product and runtime behavior
- [docs/architecture.md](docs/architecture.md): system design and resilience model
- [docs/implementation-plan.md](docs/implementation-plan.md): phased build and verification plan
- [docs/open-questions.md](docs/open-questions.md): implementation clarifications and resolved defaults
- [docs/security.md](docs/security.md): secrets, auth, and logging policy
- [docs/project-preferences.md](docs/project-preferences.md): durable project maintenance preferences
