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
- Include concise verification evidence in PRs/issues when practical.

## Collaboration Preferences

- Preserve accurate author/committer attribution for each contributor.
- Prefer commit author identities tied to genuine human GitHub accounts, not fabricated bot names/emails.
- Avoid destructive history rewrites unless required for secret/confidentiality remediation.
