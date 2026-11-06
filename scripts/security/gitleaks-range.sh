#!/usr/bin/env bash
# Check git and scanner output: gitleaks can exit zero after a git failure.
set -euo pipefail
if [[ "$#" -ne 2 || ! "$1" =~ ^[A-Za-z0-9_./@{}^~-]+$ || "$1" == -* ]]; then
  echo "usage: gitleaks-range.sh <revision-or-range> <config>" >&2
  exit 1
fi
range="$1"
expected="$(git rev-list --count --end-of-options "$range" --)" || {
  echo "unable to list scan commits" >&2
  exit 1
}
[[ "$expected" =~ ^[0-9]+$ ]] || exit 1
if [[ "$expected" -eq 0 ]]; then
  echo "empty revision set; no commits to scan"
  exit 0
fi
log="$(mktemp)"
trap 'rm -- "$log"' EXIT
status=0
# --verbose shows the rule, file, and commit of a finding; --redact hides its value.
NO_COLOR=1 gitleaks git --no-banner --redact --verbose --log-level info --config "$2" \
  --log-opts="--text --no-textconv --no-ext-diff ${range}" >"$log" 2>&1 || status=$?
cat -- "$log"
if [[ "$status" -ne 0 ]]; then
  echo "gitleaks failed (status ${status})" >&2
  exit "$status"
fi
if grep -Eq '(^|[[:space:]])(ERR|FTL)([[:space:]]|$)' "$log"; then
  echo "gitleaks logged an error" >&2
  exit 1
fi
if ! grep -Eq '(^|[[:space:]])[1-9][0-9]* commits? scanned\.' "$log"; then
  echo "gitleaks did not confirm scanned commits" >&2
  exit 1
fi
echo "gitleaks scanned commits without leaks"
