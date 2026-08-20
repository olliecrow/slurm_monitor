# slurm_monitor

`slurm_monitor` is a terminal-first monitor for Slurm clusters.
It supports local mode and remote mode over SSH, and it stays read only.

## Current status

This project is actively maintained for read-only cluster monitoring workflows.

## What this project is trying to achieve

Give you a clear live view of scheduler activity and queue pressure without running any mutating Slurm commands.

## What you experience as a user

1. Run the tool locally on a cluster node, or remotely over SSH.
2. See a live terminal user interface (TUI) with cluster, partition, user, pending-reason, and job insights.
3. Compare CPU-only jobs, GPU jobs, and CPU/GPU resource pressure with plain-language grouped columns.
4. Keep monitoring through transient SSH or network failures, with automatic retries.
5. On very large clusters, tables state how many rows are shown and hidden so the terminal stays stable.

## Requirements

- Go `1.22+`
- POSIX `sh` available on the operator host and remote target environment
- Slurm CLI tools available on target environment (`squeue`, `scontrol`)
- OpenSSH `ssh` available for remote mode
- supported operator platforms: macOS and Linux only

## Security guardrails for public/open-source readiness

- `gitleaks` runs in local pre-commit hooks and in GitHub Actions.
- Commit messages are blocked if they include local absolute paths or credential-like values.
- Outbound pushes are blocked locally when new commit messages or patches contain sensitive patterns.
- Pull request titles/descriptions are checked in CI for the same policy.

Install [pre-commit](https://pre-commit.com/#install), then set up the repository hooks.

```bash
pre-commit install
```

Run all configured hooks manually.

```bash
pre-commit run --all-files
```

Run the Go development checks.

```bash
gofmt -l .
go mod verify
go test ./...
go test -race ./...
go vet ./...
```

## Quick start

Install the latest public version.

```bash
go install github.com/olliecrow/slurm_monitor/cmd/slurm-monitor@latest
```

Or build from a cloned repository.

```bash
go build ./cmd/slurm-monitor
```

Show help.

```bash
go run ./cmd/slurm-monitor --help
```

Install shell tab completion.

```bash
# bash
go run ./cmd/slurm-monitor completion bash > ~/.local/share/bash-completion/completions/slurm-monitor

# zsh
mkdir -p ~/.zsh/completions
go run ./cmd/slurm-monitor completion zsh > ~/.zsh/completions/_slurm-monitor
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

`--once` prints queue job and resource totals, top partition, user, and pending-reason rows, and grouped root jobs. Array-task counts stay task-granular; the job list groups matching tasks to stay useful on large arrays.

## Doctor output example

```text
slurm-monitor doctor
mode: local
target: local

[ok] local tool sh: /bin/sh
[ok] local tool squeue: /usr/bin/squeue
[ok] local tool scontrol: /usr/bin/scontrol
[ok] slurm preflight: required Slurm commands are reachable on local

doctor result: PASS
```

## Dry-run output example

```text
slurm-monitor dry-run
mode: local
target: local
refresh: 2s
connect-timeout: 10s
command-timeout: 15s
duration: unbounded
once: false
compact: false
no-color: false

planned sequence:
1. Parse flags and build the configured transport.
2. Run a local preflight check for sh, squeue, and scontrol.
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

## Known limitations

- Read-only monitor only; it does not support queue mutation actions.
- Remote mode requires working OpenSSH access and remote Slurm command availability.
- Very large clusters are intentionally summarized in capped top slices with explicit hidden-row counts.

## Completion command

- `slurm-monitor completion [bash|zsh]`
- `slurm-monitor completion --help`

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

- [docs/README.md](docs/README.md): entrypoint index for repository-maintainer docs
- [docs/spec.md](docs/spec.md): product and runtime behavior
- [docs/architecture.md](docs/architecture.md): system design and resilience model
- [docs/decisions.md](docs/decisions.md): durable rationale and decision records
- [docs/security.md](docs/security.md): secrets, auth, and logging policy
