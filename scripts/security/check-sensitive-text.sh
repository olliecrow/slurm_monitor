#!/usr/bin/env bash
set -euo pipefail

context="text"
if [[ "${1:-}" == --context=* ]]; then
  context="${1#--context=}"
  shift
fi

if [[ "$#" -lt 1 ]]; then
  echo "usage: check-sensitive-text.sh [--context=<label>] <file> [file...]" >&2
  exit 2
fi

local_path_regex='(/Users/[A-Za-z0-9._-]+|/home/[A-Za-z0-9._-]+|[A-Za-z]:\\+Users\\+[A-Za-z0-9._-]+)'
allowed_path_placeholder_regex='(/Users/(YOU|USER|username)|/home/(user|USER|username)|[A-Za-z]:\\+Users\\+(YOU|USER|USERNAME|username))'
secret_assignment_regex='([Aa][Pp][Ii][_-]?[Kk][Ee][Yy]|[Tt][Oo][Kk][Ee][Nn]|[Pp][Aa][Ss][Ss][Ww][Oo][Rr][Dd]|[Ss][Ee][Cc][Rr][Ee][Tt])[[:space:]]*[:=][[:space:]]*["'"'"']?[A-Za-z0-9_./+=-]{12,}'
known_token_regex='((ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9]{20,})'

search_pattern() {
  local pattern="$1"
  local file_path="$2"
  if command -v rg >/dev/null 2>&1; then
    rg --line-number --no-heading --color never -e "$pattern" "$file_path" || true
  else
    grep -nE "$pattern" "$file_path" || true
  fi
}

exclude_pattern() {
  local pattern="$1"
  if command -v rg >/dev/null 2>&1; then
    rg --invert-match --ignore-case --no-heading -e "$pattern" || true
  else
    grep -viE "$pattern" || true
  fi
}

failed=0
for target in "$@"; do
  if [[ ! -f "$target" ]]; then
    continue
  fi

  path_matches="$(search_pattern "$local_path_regex" "$target")"
  if [[ -n "$path_matches" ]]; then
    path_matches="$(printf '%s\n' "$path_matches" | exclude_pattern "$allowed_path_placeholder_regex")"
  fi

  secret_assignment_matches="$(search_pattern "$secret_assignment_regex" "$target")"
  known_token_matches="$(search_pattern "$known_token_regex" "$target")"

  matches="$(printf '%s\n%s\n%s\n' "$path_matches" "$secret_assignment_matches" "$known_token_matches" | sed '/^$/d' | sort -u)"
  if [[ -n "$matches" ]]; then
    echo "policy violation in ${context}: ${target}" >&2
    echo "$matches" >&2
    failed=1
  fi
done

if [[ "$failed" -ne 0 ]]; then
  cat >&2 <<'EOF'
Blocked by sensitive-text policy.
- Remove or redact secrets and credential-like values.
- Replace local absolute paths with repo-relative paths or placeholders like /path/to/project.
EOF
fi

exit "$failed"
