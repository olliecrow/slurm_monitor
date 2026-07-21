# slurm_monitor repository guidance

## Purpose and layout

- `slurm_monitor` is a public Go CLI/TUI for read-only local or SSH monitoring of Slurm clusters on macOS and Linux.
- The CLI entry point is `cmd/slurm-monitor/`. Runtime packages live under `internal/`; fixtures and tests stay beside the packages they cover.
- `docs/spec.md` is the behavior contract, `docs/architecture.md` defines component and resilience boundaries, `docs/security.md` defines auth and disclosure constraints, and `docs/decisions.md` records durable product rationale.

## Product and safety contracts

- Keep Slurm access strictly read-only. The command allowlist is limited to `sinfo`, `squeue`, and read-only `scontrol` queries; do not add queue mutation controls.
- Preserve local and system-OpenSSH transports, POSIX `sh -lc`, standard SSH config/agent/key behavior, aliases, `user@host`, ProxyJump, and optional identity/config/port overrides. Never accept password flags or persist credentials.
- Fail fast for invalid arguments, missing capabilities, and permanent auth/config/parser failures. Retry only transient transport failures, keep the last good snapshot visible, and retain explicit stale/recovery state.
- Count arrays at task granularity with `squeue -r`; use Slurm `tres-alloc` for queue CPU/GPU totals and keep the bounded job-detail fallback where required.
- Preserve composite node states, node alerts, allocation-vs-utilization labels, deterministic clipping metadata, mandatory total rows, resize behavior, and terminal restoration.
- Keep public surfaces free of secrets, private targets, machine paths, confidential cluster details, and sensitive logs.

## Development and verification

- Format Go changes with `gofmt`.
- Run `go test ./...`, `go test -race ./...`, and `go vet ./...` for material runtime, parser, transport, or TUI changes.
- Use `go run ./cmd/slurm-monitor --help` and `go run ./cmd/slurm-monitor dry-run [target]` for non-mutating CLI checks. Use `doctor` or live monitor mode only against an explicitly authorized target.
- Keep README, completion/help text, spec, architecture, security notes, and tests aligned when their shared contract changes.
- Run the repository's sensitive-text checks before publishing changes.
