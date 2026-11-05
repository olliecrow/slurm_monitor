#!/usr/bin/env bash
# Regression checks for secret-scan bypasses. A tool or git failure is not a pass.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
checks="${repo_root}/scripts/security"
push_check="${checks}/check-push-range.sh"
fixture_value="abc123456789XYZ"
root="$(mktemp -d)"
log="${root}/log.txt"
trap 'rm -r -- "${root}"' EXIT

if ! command -v gitleaks >/dev/null 2>&1; then
  echo "gitleaks is required" >&2
  exit 1
fi

reject() {
  local status=0
  "$@" >"$log" 2>&1 || status=$?
  if [[ "$status" -ne 1 ]]; then
    echo "expected rejection status 1, got ${status}" >&2
    exit 1
  fi
}

allow() {
  if ! "$@" >"$log" 2>&1; then
    echo "expected a clean scan" >&2
    exit 1
  fi
}

write_secret() {
  printf '%s=%s\n' token "$fixture_value" >"$1"
}

scan_paths() {
  local config="$1"
  local file="$2"
  gitleaks dir --no-banner --redact --config "$config" "$file"
}

path_dir="${root}/paths"
mkdir -p "$path_dir"
printf '/home/%s/app\n' notuser >"$path_dir/home.txt"
printf '/Users/%s\n' YOUNG >"$path_dir/young.txt"
printf '/home/%s\n' userbob >"$path_dir/userbob.txt"
printf '/Users/YOU /Users/%s\n' example >"$path_dir/mixed.txt"
printf '%s:\\Users\\%s\n' C example >"$path_dir/windows.txt"
printf '/Users/YOU\n/Users/username\n/home/user\n/home/username\n' >"$path_dir/placeholders.txt"
printf '%s:\\Users\\%s\n' C YOU >>"$path_dir/placeholders.txt"

config="$repo_root/.gitleaks.toml"
reject scan_paths "$config" "$path_dir/home.txt"
reject scan_paths "$config" "$path_dir/userbob.txt"
reject scan_paths "$config" "$path_dir/young.txt"
reject scan_paths "$config" "$path_dir/mixed.txt"
reject scan_paths "$config" "$path_dir/windows.txt"
allow scan_paths "$config" "$path_dir/placeholders.txt"

commit_as() {
  git -C "$1" -c user.name=Test -c user.email=test@example.com commit -qm "$2"
}

new_repo() {
  local repo="$1"
  git init -q -b main "$repo"
  git -C "$repo" config commit.gpgsign false
}

push_scan() {
  local repo="$1"
  local from_ref="$2"
  local to_ref="$3"
  (
    cd "$repo"
    PRE_COMMIT_FROM_REF="$from_ref" PRE_COMMIT_TO_REF="$to_ref" bash "$push_check"
  )
}

gitleaks_text() {
  local repo="$1"
  local rev="$2"
  git -C "$repo" -c core.quotepath=false rev-parse --verify "$rev" >/dev/null
  (
    cd "$repo"
    gitleaks git --no-banner --redact --config "$repo_root/.gitleaks.toml" \
      --log-opts="--text --no-textconv --no-ext-diff ${rev}"
  )
}

nul_repo="${root}/nul"
new_repo "$nul_repo"
printf 'clean\n' >"$nul_repo/clean.txt"
git -C "$nul_repo" add clean.txt
commit_as "$nul_repo" base
write_secret "$nul_repo/secret.txt"
printf '\0' >>"$nul_repo/secret.txt"
git -C "$nul_repo" add secret.txt
commit_as "$nul_repo" nul
reject push_scan "$nul_repo" "$(git -C "$nul_repo" rev-parse HEAD^)" "$(git -C "$nul_repo" rev-parse HEAD)"
reject gitleaks_text "$nul_repo" HEAD

attr_repo="${root}/attr"
new_repo "$attr_repo"
printf '* -diff\n' >"$attr_repo/.gitattributes"
printf 'clean\n' >"$attr_repo/clean.txt"
git -C "$attr_repo" add .gitattributes clean.txt
commit_as "$attr_repo" base
write_secret "$attr_repo/secret.txt"
git -C "$attr_repo" add secret.txt
commit_as "$attr_repo" secret
reject push_scan "$attr_repo" "$(git -C "$attr_repo" rev-parse HEAD^)" "$(git -C "$attr_repo" rev-parse HEAD)"
reject gitleaks_text "$attr_repo" HEAD

plus_repo="${root}/plus"
new_repo "$plus_repo"
printf 'clean\n' >"$plus_repo/clean.txt"
git -C "$plus_repo" add clean.txt
commit_as "$plus_repo" base
printf '++%s=%s\n' token "$fixture_value" >"$plus_repo/plus.txt"
git -C "$plus_repo" add plus.txt
commit_as "$plus_repo" plus
reject push_scan "$plus_repo" "$(git -C "$plus_repo" rev-parse HEAD^)" "$(git -C "$plus_repo" rev-parse HEAD)"

merge_repo="${root}/merge"
new_repo "$merge_repo"
printf 'base\n' >"$merge_repo/file.txt"
git -C "$merge_repo" add file.txt
commit_as "$merge_repo" base
git -C "$merge_repo" branch side
printf 'main\n' >"$merge_repo/file.txt"
git -C "$merge_repo" add file.txt
commit_as "$merge_repo" main
git -C "$merge_repo" checkout -q side
printf 'side\n' >"$merge_repo/file.txt"
git -C "$merge_repo" add file.txt
commit_as "$merge_repo" side
git -C "$merge_repo" checkout -q main
git -C "$merge_repo" merge --no-ff --no-commit side >/dev/null 2>&1 || true
write_secret "$merge_repo/file.txt"
git -C "$merge_repo" add file.txt
commit_as "$merge_repo" merge
reject push_scan "$merge_repo" "$(git -C "$merge_repo" rev-parse 'HEAD^1')" "$(git -C "$merge_repo" rev-parse HEAD)"

remote_repo="${root}/remote"
new_repo "$remote_repo"
write_secret "$remote_repo/secret.txt"
git -C "$remote_repo" add secret.txt
commit_as "$remote_repo" secret
printf 'clean\n' >"$remote_repo/clean.txt"
git -C "$remote_repo" add clean.txt
commit_as "$remote_repo" clean
git -C "$remote_repo" update-ref refs/remotes/backup/main HEAD
reject push_scan "$remote_repo" "0000000000000000000000000000000000000000" "$(git -C "$remote_repo" rev-parse HEAD)"
reject push_scan "$remote_repo" "" "$(git -C "$remote_repo" rev-parse HEAD)"

clean_repo="${root}/clean"
new_repo "$clean_repo"
printf 'hello\n' >"$clean_repo/ok.txt"
git -C "$clean_repo" add ok.txt
commit_as "$clean_repo" clean
allow push_scan "$clean_repo" "" "$(git -C "$clean_repo" rev-parse HEAD)"
allow push_scan "$clean_repo" "0000000000000000000000000000000000000000" "$(git -C "$clean_repo" rev-parse HEAD)"

reject push_scan "$clean_repo" "" "1111111111111111111111111111111111111111"
if PRE_COMMIT_FROM_REF="" PRE_COMMIT_TO_REF="" bash "$push_check" >"$log" 2>&1; then
  echo "missing push target was treated as clean" >&2
  exit 1
fi
allow push_scan "$clean_repo" "" "0000000000000000000000000000000000000000"

echo "Scan gap tests passed"
