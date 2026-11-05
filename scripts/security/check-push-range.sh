#!/usr/bin/env bash
# Scan commit messages and added lines in a push range.
# A git failure or a policy hit exits non-zero. An error never counts as clean.
set -euo pipefail

zero_ref="0000000000000000000000000000000000000000"
sha_regex='^[0-9a-fA-F]{40}$|^[0-9a-fA-F]{64}$'
from_ref="${PRE_COMMIT_FROM_REF:-}"
to_ref="${PRE_COMMIT_TO_REF:-}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

die() {
  echo "error: $*" >&2
  exit 1
}

if [[ -z "$to_ref" ]]; then
  die "no push target given"
fi
# Deleting a remote ref sends no commits.
if [[ "$to_ref" == "$zero_ref" ]]; then
  exit 0
fi
[[ "$to_ref" =~ $sha_regex ]] || die "push target is not a full commit id"

if [[ -n "$from_ref" && "$from_ref" != "$zero_ref" ]]; then
  [[ "$from_ref" =~ $sha_regex ]] || die "push base is not a full commit id"
  commit_list="$(git rev-list "${from_ref}..${to_ref}")" || die "unable to list commits in the push range"
else
  # A missing or all-zero base still scans every commit reachable from the target.
  commit_list="$(git rev-list "${to_ref}")" || die "unable to list commits in the push range"
fi

[[ -n "$commit_list" ]] || exit 0

commits=()
while IFS= read -r commit; do
  [[ -n "$commit" ]] || continue
  [[ "$commit" =~ $sha_regex ]] || die "unexpected commit id in the push range"
  commits+=("$commit")
done <<< "$commit_list"

tmp_dir="$(mktemp -d)"
trap 'rm -r -- "${tmp_dir}"' EXIT

commit_msg_file="${tmp_dir}/commit_messages.txt"
added_lines_file="${tmp_dir}/added_lines.txt"

printf '%s\n' "$commit_list" |
  git log --stdin --no-walk=unsorted --format='%H%n%s%n%b%n' >"$commit_msg_file" ||
  die "unable to read commit messages"

# Force a text patch so a trailing NUL or a -diff attribute cannot hide a
# line. Compare merge commits with their first parent, and keep an added
# line that itself starts with "+".
printf '%s\n' "$commit_list" |
  git log --stdin --no-walk=unsorted -m --first-parent --format= --patch \
    --text --no-color --unified=0 --no-ext-diff --no-textconv |
  awk '/^diff / { in_hunk = 0; next } /^@@ / { in_hunk = 1; next } in_hunk && /^\+/ { sub(/^\+/, ""); print }' \
    >"$added_lines_file" || die "unable to read added lines"

bash "${script_dir}/check-git-identities.sh" --context=push-git-identity "${commits[@]}"
bash "${script_dir}/check-sensitive-text.sh" --context=push-commit-message "$commit_msg_file"
bash "${script_dir}/check-sensitive-text.sh" --context=push-added-lines "$added_lines_file"
