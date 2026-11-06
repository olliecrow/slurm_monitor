#!/usr/bin/env bash
set -euo pipefail
repo_root="$(git rev-parse --show-toplevel)"
wrapper="${1:-$repo_root/scripts/security/gitleaks-range.sh}"
root="$(mktemp -d)"
trap 'ls -ld -- "$root"; rm -r -- "$root"' EXIT
mkdir "$root/bin"
cat >"$root/bin/gitleaks" <<'MOCK'
#!/usr/bin/env bash
echo '0 commits scanned.'
echo 'no leaks found'
MOCK
chmod +x "$root/bin/gitleaks"
commit() {
  git -C "$root/repo" add -A
  git -C "$root/repo" -c user.name=Test -c user.email=test@example.com commit -qm fixture --allow-empty
}
scan() {
  (cd "$root/repo"; bash "$wrapper" "$1" "$repo_root/.gitleaks.toml") >"$root/log" 2>&1
}
for kind in deletion rename mode empty binary attribute cancelled merge; do
  [[ -z "${TEST_CASE:-}" || "$TEST_CASE" == "$kind" ]] || continue
  mkdir "$root/repo"
  git -C "$root/repo" init -q -b main
  git -C "$root/repo" config commit.gpgsign false
  git -C "$root/repo" config core.fileMode true
  printf 'clean\n' >"$root/repo/file"
  commit
  base="$(git -C "$root/repo" rev-parse HEAD)"
  case "$kind" in
    deletion) git -C "$root/repo" rm -q file ;;
    rename) git -C "$root/repo" mv file renamed ;;
    mode) chmod +x "$root/repo/file" ;;
    empty) : ;;
    binary) printf 'added\0\n' >"$root/repo/added" ;;
    attribute)
      printf 'added -diff\n' >"$root/repo/.gitattributes"
      commit
      base="$(git -C "$root/repo" rev-parse HEAD)"
      printf 'added\n' >"$root/repo/added" ;;
    cancelled) printf 'added\n' >"$root/repo/added" ;;
    merge)
      git -C "$root/repo" checkout -qb feature
      leak_name="notuser"
      printf '/home/%s/notes\n' "$leak_name" >"$root/repo/added"
      commit
      git -C "$root/repo" checkout -q main
      git -C "$root/repo" -c user.name=Test -c user.email=test@example.com merge -q --no-ff feature -m merge ;;
  esac
  [[ "$kind" == merge ]] || commit
  if [[ "$kind" == cancelled ]]; then
    git -C "$root/repo" rm -q added
    commit
  fi
  case "$kind" in
    deletion|rename|mode|empty)
      scan "$base..HEAD" || { cat "$root/log"; echo "rejected $kind with real scanner" >&2; exit 1; }
      PATH="$root/bin:$PATH" scan "$base..HEAD" || { cat "$root/log"; echo "rejected $kind with zero scanner" >&2; exit 1; } ;;
    merge)
      if scan 'HEAD^2..HEAD'; then
        echo 'missed merge-only finding' >&2
        exit 1
      fi
      grep -q '^RuleID:' "$root/log" || { cat "$root/log"; exit 1; }
      grep -q '^File:.*added' "$root/log" || { cat "$root/log"; exit 1; }
      if grep -Fq "$leak_name" "$root/log"; then
        echo 'exposed merge finding' >&2
        exit 1
      fi ;;
    *)
      if PATH="$root/bin:$PATH" scan "$base..HEAD"; then
        echo "accepted zero count for $kind additions" >&2
        exit 1
      fi ;;
  esac
  echo "$kind passed"
  ls -ld -- "$root/repo"
  rm -r -- "$root/repo"
done
