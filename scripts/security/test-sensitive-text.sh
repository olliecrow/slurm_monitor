#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
checker="${repo_root}/scripts/security/check-sensitive-text.sh"
identity_checker="${repo_root}/scripts/security/check-git-identities.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

ok_file="${tmp_dir}/ok.txt"
bad_secret_file="${tmp_dir}/bad-secret.txt"
bad_path_file="${tmp_dir}/bad-path.txt"
bad_email_file="${tmp_dir}/bad-email.txt"
bad_phone_file="${tmp_dir}/bad-phone.txt"
output_file="${tmp_dir}/output.txt"
fixture_value="abc123456789XYZ"
private_email_fixture="contributor@private."
private_email_fixture+="test"
private_phone_fixture="+44 7700 "
private_phone_fixture+="900 123"

printf 'docs: keep /Users/YOU, /home/user, user@cluster.example.org, and 12345678 as examples\n' > "$ok_file"
printf 'fix: %s=%s\n' "token" "$fixture_value" > "$bad_secret_file"
printf 'placeholder /Users/YOU does not excuse /Users/%s\n' "example" > "$bad_path_file"
printf 'contact: %s\n' "$private_email_fixture" > "$bad_email_file"
printf 'phone: %s\n' "$private_phone_fixture" > "$bad_phone_file"

bash "$checker" --context=policy-selftest "$ok_file"

assert_blocked_without_exposure() {
  local bad_file="$1"
  local private_value="$2"
  local label="$3"
  if bash "$checker" --context=policy-selftest "$bad_file" >"$output_file" 2>&1; then
    echo "expected ${label} policy self-test failure did not occur" >&2
    exit 1
  fi
  if grep -Fq "$private_value" "$output_file"; then
    echo "sensitive-text checker exposed ${label} content" >&2
    exit 1
  fi
}

assert_blocked_without_exposure "$bad_secret_file" "$fixture_value" "secret"
if bash "$checker" --context=policy-selftest "$bad_path_file" >/dev/null 2>&1; then
  echo "placeholder incorrectly allowed a real path on the same line" >&2
  exit 1
fi
if PATH="/usr/bin:/bin" bash "$checker" --context=policy-fallback-selftest "$bad_secret_file" >/dev/null 2>&1; then
  echo "fallback policy self-test failure did not occur" >&2
  exit 1
fi
assert_blocked_without_exposure "$bad_email_file" "$private_email_fixture" "personal email"
assert_blocked_without_exposure "$bad_phone_file" "$private_phone_fixture" "phone number"
bad_email_name="${tmp_dir}/${private_email_fixture}.txt"
printf 'ordinary text\n' > "$bad_email_name"
assert_blocked_without_exposure "$bad_email_name" "$private_email_fixture" "personal email in file name"
if PATH="/usr/bin:/bin" bash "$checker" --context=policy-fallback-selftest "$bad_email_file" >/dev/null 2>&1; then
  echo "fallback email policy self-test failure did not occur" >&2
  exit 1
fi

GIT_AUTHOR_NAME="Example Contributor" GIT_AUTHOR_EMAIL="12345+contributor@users.noreply.github.com" \
  GIT_COMMITTER_NAME="Example Contributor" GIT_COMMITTER_EMAIL="12345+contributor@users.noreply.github.com" \
  bash "$identity_checker" --context=identity-selftest --current
if GIT_AUTHOR_NAME="Example Contributor" GIT_AUTHOR_EMAIL="$private_email_fixture" \
  GIT_COMMITTER_NAME="Example Contributor" GIT_COMMITTER_EMAIL="$private_email_fixture" \
  bash "$identity_checker" --context=identity-selftest --current >"$output_file" 2>&1; then
  echo "expected Git identity policy self-test failure did not occur" >&2
  exit 1
fi
if grep -Fq "$private_email_fixture" "$output_file"; then
  echo "Git identity checker exposed a private address" >&2
  exit 1
fi
