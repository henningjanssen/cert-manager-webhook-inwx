# 2. Per-tool releases, moving tags and PR previews

Date: 2026-11-08
Status: accepted

## Context

Each tool needs its own semantic versions. A released version must never change
or disappear, while people also want `main`, `dev`, `latest` and `x.y` to follow
along, and PRs to publish artifacts for the tools they change. The previous PR
cleanup used an input `actions/delete-package-versions` does not have, so every
closed PR silently deleted the oldest versions, including releases.

## Decision

- A tag `<tool>@<version>` releases that tool. Its image and chart share the
  version: `x.y.z`, or `x.y.z-(alpha|beta|rc).N`.
- `x.y.z` and prerelease tags are immutable. A stable release moves `x.y`, `x`
  and `latest` only if it is the newest in that range, computed from the
  registry's `X.Y.Z` tags. Prereleases move nothing.
- `main` and `dev` follow their branches; PRs publish `pr-<n>` artifacts. Both
  are limited to tools the change affects, and image and chart go together.
- Moving tags are applied after attestation, so they never point at an
  unattested release.
- Helm only pushes semver tags, so chart aliases are added with `crane tag`.
  Helm itself ignores non-semver tags and treats `--version 1.2` as a range.
- PR cleanup deletes a package version only if every tag on it belongs to that
  PR. Reproducible builds can give a PR and a release one digest, so a version
  with any other tag stays. Untagged versions are never deleted.
- SBOMs and SLSA provenance are separate signed attestations on the digest, so
  they do not affect reproducibility. Previews and branches do not get them.
- The GitHub Release is created as a draft, gets all its assets, and is
  published last, because the repository's immutable releases setting locks
  assets and the git tag on publish.
- Legacy `v0.0.x` tags and releases stay as they are.

## Consequences

- GHCR has no server-side tag immutability. The publish scripts enforce it by
  refusing an existing version before anything is pushed, and they alone move
  aliases. Anyone with direct registry write access could still overwrite a tag;
  the digests in the release notes and the attestations would expose that.
- A release that fails after a push uses up the version: its image or chart may
  exist unattested, and a draft release may remain. Delete the draft and release
  the next version.
- GitHub keeps one pending run per concurrency group, so a third queued release
  of the same tool cancels the second. Re-run the cancelled tag.
- Reproducibility covers the image digest and the chart `.tgz`. A chart's OCI
  manifest digest differs per push, because helm records a push-time annotation.
