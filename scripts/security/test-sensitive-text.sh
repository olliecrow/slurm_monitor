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

assert_path_blocked() {
  local content="$1"
  local label="$2"
  printf '%s\n' "$content" >"$tmp_dir/path-case.txt"
  if bash "$checker" --context=policy-selftest "$tmp_dir/path-case.txt" >/dev/null 2>&1; then
    echo "expected ${label} to be blocked" >&2
    exit 1
  fi
}

assert_path_blocked "$(printf '/Users/%s' YOUNG)" "longer user-home name"
assert_path_blocked "$(printf '/home/%s' userbob)" "longer home name"
assert_path_blocked "$(printf '/Users/YOU /Users/%s' example)" "real path beside a placeholder"
assert_path_blocked "$(printf '%s:\\Users\\%s' C example)" "windows profile path"
printf '%s\n' '/Users/YOU' '/home/user' '/home/username' >"$tmp_dir/allowed-paths.txt"
bash "$checker" --context=policy-selftest "$tmp_dir/allowed-paths.txt"
if bash "$checker" --context=policy-selftest "$tmp_dir/does-not-exist" >"$output_file" 2>&1; then
  echo "missing policy input was treated as clean" >&2
  exit 1
fi
assert_blocked_without_exposure "$bad_email_file" "$private_email_fixture" "personal email"
assert_blocked_without_exposure "$bad_phone_file" "$private_phone_fixture" "phone number"
bad_email_name="${tmp_dir}/${private_email_fixture}.txt"
printf 'ordinary text\n' > "$bad_email_name"
assert_blocked_without_exposure "$bad_email_name" "$private_email_fixture" "personal email in file name"

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

# Build synthetic values at runtime so the fixtures cannot resemble real keys.
for prefix in sk-ant-oat01- sk-ant-api03- sk-proj- ghp_ gho_ ghu_ ghs_ ghr_ github_pat_; do
  value="${prefix}$(printf 'a%.0s' {1..80})"
  printf '%s\n' "$value" >"$tmp_dir/token.txt"
  assert_blocked_without_exposure "$tmp_dir/token.txt" "$value" "$prefix"
done
value="eyJ$(printf 'a%.0s' {1..20}).eyJ$(printf 'b%.0s' {1..20}).$(printf 'c%.0s' {1..43})"
printf '%s\n' "$value" >"$tmp_dir/token.txt"
assert_blocked_without_exposure "$tmp_dir/token.txt" "$value" "JWT"

# Real-shaped values: mixed case, digits, and "_" or "-" in the known bodies.
body="$(printf 'Ab3_xY9-Qz%.0s' {1..4})"
alnum="$(printf 'Ab3xY9Qz%.0s' {1..5})"
for value in "sk-ant-oat01-${body}" "x=sk-ant-api03-${body}" "\"sk-proj-${body}\"" "sk-${alnum}" \
  "ghp_${alnum}" "Bearer github_pat_${alnum}_${alnum}" "(AKIA$(printf 'Q7%.0s' {1..8}))"; do
  printf '%s\n' "$value" >"$tmp_dir/token.txt"
  assert_blocked_without_exposure "$tmp_dir/token.txt" "$value" "real-shaped token"
done

# A token prefix inside an ordinary word is not a token.
printf '%s\n' 'class="task-list-item"' 'class="task-list-item-checkbox-v2"' \
  'the risk-assessment-for-new-users doc' 'see desk-booking_service_module' \
  "xxghp_${alnum}" "XAKIA$(printf 'Q7%.0s' {1..8})" >"$tmp_dir/prose.txt"
bash "$checker" --context=policy-selftest "$tmp_dir/prose.txt"

# A block names the file and line, not the value.
value="sk-proj-${body}"
printf 'one\ntwo\n%s\n' "$value" >"$tmp_dir/line.txt"
assert_blocked_without_exposure "$tmp_dir/line.txt" "$value" "line report"
grep -Fq "$tmp_dir/line.txt:3 (known token format)" "$output_file" || {
  echo "block message does not name the file and line" >&2
  exit 1
}
echo "Sensitive text tests passed"
