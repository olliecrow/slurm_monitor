---
name: slurm-monitor-live-cluster-validation
description: Validate slurm_monitor against an explicitly authorized live Slurm SSH target after runtime, parser, transport, or TUI changes. Use for bounded read-only checks; never use it to mutate Slurm or network state.
---

# slurm_monitor live cluster validation

Check changed behavior against a real cluster without exposing or changing cluster state.

## Preconditions

- Use only the exact target that the user authorized for this task.
- Use the existing SSH configuration and authentication session. Do not sign in, change accounts, or inspect or change VPN, Tailscale, routing, or SSH configuration.
- Keep the target alias, live scheduler data, local paths, and captured output out of tracked files, commits, pull requests, and public reports.
- Stop if validation would require a mutating Slurm command, authentication action, privileged action, or another target.

## Workflow

1. Run local tests and `dry-run` before contacting the target. Build the current tree into a temporary location when the installed binary is not the artifact under test.
2. Run `slurm-monitor doctor TARGET`, then `slurm-monitor --once TARGET`. Keep raw output in a temporary untracked file and check its structure without publishing live rows.
3. For TUI changes, use a bounded `--duration` in a real pseudo-terminal. Test only sizes relevant to the change. Use 72x20 as the default compact test size; add short-wide, narrow, or large-wide cases when the affected layout boundary requires them.
4. Check connection state, terminal restoration, required section visibility, frame and divider continuity, complete CPU and GPU values, spacing, alignment, and accurate clipping metadata. Compare the installed binary only when the task includes installation verification.
5. Test timeout or reconnect behavior only when transport or resilience behavior changed, or when a specific regression requires it.
6. Remove temporary binaries and captures. Report the tested dimensions and outcomes without reproducing private cluster data.

Use `docs/spec.md` as the behavior contract and `docs/security.md` as the disclosure boundary. A live pass supplements the repository test gates; it does not replace them.
