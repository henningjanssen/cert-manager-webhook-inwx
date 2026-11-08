#!/usr/bin/env bash
# Deletes one closed PR's preview images and charts from GHCR. Needs PR_NUMBER
# and OWNER; DRY_RUN=1 only prints. Release artifacts must never be touched, so
# a version goes only when every tag on it belongs to this PR.

set -euo pipefail

# select_versions <regex>, version JSON on stdin (concatenated pages are fine).
# shortcut: untagged versions are kept, so a multi-arch preview leaves its per-platform manifests behind; prune those by age if a tool goes multi-arch.
select_versions() {
  jq -r --arg re "$1" '.[]
    | select((.metadata.container.tags | length > 0) and all(.metadata.container.tags[]; test($re)))
    | .id'
}

clean() { # <package> <regex>
  local path="users/${OWNER}/packages/container/${1//\//%2F}/versions" err out id
  err="$(mktemp)"
  if ! out="$(gh api --paginate "$path" 2>"$err")"; then
    if grep -q 'HTTP 404' "$err"; then echo "skip $1: no such package"; return 0; fi
    cat "$err" >&2
    return 1
  fi
  for id in $(select_versions "$2" <<<"$out"); do
    echo "delete $1 version $id"
    [ -n "${DRY_RUN:-}" ] || gh api -X DELETE "$path/$id" --silent
  done
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  : "${OWNER:?}" "${PR_NUMBER:?}"
  [[ "$PR_NUMBER" =~ ^[1-9][0-9]*$ ]] || { echo "bad PR_NUMBER" >&2; exit 1; }
  cd "$(dirname "${BASH_SOURCE[0]}")/.."
  for moon_yml in tools/*/moon.yml; do
    tool="$(basename "$(dirname "$moon_yml")")"
    # shortcut: assumes the default package names (the tool id), read IMAGE_NAME/CHART_NAME here if a tool ever overrides them.
    clean "$tool" "^pr-${PR_NUMBER}(-[0-9a-f]+)?$"
    clean "charts/$tool" "^0\\.0\\.0-pr${PR_NUMBER}([+_.-].*)?$"
  done
fi
