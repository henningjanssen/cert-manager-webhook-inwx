#!/usr/bin/env bash
# Moves x.y, x and latest onto a just-released, already attested version, for
# both image and chart. Needs RELEASE_VERSION, IMAGE_DIGEST and CHART_DIGEST.

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
# shellcheck source=scripts/lib.sh
source scripts/lib.sh

require_cmd crane
[ "$(publish_kind)" = release ] || die "aliases only apply to releases"
registry_login docker

move() {
  local repo="$1" digest="$2" tags alias
  [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || die "bad digest '$digest' for $repo"
  tags="$(crane ls "$repo")"
  for alias in $(compute_aliases "$RELEASE_VERSION" <<<"$tags"); do
    run crane tag "${repo}@${digest}" "$alias"
  done
}

move "$IMAGE_REPO" "${IMAGE_DIGEST:?}"
move "$CHART_OCI_REPO" "${CHART_DIGEST:?}"
log "aliases done"
