#!/usr/bin/env bash
set -euo pipefail

context="git-identity"
if [[ "${1:-}" == --context=* ]]; then
  context="${1#--context=}"
  shift
fi

is_privacy_safe_email() {
  local email
  email="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  case "$email" in
    *@example.com | *@*.example.com | *@example.net | *@*.example.net | *@example.org | *@*.example.org | *@users.noreply.github.com | *@noreply.github.com | noreply@github.com)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

check_email() {
  local label="$1"
  local email="$2"
  if is_privacy_safe_email "$email"; then
    return 0
  fi
  echo "policy violation in ${context}: ${label} uses a personal or unapproved Git email address" >&2
  return 1
}

failed=0
if [[ "${1:-}" == "--current" ]]; then
  if [[ "$#" -ne 1 ]]; then
    echo "usage: check-git-identities.sh [--context=<label>] --current" >&2
    exit 2
  fi
  author_ident="$(git var GIT_AUTHOR_IDENT)"
  committer_ident="$(git var GIT_COMMITTER_IDENT)"
  author_email="${author_ident#*<}"
  author_email="${author_email%%>*}"
  committer_email="${committer_ident#*<}"
  committer_email="${committer_email%%>*}"
  check_email "current author" "$author_email" || failed=1
  check_email "current committer" "$committer_email" || failed=1
elif [[ "$#" -gt 0 ]]; then
  for commit in "$@"; do
    author_email="$(git show -s --format='%ae' "$commit")"
    committer_email="$(git show -s --format='%ce' "$commit")"
    short_commit="$(git rev-parse --short=12 "$commit")"
    check_email "author of commit ${short_commit}" "$author_email" || failed=1
    check_email "committer of commit ${short_commit}" "$committer_email" || failed=1
  done
else
  echo "usage: check-git-identities.sh [--context=<label>] (--current | <commit> [commit...])" >&2
  exit 2
fi

if [[ "$failed" -ne 0 ]]; then
  cat >&2 <<'EOF'
Blocked by Git identity privacy policy.
Configure this repository to use a GitHub-provided no-reply address, then create a new commit.
Existing commits are not changed by this check.
EOF
fi

exit "$failed"
