#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
checker="${repo_root}/scripts/security/check-sensitive-text.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

ok_file="${tmp_dir}/ok.txt"
bad_secret_file="${tmp_dir}/bad-secret.txt"
bad_path_file="${tmp_dir}/bad-path.txt"
output_file="${tmp_dir}/output.txt"
fixture_value="abc123456789XYZ"

printf 'docs: keep /Users/YOU and /home/user as placeholders\n' > "$ok_file"
printf 'fix: %s=%s\n' "token" "$fixture_value" > "$bad_secret_file"
printf 'placeholder /Users/YOU does not excuse /Users/%s\n' "example" > "$bad_path_file"

bash "$checker" --context=policy-selftest "$ok_file"

if bash "$checker" --context=policy-selftest "$bad_secret_file" >"$output_file" 2>&1; then
  echo "expected secret policy self-test failure did not occur" >&2
  exit 1
fi
if grep -Fq "$fixture_value" "$output_file"; then
  echo "sensitive-text checker exposed matched content" >&2
  exit 1
fi
if bash "$checker" --context=policy-selftest "$bad_path_file" >/dev/null 2>&1; then
  echo "placeholder incorrectly allowed a real path on the same line" >&2
  exit 1
fi
if PATH="/usr/bin:/bin" bash "$checker" --context=policy-fallback-selftest "$bad_secret_file" >/dev/null 2>&1; then
  echo "fallback policy self-test failure did not occur" >&2
  exit 1
fi
