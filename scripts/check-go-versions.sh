#!/usr/bin/env bash
# Fails if the Go version drifts between:
#   - <project>/go.mod            (source of truth)
#   - <project>/Dockerfile        (golang:X.Y base image)
#   - mise.toml                   (workspace-wide `go = "X.Y.Z"`)
# Invoked by the `versions-check` task in .moon/tasks/go.yml.
set -euo pipefail

root="${MOON_PROJECT_ROOT:-$PWD}"
ws="${MOON_WORKSPACE_ROOT:-$(git rev-parse --show-toplevel)}"
gomod="${root}/go.mod"
dockerfile="${root}/Dockerfile"
misetoml="${ws}/mise.toml"

[ -f "$gomod" ] || { echo "no go.mod at $gomod"; exit 1; }

want="$(awk '/^go /{print $2}' "$gomod" | cut -d. -f1,2)"
rc=0

if [ -f "$dockerfile" ]; then
  have="$(grep -oE 'golang:[0-9]+\.[0-9]+' "$dockerfile" | head -1 | cut -d: -f2 || true)"
  if [ -n "$have" ] && [ "$want" != "$have" ]; then
    echo "Go version mismatch: go.mod=$want  Dockerfile=$have"; rc=1
  fi
fi

if [ -f "$misetoml" ]; then
  have="$(grep -oE '^go[[:space:]]*=[[:space:]]*"[0-9]+\.[0-9]+' "$misetoml" | grep -oE '[0-9]+\.[0-9]+' | head -1 || true)"
  if [ -n "$have" ] && [ "$want" != "$have" ]; then
    echo "Go version mismatch: go.mod=$want  mise.toml=$have"; rc=1
  fi
fi

[ "$rc" -eq 0 ] && echo "Go versions match: $want"
exit "$rc"
