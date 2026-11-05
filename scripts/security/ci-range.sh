#!/usr/bin/env bash
set -euo pipefail
if [[ "${GITHUB_EVENT_NAME}" == pull_request ]]; then
  printf '%s..%s\n' "${PR_BASE_SHA}" "${PR_HEAD_SHA}"
elif [[ -n "${PUSH_BEFORE_SHA:-}" && ! "${PUSH_BEFORE_SHA}" =~ ^0+$ ]] && git cat-file -e "${PUSH_BEFORE_SHA}^{commit}" 2>/dev/null; then
  printf '%s..%s\n' "${PUSH_BEFORE_SHA}" "${GITHUB_SHA}"
elif [[ "${GITHUB_REF_NAME}" != main ]] && base=$(git merge-base origin/main "${GITHUB_SHA}"); then
  printf '%s..%s\n' "${base}" "${GITHUB_SHA}"
else
  # Missing push bases (including rewritten history) require a full history scan.
  printf '%s\n' "${GITHUB_SHA}"
fi
