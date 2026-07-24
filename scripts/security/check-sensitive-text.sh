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

has_pattern() {
  local pattern="$1"
  local file_path="$2"
  if command -v rg >/dev/null 2>&1; then
    rg --quiet --no-messages -e "$pattern" "$file_path"
  else
    grep -qE "$pattern" "$file_path"
  fi
}

has_disallowed_path() {
  local file_path="$1"
  local matches
  if command -v rg >/dev/null 2>&1; then
    matches="$(rg --only-matching --no-filename --no-line-number -e "$local_path_regex" "$file_path" || true)"
  else
    matches="$(grep -Eo "$local_path_regex" "$file_path" || true)"
  fi
  [[ -n "$matches" ]] || return 1
  printf '%s\n' "$matches" | grep -qEiv "^${allowed_path_placeholder_regex}$"
}

failed=0
for target in "$@"; do
  if [[ ! -f "$target" ]]; then
    continue
  fi

  violations=()
  if has_disallowed_path "$target"; then
    violations+=("local absolute path")
  fi
  if has_pattern "$secret_assignment_regex" "$target"; then
    violations+=("credential-like assignment")
  fi
  if has_pattern "$known_token_regex" "$target"; then
    violations+=("known token format")
  fi

  if [[ "${#violations[@]}" -gt 0 ]]; then
    violation_list="$(IFS=', '; echo "${violations[*]}")"
    echo "policy violation in ${context}: ${target} (${violation_list})" >&2
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
