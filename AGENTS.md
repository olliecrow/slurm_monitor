# Repository Guidelines

## Project Overview (slurm_monitor)
- `slurm_monitor` is a CLI + TUI monitor for Slurm clusters.
- Primary use case is robust remote monitoring over SSH from unreliable networks.
- Secondary use case is local monitoring on hosts that already have Slurm CLI tooling.
- Monitoring is strictly read-only; queue mutation actions are out of scope.

## Open-Source Transition Posture
- Treat this repository as open-source-ready now, even while private.
- Never commit secrets, credentials, tokens, private keys, passwords, or confidential internal details.
- Keep auth material in local environment/secret stores or SSH agent/config only.
- Assume docs and logs may become public; redact sensitive details by default.

## Docs, Plans, and Decisions (agent usage)
- `docs/` is long-lived and committed (and may use nested directories + cross-links to stay organized).
- `plan/` is short-lived scratch space and is not committed.
- Decision capture policy lives in `docs/decisions.md`.
- Operating workflow conventions live in `docs/workflows.md`.
- Canonical runtime behavior lives in `docs/spec.md`.
- System architecture lives in `docs/architecture.md`.
- Implementation and validation planning lives in `docs/implementation-plan.md`.
- Requirement traceability lives in `docs/alignment.md`.
- Security and credential-handling policy lives in `docs/security.md`.

## Note Routing (agent usage)
- Active notes go in `plan/current/notes.md`.
- Multi-workstream index goes in `plan/current/notes-index.md`.
- Orchestration status goes in `plan/current/orchestrator-status.md`.

## Plan Directory Structure (agent usage)
- `plan/current/`
- `plan/backlog/`
- `plan/complete/`
- `plan/experiments/`
- `plan/artifacts/`
- `plan/scratch/`
- `plan/handoffs/`

## Dictation-Aware Input Handling
- The user often dictates prompts, so minor transcription errors and homophone substitutions are expected.
- Infer intent from local context and repository state; ask a concise clarification only when ambiguity changes execution risk.
- Keep explicit typo dictionaries at workspace level (do not duplicate repo-local typo maps).
