#!/usr/bin/env bash
# Shared helpers for the docker/helm publish scripts.
#
# These scripts are the single source of truth for how anything is built and
# published. The GitHub workflows only decide *which* tool runs and pass:
#
#   RELEASE_VERSION   x.y.z[-(alpha|beta|rc).N]   (from the `<tool>@<version>` tag)
#   PR_NUMBER         123                         (PR preview)
#   BRANCH_TAG        main | dev                  (branch snapshot)
#   REGISTRY          ghcr.io       (override the registry host)
#   REGISTRY_OWNER    henningjanssen
#
# Everything else is derived from moon's per-task environment
# (MOON_PROJECT_ID, MOON_PROJECT_ROOT, …) and the project's moon.yml `env`.

set -euo pipefail

# ── logging ────────────────────────────────────────────────────────────────
log()  { printf '\033[36m▸ %s\033[0m\n' "$*" >&2; }
warn() { printf '\033[33m! %s\033[0m\n' "$*" >&2; }
die()  { printf '\033[31m✗ %s\033[0m\n' "$*" >&2; exit 1; }
run()  { log "\$ $*"; "$@"; }

# ── context, resolved once ─────────────────────────────────────────────────
PROJECT_ID="${MOON_PROJECT_ID:?run me through moon (missing MOON_PROJECT_ID)}"
PROJECT_ROOT="${MOON_PROJECT_ROOT:?run me through moon (missing MOON_PROJECT_ROOT)}"
WORKSPACE_ROOT="${MOON_WORKSPACE_ROOT:-$(git rev-parse --show-toplevel)}"

REGISTRY="${REGISTRY:-ghcr.io}"
REGISTRY_OWNER="${REGISTRY_OWNER:-${GITHUB_REPOSITORY_OWNER:-henningjanssen}}"
REGISTRY_OWNER="$(printf '%s' "$REGISTRY_OWNER" | tr '[:upper:]' '[:lower:]')"

# Per-project overrides (set in tools/<name>/moon.yml `env:`), with defaults.
IMAGE_NAME="${IMAGE_NAME:-$PROJECT_ID}"
IMAGE_REPO="${IMAGE_REPO:-${REGISTRY}/${REGISTRY_OWNER}/${IMAGE_NAME}}"
IMAGE_PLATFORMS="${IMAGE_PLATFORMS:-linux/amd64}"
DOCKERFILE="${DOCKERFILE:-${PROJECT_ROOT}/Dockerfile}"
DOCKER_CONTEXT="${DOCKER_CONTEXT:-$PROJECT_ROOT}"

CHART_DIR_NAME="${CHART_DIR:-chart}"
CHART_DIR="${PROJECT_ROOT}/${CHART_DIR_NAME}"
CHART_NAME="${CHART_NAME:-$PROJECT_ID}"
CHART_REPO="${CHART_REPO:-oci://${REGISTRY}/${REGISTRY_OWNER}/charts}"
CHART_OCI_REPO="${CHART_REPO#oci://}/${CHART_NAME}"

# What was checked out; GITHUB_SHA is a synthetic merge commit on PRs.
GIT_SHA="$(git -C "$WORKSPACE_ROOT" rev-parse HEAD)"
GIT_SHORT_SHA="${GIT_SHA:0:7}"
SOURCE_REPOSITORY="${GITHUB_REPOSITORY:-henningjanssen/cert-manager-webhook-inwx}"
# Same commit, same timestamps: the base of every reproducible artifact.
SOURCE_DATE_EPOCH="$(git -C "$WORKSPACE_ROOT" log -1 --format=%ct)"
export SOURCE_DATE_EPOCH

DIST_DIR="${PROJECT_ROOT}/dist"

# ── output for CI (digests etc.) ──────────────────────────────────────────
# Appends key=value to $GITHUB_OUTPUT when present, always echoes.
emit() {
  local kv="$1"
  echo "$kv"
  [ -n "${GITHUB_OUTPUT:-}" ] && echo "$kv" >>"$GITHUB_OUTPUT" || true
}

step_summary() {
  [ -n "${GITHUB_STEP_SUMMARY:-}" ] || return 0
  printf '%s\n' "$*" >>"$GITHUB_STEP_SUMMARY"
}

require_cmd() { command -v "$1" >/dev/null 2>&1 || die "'$1' is required but not installed"; }

registry_login() {
  # Only attempts a login when a token is available (CI). Local runs are
  # expected to already be logged in.
  local tool="$1" # docker | helm
  [ -n "${REGISTRY_PASSWORD:-}" ] || { warn "no REGISTRY_PASSWORD set — assuming an existing $tool login"; return 0; }
  local user="${REGISTRY_USERNAME:-${GITHUB_ACTOR:-x}}"
  case "$tool" in
    docker) printf '%s' "$REGISTRY_PASSWORD" | run docker login "$REGISTRY" -u "$user" --password-stdin ;;
    helm)   printf '%s' "$REGISTRY_PASSWORD" | run helm registry login "$REGISTRY" -u "$user" --password-stdin ;;
  esac
}

# ── publish modes ──────────────────────────────────────────────────────────
RELEASE_RE='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(alpha|beta|rc)\.(0|[1-9][0-9]*))?$'

# Prints release | pr | branch. Exactly one of the mode variables must be set
# and valid, so a misconfigured job cannot publish under the wrong kind of tag.
publish_kind() {
  local set=0
  [ -z "${RELEASE_VERSION:-}" ] || set=$((set + 1))
  [ -z "${PR_NUMBER:-}" ] || set=$((set + 1))
  [ -z "${BRANCH_TAG:-}" ] || set=$((set + 1))
  [ "$set" -eq 1 ] || die "publish needs exactly one of RELEASE_VERSION, PR_NUMBER, BRANCH_TAG"
  if [ -n "${RELEASE_VERSION:-}" ]; then
    [[ "$RELEASE_VERSION" =~ $RELEASE_RE ]] || die "RELEASE_VERSION '$RELEASE_VERSION' is not x.y.z or x.y.z-(alpha|beta|rc).N"
    echo release
  elif [ -n "${PR_NUMBER:-}" ]; then
    [[ "$PR_NUMBER" =~ ^[1-9][0-9]*$ ]] || die "PR_NUMBER '$PR_NUMBER' is not a number"
    echo pr
  else
    [[ "$BRANCH_TAG" =~ ^(main|dev)$ ]] || die "BRANCH_TAG must be main or dev"
    echo branch
  fi
}

# compute_aliases <version>, existing tags on stdin: prints the moving tags
# <version> takes over. Prereleases move nothing; only X.Y.Z tags compete.
compute_aliases() {
  local version="$1" n='(0|[1-9][0-9]*)' stable minor="${1%.*}" major="${1%%.*}"
  [[ "$version" =~ ^$n\.$n\.$n$ ]] || return 0
  stable="$({ grep -E "^$n\.$n\.$n$" || true; echo "$version"; } | sort -Vu)"
  [ "$(grep -E "^${minor//./\\.}\." <<<"$stable" | tail -1)" != "$version" ] || echo "$minor"
  [ "$(grep -E "^$major\." <<<"$stable" | tail -1)" != "$version" ] || echo "$major"
  [ "$(tail -1 <<<"$stable")" != "$version" ] || echo latest
}

# Exact versions are immutable: fail if <ref> exists, and on any lookup error
# other than a definite "not found".
assert_unpublished() {
  local ref="$1" out
  if out="$(docker buildx imagetools inspect --raw "$ref" 2>&1)"; then
    die "$ref already exists; releases are immutable"
  fi
  grep -qE '^ERROR: .*: not found$|manifest unknown|name unknown' <<<"$out" || die "could not check whether $ref exists: $out"
  log "$ref is not published yet"
}
