# Security and Secret-Handling Policy

## Core rules
- Never commit secrets, credentials, passwords, tokens, private keys, or confidential internal data.
- Never commit personal email addresses or labelled phone numbers. Use reserved example domains in documentation and GitHub-provided no-reply addresses for Git identities.
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
- Transport errors omit executed commands and captured stdout.
- SSH targets and stderr remain visible locally for diagnosis; review them before sharing terminal output.

## Development safeguards
- Do not hardcode hostnames, users, ports, keys, or tokens in committed code.
- Keep local test targets in ignored files or local shell environment.
- Review diffs for accidental secret leakage before commit.
- Repo-enforced checks:
  - `.pre-commit-config.yaml` runs `gitleaks` before commits.
  - Local hooks block personal contact details, local absolute system paths, and credential-like values in staged files and commit messages.
  - The commit hook requires a GitHub no-reply or reserved example address for Git author and committer metadata.
  - The pre-push hook scans each outbound commit identity, message, and added line for the same patterns.
  - `.github/workflows/ci.yml` applies the same checks to the exact push or pull request range and checks pull request title/body text.
  - Policy self-tests verify allowed examples, personal-data detection, and redaction of detected content.
  - Sensitive-text checks use portable extended `grep` expressions on macOS and Linux.

## Runtime safety posture
- Monitor is strictly read-only.
- Command execution allowlist remains Slurm read commands and basic shell wrappers only.
- No destructive cluster actions are executed by this tool.
