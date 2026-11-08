# 1. Monorepo of docker + helm tools, orchestrated by moon

Date: 2026-09-06
Status: accepted

## Context

Several small, unrelated tools are each built into a container image and a Helm
chart and published to GHCR. Separate repositories would carry near-identical
pipelines that drift apart. Each tool still needs to be built and released
independently, and CI should only touch what changed.

## Decision

- One moon project per tool under `tools/<tool>/`.
- Shared tasks in `.moon/tasks/`: one file per language, plus `tag-docker.yml`
  and `tag-helm.yml` that a tool opts into with `tags`.
- All build and publish logic lives in `scripts/`. Moon tasks are thin wrappers;
  workflows only choose which tools run and pass a few variables.
- CI is limited to tools affected by a change. A change to `scripts/` or
  `mise.toml` affects every tool.
- mise pins every CLI, locally and in CI. moon manages no toolchain.

Release design is in [ADR 0004](0004-immutable-per-tool-releases.md).

## Consequences

- A new tool needs no new workflow or pipeline code.
- moon and mise are hard dependencies of local development and CI.
