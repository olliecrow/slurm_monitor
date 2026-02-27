#!/usr/bin/env bash
set -euo pipefail

from_ref="${PRE_COMMIT_FROM_REF:-}"
to_ref="${PRE_COMMIT_TO_REF:-}"

if [[ -z "${to_ref}" || "${to_ref}" == "0000000000000000000000000000000000000000" ]]; then
  exit 0
fi

zero_ref="0000000000000000000000000000000000000000"
commits=()
if [[ -z "${from_ref}" || "${from_ref}" == "${zero_ref}" ]]; then
  while IFS= read -r commit; do
    [[ -n "${commit}" ]] && commits+=("${commit}")
  done < <(git rev-list "${to_ref}" --not --remotes)
else
  while IFS= read -r commit; do
    [[ -n "${commit}" ]] && commits+=("${commit}")
  done < <(git rev-list "${from_ref}..${to_ref}")
fi

if [[ "${#commits[@]}" -eq 0 ]]; then
  exit 0
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

commit_msg_file="${tmp_dir}/commit_messages.txt"
patch_file="${tmp_dir}/patches.diff"

git log --format='%H%n%s%n%b%n' "${commits[@]}" > "${commit_msg_file}"
git show --format= --patch --no-color "${commits[@]}" > "${patch_file}"

bash scripts/security/check-sensitive-text.sh --context=push-commit-message "${commit_msg_file}"
bash scripts/security/check-sensitive-text.sh --context=push-diff "${patch_file}"
