# slurm_monitor

`slurm_monitor` shows Slurm scheduler activity and queue pressure in a terminal. It runs locally or over SSH and never changes Slurm state.

## What it shows

1. Run locally on a cluster node or remotely over SSH.
2. View queue totals and partition, user, and pending-reason breakdowns.
3. Compare CPU and GPU use, queued demand, and available capacity.
4. Keep CPU-only and GPU job counts separate from CPU-core and GPU totals.
5. Continue monitoring while the tool retries transient SSH or network failures.
6. On large clusters, each clipped table states how many rows it shows and hides.

## Requirements

- Go `1.22+`
- POSIX `sh` available on the operator host and remote target environment
- Slurm CLI tools available on target environment (`squeue`, `scontrol`)
- OpenSSH `ssh` available for remote mode
- macOS or Linux on the operator host

## Repository security checks

- `gitleaks` runs in local pre-commit hooks and in GitHub Actions.
- Staged files and commit messages are blocked if they contain personal contact details, user-home path prefixes, or credential-like values.
- Commits must use a GitHub no-reply or reserved example address for author and committer metadata.
- Outbound pushes are blocked locally when new identities, messages, or added lines violate the same policy.
- Pull request titles and descriptions are checked in CI.

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

`--once` prints queue job and resource totals, available CPU and GPU capacity, and up to 10 rows from each sorted partition, user, pending-reason, and grouped-job list. Queue counts include each array task. The job list groups tasks only when root job, user, partition, state, and pending reason match.

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

## Common options

- `--refresh <duration>`, default `2s`
- `--connect-timeout <duration>`, default `10s`
- `--command-timeout <duration>`, default `15s`
- `--ssh-config <path>`
- `--identity-file <path>`
- `--port <int>`
- `--compact`
- `--no-color`
- `--once`
- `--duration <duration>` cleanly stops the monitor at its deadline, including during transient startup retries

## Limitations

- The monitor cannot cancel, requeue, hold, release, or submit jobs.
- Remote mode requires working OpenSSH access and remote Slurm command availability.
- Large clusters use capped tables that state how many rows are hidden.

## Completion command

- `slurm-monitor completion [bash|zsh]`
- `slurm-monitor completion --help`

## Optional shell alias

If `slurm-monitor` is in your `PATH`, use:

```bash
alias slurm_monitor='slurm-monitor'
```

To run from this repository, use:

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
