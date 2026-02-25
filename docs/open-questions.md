# Implementation Clarifications

This file captures clarified defaults from requirement discussions so implementation can proceed without re-opening settled points.

## Confirmed defaults

## Language
- Implementation language is Go.

## SSH authentication and targets
- Support standard SSH usage patterns:
  - SSH config aliases,
  - explicit `user@host` targets,
  - local mode when no target is passed.
- Support bastion/relay behavior through existing SSH config (`ProxyJump` and related directives).
- Do not add password CLI flags.

## Slurm availability checks
- If local mode is selected and Slurm CLI tools are not available, exit with a clear error.
- If remote mode is selected and the remote destination does not have Slurm CLI tools, exit with a clear error.
- If startup failures are transient (SSH/network/timeouts), keep retrying with backoff until operator quit (or until `--duration` deadline if set).

## UI data views
- The TUI provides three primary data views:
  - node summary with aggregate totals,
  - queue summary,
  - per-user view.
- Rendering groups these into two vertical panels:
  - node summary panel,
  - combined queue panel (queue summary section + per-user section).

## Safety scope
- The monitor is strictly read-only.
- The tool never submits mutating operations to Slurm.

## Chosen operational defaults
- Default refresh interval: `2s` (overridable by `--refresh`).
- Utilization fields use Slurm-reported data when available, otherwise display `n/a`.
- Queue and user counts include Slurm job arrays at array-task granularity.
- CLI help (`-h`/`--help`) is self-contained and includes behavior/auth/examples, and parse errors point to help.
