#!/usr/bin/env bash
# Build / publish a project's container image, reproducibly.
#
#   scripts/docker.sh build    — write an OCI archive to <project>/dist/
#   scripts/docker.sh publish  — build + push; tags depend on the mode (see lib.sh)
#
#     RELEASE_VERSION  <version>, plus an SBOM (x.y, x, latest: scripts/aliases.sh)
#     PR_NUMBER        pr-<n>, pr-<n>-<sha>
#     BRANCH_TAG       <branch>, sha-<short>
#
# BuildKit's own --provenance/--sbom are off: they embed build times and would
# change the digest. CI attests the digest emitted here instead.

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
# shellcheck source=scripts/lib.sh
source scripts/lib.sh

require_cmd docker
require_cmd jq
[ -f "$DOCKERFILE" ] || die "no Dockerfile at $DOCKERFILE"

mode="${1:-build}"
kind=""
case "$mode" in
  build) version="0.0.0-dev"; tags=("$version") ;;
  publish)
    kind="$(publish_kind)"
    case "$kind" in
      release) version="$RELEASE_VERSION"; tags=("$version") ;;
      pr)      version="pr-${PR_NUMBER}"; tags=("$version" "pr-${PR_NUMBER}-${GIT_SHA}") ;;
      branch)  version="$BRANCH_TAG"; tags=("$version" "sha-${GIT_SHORT_SHA}") ;;
    esac
    ;;
  *) die "unknown mode '$mode' (build|publish)" ;;
esac
ref="${IMAGE_REPO}:${tags[0]}"
output_opts="oci-mediatypes=true,rewrite-timestamp=true"
metadata="${DIST_DIR}/image-metadata.json"
mkdir -p "$DIST_DIR"

build=(docker buildx build
  "$DOCKER_CONTEXT"
  --file "$DOCKERFILE"
  --platform "$IMAGE_PLATFORMS"
  --build-arg "SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH}"
  --provenance=false
  --sbom=false
  --label "org.opencontainers.image.source=https://github.com/${SOURCE_REPOSITORY}"
  --label "org.opencontainers.image.revision=${GIT_SHA}"
  --label "org.opencontainers.image.version=${version}"
  --metadata-file "$metadata"
)
for t in "${tags[@]}"; do build+=(--tag "${IMAGE_REPO}:${t}"); done
if [ "$kind" = release ] || [ -n "${NO_CACHE:-}" ]; then build+=(--no-cache); fi

if [ "$mode" = build ]; then
  run "${build[@]}" --output "type=oci,dest=${DIST_DIR}/${IMAGE_NAME}.oci.tar,${output_opts}"
  log "digest: $(jq -r '."containerimage.digest"' "$metadata")"
  exit 0
fi

registry_login docker
if [ "$kind" = release ]; then
  require_cmd syft
  # Both, before anything is pushed: the image is the first artifact out.
  assert_unpublished "$ref"
  assert_unpublished "${CHART_OCI_REPO}:${version}"
fi
run "${build[@]}" --output "type=image,push=true,${output_opts}"

digest="$(jq -r '."containerimage.digest" // empty' "$metadata")"
[ -n "$digest" ] || die "buildx reported no digest for $ref"

emit "image-repo=${IMAGE_REPO}"
emit "image-digest=${digest}"
if [ "$kind" = release ]; then
  sbom="${DIST_DIR}/${IMAGE_NAME}.image.spdx.json"
  # shortcut: SBOM covers only the first of IMAGE_PLATFORMS, generate one per platform when a tool ships several.
  run syft scan "registry:${IMAGE_REPO}@${digest}" --platform "${IMAGE_PLATFORMS%%,*}" -o "spdx-json=${sbom}"
  emit "image-sbom=${sbom}"
fi
step_summary "**\`${PROJECT_ID}\`** image \`${ref}\` (\`${digest}\`)"

log "docker ${kind} done"
