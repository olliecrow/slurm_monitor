#!/usr/bin/env bash
set -euo pipefail
repo_root="$(git rev-parse --show-toplevel)"
wrapper="${1:-$repo_root/scripts/security/gitleaks-range.sh}"
root="$(mktemp -d)"
trap 'rm -r -- "$root"' EXIT
mkdir "$root/repo" "$root/bin"
git -C "$root/repo" init -q -b main
git -C "$root/repo" config commit.gpgsign false
printf 'clean\n' >"$root/repo/file"
git -C "$root/repo" add file
git -C "$root/repo" -c user.name=Test -c user.email=test@example.com commit -qm base
scan() {
  (cd "$root/repo"; bash "$wrapper" "$1" "$repo_root/.gitleaks.toml") >"$root/log" 2>&1
}
reject() {
  if scan "$1"; then
    echo "accepted invalid scan: $2" >&2
    exit 1
  fi
}
scan HEAD
scan HEAD..HEAD
reject missing-ref..HEAD "invalid range"
reject 'HEAD ' "whitespace"
# A real finding must show the rule, file, and commit, but not the value.
leak_name="notuser"
printf '/home/%s/notes\n' "$leak_name" >"$root/repo/path.txt"
git -C "$root/repo" add path.txt
git -C "$root/repo" -c user.name=Test -c user.email=test@example.com commit -qm leak
reject HEAD "home path"
for field in RuleID: File: Commit:; do
  grep -q "^${field}" "$root/log" || { echo "gitleaks output lacks ${field}" >&2; exit 1; }
done
if grep -Fq "$leak_name" "$root/log"; then
  echo "gitleaks output exposed the finding" >&2
  exit 1
fi
cat >"$root/bin/gitleaks" <<'MOCK'
#!/usr/bin/env bash
case "$SCAN_MODE" in
 fatal) echo 'FTL simulated error'; echo '1 commits scanned.';;
 error) echo 'ERR [git] fatal: simulated error'; echo '1 commits scanned.';;
 zero) echo '0 commits scanned.'; echo 'no leaks found';;
 missing) echo 'no leaks found';;
 status) exit 42;;
esac
MOCK
chmod +x "$root/bin/gitleaks"
export PATH="$root/bin:$PATH"
for SCAN_MODE in error fatal zero missing; do
  export SCAN_MODE
  reject HEAD "$SCAN_MODE"
done
export SCAN_MODE=status
status=0
scan HEAD || status=$?
[[ "$status" -eq 42 ]] || { echo "scanner exit status lost" >&2; exit 1; }
echo "Gitleaks range tests passed"
