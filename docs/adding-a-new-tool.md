# Adding a new tool

Every tool is one moon project under `tools/<tool-id>/`. Nothing in
`.github/workflows/` changes. Tasks, tags and releases are described in the
[root README](../README.md).

## 1. Scaffold

Copy the webhook's pipeline files, then add your sources:

```bash
mkdir tools/<tool-id>
cd tools/cert-manager-webhook-inwx
cp -r moon.yml Dockerfile .dockerignore chart ../<tool-id>/
```

Then edit:

- `moon.yml`: set `id` and `project`. Keep `tags: ['docker', 'helm']`, because a
  release ships both.
- `Dockerfile`: the build steps and binary name.
- `chart/`: set `name` in `Chart.yaml` to the tool id and replace the
  webhook-specific templates and values. `version` and `appVersion` are
  placeholders; `scripts/helm.sh` injects the real values.

The defaults are image `ghcr.io/<owner>/<tool-id>`, chart
`oci://ghcr.io/<owner>/charts/<tool-id>` and platform `linux/amd64`. `moon.yml`
`env:` can override `IMAGE_NAME`, `IMAGE_REPO`, `IMAGE_PLATFORMS`, `DOCKERFILE`,
`DOCKER_CONTEXT`, `CHART_DIR`, `CHART_NAME` and `CHART_REPO`, but
`scripts/cleanup-pr.sh` assumes the default package names.

A language other than Go needs its runtime in `mise.toml` and a
`.moon/tasks/<language>.yml` defining the tasks in
[`go.yml`](../.moon/tasks/go.yml).

## 2. Multi-arch images

Set `IMAGE_PLATFORMS: 'linux/amd64,linux/arm64'` in `moon.yml` `env:` to publish
one multi-arch index; the attestations cover its digest, but the SBOM covers only
the first platform. CI has no QEMU, so the `Dockerfile` must cross-compile
(`FROM --platform=$BUILDPLATFORM`, `GOOS=$TARGETOS GOARCH=$TARGETARCH`), as the
webhook's does.

## 3. Verify locally

```bash
moon run <tool-id>:lint <tool-id>:format <tool-id>:test <tool-id>:build
moon run <tool-id>:build-image <tool-id>:build-chart
```

The first release of a new tool has not been tested end to end: GHCR may answer
the existence check for a package that doesn't exist yet with an auth error,
which fails the publish safely.
