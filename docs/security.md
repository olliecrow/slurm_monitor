# Security and Secret-Handling Policy

## Core rules
- Never commit secrets, credentials, passwords, tokens, private keys, or confidential internal data.
- Treat this repository as open-source-ready.
- Keep authentication material in SSH config, SSH agent, environment, or local secret stores only.

## SSH/auth policy
- Supported auth inputs:
  - SSH config alias target
  - `user@host` target
  - optional identity file flag
- Preferred auth mechanism:
  - SSH keys with agent forwarding/loading where needed.
- Not supported by default:
  - password passed via CLI flag.

## Logging policy
- Do not log sensitive credential material.
- Keep error messages actionable but avoid exposing full sensitive command payloads.
- If target strings contain sensitive fragments, redact before writing logs.

## Development safeguards
- Do not hardcode hostnames, users, ports, keys, or tokens in committed code.
- Keep local test targets in ignored files or local shell environment.
- Review diffs for accidental secret leakage before commit.
- Repo-enforced checks:
  - `.pre-commit-config.yaml` runs `gitleaks` before commits.
  - `commit-msg` hook blocks local absolute system paths and credential-like values in commit messages.
  - `pre-push` hook scans outbound commit messages and outbound diffs for the same sensitive patterns.
  - `.github/workflows/security-policy.yml` re-checks git history, commit messages, and PR title/body in CI.
  - Sensitive-text checks support both `rg` and `grep` so local/CI environments without `rg` still enforce policy.
  - Branch protection blocks force-push and branch deletion on `main`; direct personal pushes to `main` are allowed by project preference.
  - GitHub Secret Scanning and Push Protection are enabled for server-side secret detection.

## Runtime safety posture
- Monitor is strictly read-only.
- Command execution allowlist remains Slurm read commands and basic shell wrappers only.
- No destructive cluster actions are executed by this tool.
