# Docs Directory

This directory holds long-term, agent-focused documentation for this repo. It is not intended for human readers and is committed to git.

## Index
- [`spec.md`](spec.md): canonical product and runtime behavior contract.
- [`architecture.md`](architecture.md): component boundaries, transport model, and resilience design.
- [`implementation-plan.md`](implementation-plan.md): phased implementation with verification and battle testing.
- [`alignment.md`](alignment.md): requirement traceability from user requirements to implementation/testing.
- [`security.md`](security.md): secret handling and authentication policy.
- [`open-questions.md`](open-questions.md): implementation clarifications and chosen defaults.
- [`decisions.md`](decisions.md): durable rationale and decision records.
- [`workflows.md`](workflows.md): note routing and operating workflow conventions.

Principles:
- Keep content evergreen and aligned with the codebase.
- Avoid time- or date-dependent language.
- Prefer updating existing docs when they have a clear home, but do not hesitate to create new focused docs and nested subdirectories when it improves organization and findability.
- Use docs for cross-cutting context or rationale that does not belong in code comments or tests.
- Keep entries concise and high-signal.
- Make docs interrelate: use relative links between related docs and avoid orphan docs by linking new docs from an index or a nearby "parent" doc.

Relationship to `/plan/`:
- `/plan/` is a short-term, disposable scratch space for agents and is not committed to git.
- `/plan/handoffs/` is used for sequential workflow handoffs between automation scripts when needed.
- Active notes should be routed into `/plan/current/` and promoted into `/docs/` only when they become durable guidance.
- `/docs/` is long-lived; only stable guidance should live here.
