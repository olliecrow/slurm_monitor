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
known_token_regex='((ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|sk-(ant-(oat01|api03)-|proj-)?[A-Za-z0-9_-]{20,}|eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]{20,})'
email_regex='[A-Za-z0-9.!#$%&*+/=?^_{}|~-]+@[A-Za-z0-9][A-Za-z0-9.-]*\.[A-Za-z][A-Za-z]+'
allowed_email_regex='^([^@]+@([A-Za-z0-9-]+\.)*(example\.com|example\.net|example\.org|users\.noreply\.github\.com|noreply\.github\.com)|noreply@github\.com)$'
labelled_phone_regex='([Pp][Hh][Oo][Nn][Ee]|[Mm][Oo][Bb][Ii][Ll][Ee]|[Tt][Ee][Ll]([Ee][Pp][Hh][Oo][Nn][Ee])?)([[:space:]]+[Nn][Uu][Mm][Bb][Ee][Rr])?[[:space:]]*[:=][[:space:]]*[+]?([0-9][[:space:].()-]*){7,}'

# A search tool failure must not look like a clean file. exit is required
# because these helpers run inside if, where a plain command failure is ignored.
fail_scan() {
  echo "error: unable to scan policy input" >&2
  exit 2
}

has_pattern() {
  local pattern="$1"
  local file_path="$2"
  local status=0
  grep -aqE -- "$pattern" "$file_path" || status=$?
  if [[ "$status" -gt 1 ]]; then
    fail_scan
  fi
  [[ "$status" -eq 0 ]]
}

has_disallowed_path() {
  local file_path="$1"
  local status=0
  local matches
  matches="$(grep -aoE -- "$local_path_regex" "$file_path")" || status=$?
  if [[ "$status" -gt 1 ]]; then
    fail_scan
  fi
  [[ -n "$matches" ]] || return 1
  printf '%s\n' "$matches" | grep -qEiv "^${allowed_path_placeholder_regex}$"
}

has_disallowed_email() {
  local file_path="$1"
  local status=0
  local matches
  matches="$(grep -aoE -- "$email_regex" "$file_path")" || status=$?
  if [[ "$status" -gt 1 ]]; then
    fail_scan
  fi
  [[ -n "$matches" ]] || return 1
  printf '%s\n' "$matches" | grep -qEiv "$allowed_email_regex"
}

text_has_disallowed_email() {
  local matches
  matches="$(printf '%s\n' "$1" | grep -Eo "$email_regex" || true)"
  [[ -n "$matches" ]] || return 1
  printf '%s\n' "$matches" | grep -qEiv "$allowed_email_regex"
}

failed=0
for target in "$@"; do
  if [[ ! -e "$target" && ! -L "$target" ]]; then
    echo "error: policy input is not a file" >&2
    exit 2
  fi
  if [[ ! -f "$target" ]]; then
    continue
  fi

  violations=()
  display_target="$target"
  if text_has_disallowed_email "$target"; then
    violations+=("personal email address in file name")
    display_target="<redacted file name>"
  fi
  if has_disallowed_path "$target"; then
    violations+=("user-home path")
  fi
  if has_pattern "$secret_assignment_regex" "$target"; then
    violations+=("credential-like assignment")
  fi
  if has_pattern "$known_token_regex" "$target"; then
    violations+=("known token format")
  fi
  if has_disallowed_email "$target"; then
    violations+=("personal email address")
  fi
  if has_pattern "$labelled_phone_regex" "$target"; then
    violations+=("labelled phone number")
  fi

  if [[ "${#violations[@]}" -gt 0 ]]; then
    violation_list="$(IFS=', '; echo "${violations[*]}")"
    echo "policy violation in ${context}: ${display_target} (${violation_list})" >&2
    failed=1
  fi
done

if [[ "$failed" -ne 0 ]]; then
  cat >&2 <<'EOF'
Blocked by sensitive-text policy.
- Remove or redact secrets and credential-like values.
- Replace personal contact details with a private reporting route or a reserved example value.
- Replace user-home paths with repository-relative paths or placeholders such as /path/to/project.
EOF
fi

exit "$failed"
