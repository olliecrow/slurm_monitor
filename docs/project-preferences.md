# Project Preferences (Going Forward)

These preferences define how `slurm_monitor` should be maintained as an open-source-ready project.

## Quality and Scope

- Keep monitoring strictly read-only.
- Keep data collection and rendering resilient under transient failures.
- Keep UI/CLI behavior explicit and predictable.

## Security and Confidentiality

- Never commit secrets, credentials, tokens, API keys, or private key material.
- Never commit private/sensitive machine paths; use placeholders such as `/path/to/project`, `/Users/YOU`, `/home/user`, or `C:\\Users\\USERNAME`.
- Keep local runtime state untracked (`.env*`, SSH/private artifacts, temp files).
- If sensitive data is found in history, rotate credentials and scrub history before publication.

## Documentation Expectations

- Keep `README.md`, `AGENTS.md`, and `docs/` aligned with real behavior.
- Keep security/auth assumptions documented and up to date.

## Verification Expectations

- Verify key paths in both local and remote modes when behavior changes.
- Run `slurm-monitor doctor` and `slurm-monitor dry-run` as first-line checks before live monitor sessions.
- Include concise verification evidence in PRs/issues when practical.

## Collaboration Preferences

- Preserve accurate author/committer attribution for each contributor.
- Prefer commit author identities tied to genuine human GitHub accounts, not fabricated bot names/emails.
- Avoid destructive history rewrites unless required for secret/confidentiality remediation.

## Language and Naming

- Use plain English in chat, docs, notes, comments, reports, commit messages, issue text, and review text.
- Prefer short words, short sentences, and direct statements.
- If a technical term is needed, explain it in simple words the first time.
- In code, prefer clear descriptive names over clever or vague names.
- Rename confusing names when the change is low risk and clearly improves readability.
