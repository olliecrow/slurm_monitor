# Implementation Clarifications

This file is a compact index of resolved defaults from requirement discussions.
Despite the filename, it intentionally tracks resolved defaults only; unresolved investigation items belong in `plan/current/notes.md` until they are resolved and promoted.

Canonical behavior lives in:
- [`spec.md`](spec.md) for runtime/CLI contracts
- [`decisions.md`](decisions.md) for rationale and trade-offs

## Resolved defaults index
- Language: Go.
  - Canonical: [`decisions.md`](decisions.md)
- SSH authentication and targets: SSH config aliases and `user@host` are supported; no password flags.
  - Canonical: [`spec.md` (CLI Contract, Remote Resilience Contract)](spec.md)
- Slurm availability checks: missing commands are fail-fast; transient transport failures retry with backoff.
  - Canonical: [`spec.md` (Startup Behavior)](spec.md)
- UI views/layout: node summary plus combined queue panel (queue summary + per-user section).
  - Canonical: [`spec.md` (Runtime Data Contract, TUI Behavior)](spec.md)
- Safety scope: monitor remains strictly read-only.
  - Canonical: [`spec.md` (Safety Constraint)](spec.md)
- Operational defaults: refresh interval, utilization fallbacks, array task counting, and CLI help behavior.
  - Canonical: [`spec.md` (Core flags, Runtime Data Contract, CLI Contract)](spec.md)
