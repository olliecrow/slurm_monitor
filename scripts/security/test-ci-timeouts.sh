#!/usr/bin/env bash
set -euo pipefail
# Local builds and CI must select the same reviewed patch release.
grep -qx 'go 1.27.3' go.mod
grep -q '^          go-version-file: go.mod$' .github/workflows/ci.yml
if grep -q '^          go-version:' .github/workflows/ci.yml; then
  echo "CI must read its Go version from go.mod" >&2
  exit 1
fi
# Every job must set a positive job-level bound.
awk '
  /^jobs:$/ { jobs=1; next }
  !jobs { next }
  /^  [a-zA-Z_][a-zA-Z_0-9-]*:$/ {
    if (job && !bounded) failed=1
    job=1; bounded=0
  }
  /^    timeout-minutes: [1-9][0-9]*$/ { bounded=1 }
  END { if (!job || !bounded || failed) exit 1 }
' .github/workflows/ci.yml || {
  echo "every CI job needs a positive timeout-minutes" >&2
  exit 1
}
