#!/usr/bin/env bash
# Lint / package / publish a project's Helm chart.
#
#   scripts/helm.sh lint     — helm lint + template render
#   scripts/helm.sh package  — write a deterministic .tgz to <project>/dist/
#   scripts/helm.sh publish  — package + push; the version depends on the mode
#
#     RELEASE_VERSION  <version>, plus an SBOM (x.y, x, latest: scripts/aliases.sh)
#     PR_NUMBER        0.0.0-pr<n>+<sha> (appVersion: the PR's pr-<n>-<sha> image)
#     BRANCH_TAG       0.0.0-<branch>, also tagged <branch> (appVersion: sha-<short>)
#
# Chart + app versions are injected at package time so the chart source never
# has to be bumped by hand.

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
# shellcheck source=scripts/lib.sh
source scripts/lib.sh

require_cmd helm
require_cmd docker
[ -d "$CHART_DIR" ] || die "no chart directory at $CHART_DIR"

mode="${1:-lint}"
unpacked="${DIST_DIR:?}/.chart"
trap 'rm -rf "${unpacked:?}"' EXIT

# do_package <version> <app-version>  ->  sets $PACKAGED_TGZ, leaves the chart in $unpacked
do_package() {
  local version="$1" app_version="$2"
  mkdir -p "$DIST_DIR"
  rm -f "${DIST_DIR:?}"/*.tgz
  rm -rf "${unpacked:?}"
  run helm package "$CHART_DIR" --version "$version" --app-version "$app_version" --destination "$DIST_DIR" 1>&2
  PACKAGED_TGZ="${DIST_DIR}/${CHART_NAME}-${version}.tgz"
  [ -f "$PACKAGED_TGZ" ] || die "helm package did not produce $PACKAGED_TGZ"

  # helm stamps the current time into the archive; rewrite it deterministically.
  mkdir -p "$unpacked"
  tar -xzf "$PACKAGED_TGZ" -C "$unpacked"
  tar --sort=name --mtime="@${SOURCE_DATE_EPOCH}" --owner=0 --group=0 --numeric-owner \
    --mode='a+rX,u+w,go-w' -C "$unpacked" -cf - "$CHART_NAME" | gzip -n -9 >"${PACKAGED_TGZ}.tmp"
  mv "${PACKAGED_TGZ}.tmp" "$PACKAGED_TGZ"
  log "packaged ${PACKAGED_TGZ} sha256=$(sha256sum "$PACKAGED_TGZ" | cut -d' ' -f1)"
}

case "$mode" in
  lint)
    run helm lint "$CHART_DIR"
    run helm template "$CHART_NAME" "$CHART_DIR" >/dev/null
    [ ! -f "$CHART_DIR/ci-values.yaml" ] || run helm template "$CHART_NAME" "$CHART_DIR" --values "$CHART_DIR/ci-values.yaml" >/dev/null
    ;;
  package)
    version="0.0.0-dev"
    do_package "$version" "$version"
    ;;
  publish)
    kind="$(publish_kind)"
    case "$kind" in
      release) version="$RELEASE_VERSION"; app_version="$version"; require_cmd syft ;;
      pr)      version="0.0.0-pr${PR_NUMBER}+${GIT_SHORT_SHA}"; app_version="pr-${PR_NUMBER}-${GIT_SHA}" ;;
      branch)  version="0.0.0-${BRANCH_TAG}"; app_version="sha-${GIT_SHORT_SHA}"; require_cmd crane ;;
    esac
    # assert_unpublished and crane use docker's credentials, helm's are separate.
    registry_login docker
    registry_login helm
    [ "$kind" != release ] || assert_unpublished "${CHART_OCI_REPO}:${version}"
    do_package "$version" "$app_version"

    push_log="${DIST_DIR}/.helm-push.log"
    run helm push "$PACKAGED_TGZ" "$CHART_REPO" 2>&1 | tee "$push_log" >&2
    digest="$(grep -oE 'sha256:[0-9a-f]{64}' "$push_log" | head -1 || true)"
    [ -n "$digest" ] || die "could not read the chart digest from helm push output"

    # helm only pushes semver tags, so the plain branch name is added afterwards.
    [ "$kind" != branch ] || run crane tag "${CHART_OCI_REPO}@${digest}" "$BRANCH_TAG"

    emit "chart-repo=${CHART_OCI_REPO}"
    emit "chart-digest=${digest}"
    emit "chart-tgz=${PACKAGED_TGZ}"
    if [ "$kind" = release ]; then
      sbom="${DIST_DIR}/${CHART_NAME}.chart.spdx.json"
      run syft scan "dir:${unpacked}/${CHART_NAME}" -o "spdx-json=${sbom}"
      emit "chart-sbom=${sbom}"
    fi
    step_summary "**\`${PROJECT_ID}\`** chart \`${CHART_OCI_REPO}:${version}\` (\`${digest}\`)"
    ;;
  *) die "unknown mode '$mode' (lint|package|publish)" ;;
esac

log "helm ${mode} done"
