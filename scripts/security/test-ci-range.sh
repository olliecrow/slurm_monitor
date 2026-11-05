#!/usr/bin/env bash
set -euo pipefail
script="$(pwd)/scripts/security/ci-range.sh"
root=$(mktemp -d)
trap 'rm -r "$root"' EXIT
git -C "$root" init -q
git -C "$root" -c user.name=Test -c user.email=test@users.noreply.github.com commit -q --allow-empty -m base
base=$(git -C "$root" rev-parse HEAD)
git -C "$root" update-ref refs/remotes/origin/main "$base"
git -C "$root" -c user.name=Test -c user.email=test@users.noreply.github.com commit -q --allow-empty -m first
git -C "$root" -c user.name=Test -c user.email=test@users.noreply.github.com commit -q --allow-empty -m second
export GITHUB_SHA=$(git -C "$root" rev-parse HEAD)
export GITHUB_EVENT_NAME=push GITHUB_REF_NAME=olliecrow/ci-staging
cd "$root"
for PUSH_BEFORE_SHA in '' 0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 "$base"; do
  export PUSH_BEFORE_SHA
  range=$(bash "$script")
  test "$(git rev-list --count "$range")" = 2
done
export GITHUB_REF_NAME=main PUSH_BEFORE_SHA=1111111111111111111111111111111111111111
test "$(bash "$script")" = "$GITHUB_SHA"
echo 'CI range tests passed'
