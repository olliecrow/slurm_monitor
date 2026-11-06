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
# A left boundary keeps a token prefix from matching inside a word such as
# "task-list". Only the known sk- prefixes allow "_" and "-" in the body.
known_token_regex='(^|[^A-Za-z0-9_-])((ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|sk-(ant-(oat01|api03)-[A-Za-z0-9_-]{20,}|proj-[A-Za-z0-9_-]{20,}|[A-Za-z0-9]{20,})|eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]{20,})'
email_regex='[A-Za-z0-9.!#$%&*+/=?^_{}|~-]+@[A-Za-z0-9][A-Za-z0-9.-]*\.[A-Za-z][A-Za-z]+'
allowed_email_regex='([^@]+@([A-Za-z0-9-]+\.)*(example\.com|example\.net|example\.org|users\.noreply\.github\.com|noreply\.github\.com)|noreply@github\.com)'
labelled_phone_regex='([Pp][Hh][Oo][Nn][Ee]|[Mm][Oo][Bb][Ii][Ll][Ee]|[Tt][Ee][Ll]([Ee][Pp][Hh][Oo][Nn][Ee])?)([[:space:]]+[Nn][Uu][Mm][Bb][Ee][Rr])?[[:space:]]*[:=][[:space:]]*[+]?([0-9][[:space:].()-]*){7,}'

# A search tool failure must not look like a clean file.
fail_scan() {
  echo "error: unable to scan policy input" >&2
  exit 2
}

# The helpers below print only line numbers, never matched text.
# Status 2 means the scan failed.
match_lines() {
  local pattern="$1"
  local file_path="$2"
  local status=0
  grep -anE -- "$pattern" "$file_path" | cut -d: -f1 || status=$?
  [[ "$status" -le 1 ]] || return 2
}

# Print lines with a match that is not exactly an allowed value.
disallowed_match_lines() {
  local pattern="$1"
  local allowed="$2"
  local file_path="$3"
  local status=0
  local matches
  matches="$(grep -anoE -- "$pattern" "$file_path")" || status=$?
  [[ "$status" -le 1 ]] || return 2
  [[ -n "$matches" ]] || return 0
  status=0
  printf '%s\n' "$matches" | grep -Eiv -- "^[0-9]+:${allowed}$" | cut -d: -f1 || status=$?
  [[ "$status" -le 1 ]] || return 2
}

text_has_disallowed_email() {
  local matches
  matches="$(printf '%s\n' "$1" | grep -Eo "$email_regex" || true)"
  [[ -n "$matches" ]] || return 1
  printf '%s\n' "$matches" | grep -qEiv "^${allowed_email_regex}$"
}

failed=0
report() {
  local label="$1"
  local lines="$2"
  [[ -n "$lines" ]] || return 0
  lines="$(printf '%s\n' "$lines" | sort -un | paste -sd, -)" || fail_scan
  echo "policy violation in ${context}: ${display_target}:${lines} (${label})" >&2
  failed=1
}

for target in "$@"; do
  if [[ ! -e "$target" && ! -L "$target" ]]; then
    echo "error: policy input is not a file" >&2
    exit 2
  fi
  if [[ ! -f "$target" ]]; then
    continue
  fi

  display_target="$target"
  if text_has_disallowed_email "$target"; then
    display_target="<redacted file name>"
    echo "policy violation in ${context}: ${display_target} (personal email address in file name)" >&2
    failed=1
  fi
  lines="$(disallowed_match_lines "$local_path_regex" "$allowed_path_placeholder_regex" "$target")" || fail_scan
  report "user-home path" "$lines"
  lines="$(match_lines "$secret_assignment_regex" "$target")" || fail_scan
  report "credential-like assignment" "$lines"
  lines="$(match_lines "$known_token_regex" "$target")" || fail_scan
  report "known token format" "$lines"
  lines="$(disallowed_match_lines "$email_regex" "$allowed_email_regex" "$target")" || fail_scan
  report "personal email address" "$lines"
  lines="$(match_lines "$labelled_phone_regex" "$target")" || fail_scan
  report "labelled phone number" "$lines"
done

if [[ "$failed" -ne 0 ]]; then
  cat >&2 <<'EOF2'
Blocked by sensitive-text policy. Matched values are omitted.
- Remove or redact secrets and credential-like values.
- Replace personal contact details with a private reporting route or a reserved example value.
- Replace user-home paths with repository-relative paths or placeholders such as /path/to/project.
EOF2
fi

exit "$failed"
