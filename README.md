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
