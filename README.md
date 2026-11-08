# tools monorepo

Small, unrelated **docker + helm** tools that share one build/release pipeline,
orchestrated by [moon](https://moonrepo.dev). One directory per tool under
[`tools/`](tools/); each is built, versioned and released independently.

| Tool | What it is |
|------|------------|
| [`cert-manager-webhook-inwx`](tools/cert-manager-webhook-inwx/) | cert-manager ACME DNS01 webhook solver for INWX |

- **Add a tool:** [`docs/adding-a-new-tool.md`](docs/adding-a-new-tool.md)
- **Why it's built this way:** [`docs/adr/`](docs/adr/)

## Setup

```bash
mise install     # every tool version is pinned in mise.toml
```

## Tasks

Every tool responds to the same tasks. Run `moon run <tool>:<task>`, or
`moon run :<task>` for all tools. moon reserves `:` as the separator between
project and task, so `build:image` is spelled `build-image`.

| Task | What it does |
|------|--------------|
| `build` | Compile the tool |
| `lint` / `lint-fix` | Check code and chart / apply automatic fixes |
| `format` / `format-fix` | Check formatting / rewrite files |
| `test` | Run the tests |
| `build-image` | Build the image to `tools/<tool>/dist/<image>.oci.tar` (`docker load -i` to run it) |
| `build-chart` | Package the chart into `tools/<tool>/dist/` |
| `publish-image` | Build and push the image (needs `RELEASE_VERSION`, `PR_NUMBER` or `BRANCH_TAG`) |
| `publish-chart` | Package and push the chart (same variables) |

CI is the only place that should run the publish tasks. It uses
`moon exec --ignore-ci-checks`, because they are excluded from `moon ci`.

## CI and releases

| Trigger | Effect (changed tools only) |
|---|---|
| PR or push to `main` / `dev` | `moon ci`: checks, nothing published |
| PR opened or updated | Publishes `pr-<n>` artifacts |
| PR closed or merged | Deletes that PR's artifacts |
| Push to `main` / `dev` | Updates the branch's tags |
| `<tool>@<version>` tag | Releases that tool |

### Releasing

```bash
git tag cert-manager-webhook-inwx@1.2.3            # or 1.2.3-alpha.1, -beta.1, -rc.1
git push origin cert-manager-webhook-inwx@1.2.3
```

The tag starts `release.yml`, which tests and lints the tool, pushes its image and
chart, attests both, moves the `x.y`, `x` and `latest` tags where the version is
the newest, then creates a draft GitHub Release `<tool>@<version>` with the image
and chart digests in its notes and the chart `.tgz` and SPDX SBOMs attached, and
publishes it. Publishing locks the git tag and assets; the title and notes stay
editable.

### Tags

| Source | Image tags | Chart tags | Changes? |
|---|---|---|---|
| Stable release | `x.y.z` | `x.y.z` | Never |
| Stable release, if newest in its range | `x.y`, `x`, `latest` | the same | Moves to the newest |
| Prerelease | `x.y.z-rc.N` | `x.y.z-rc.N` | Never; moves no other tag |
| `main` / `dev` | `main` / `dev`, `sha-<short>` | `0.0.0-main` / `0.0.0-dev`, `main` / `dev` | Moves with the branch |
| PR | `pr-<n>`, `pr-<n>-<sha>` | `0.0.0-pr<n>_<short>` | Deleted when the PR closes |

Images are at `ghcr.io/<owner>/<tool>`, charts at
`oci://ghcr.io/<owner>/charts/<tool>`.

Helm ignores prereleases in ranges, so install one with its exact version or
`--devel`. `--version 1.2` resolves as a range to the newest `1.2.z`:

```bash
helm install x oci://ghcr.io/<owner>/charts/<tool> --version 1.2.3-rc.1
```

PR cleanup only deletes a version when every tag on it belongs to that PR, so
release artifacts cannot be removed by it.

A release that fails after a push uses up that version: delete any leftover draft
and release the next one. Reasoning is in
[ADR 0004](docs/adr/0004-immutable-per-tool-releases.md).

### Verifying a release

Every released image and chart has a signed build provenance attestation and an SBOM
attestation. The exact commands for a release are in its release notes:

```bash
gh attestation verify oci://ghcr.io/<owner>/<tool>@sha256:... --repo <owner>/<repo>
gh attestation verify oci://ghcr.io/<owner>/charts/<tool>@sha256:... --repo <owner>/<repo>
```

Add `--predicate-type https://spdx.dev/Document/v2.3` to check the SBOM
attestation instead of provenance.

### Reproducible builds

Building the same commit again produces the same image digest and chart
`.tgz`. A chart's OCI manifest digest differs per push, because helm records a
push-time annotation. How:

- all timestamps come from the commit (`SOURCE_DATE_EPOCH`);
- base images are pinned by digest, and BuildKit and every CLI by version;
- Go builds use `-trimpath` and no embedded VCS data;
- chart archives are repacked with sorted entries and fixed owners;
- SBOMs and provenance are attached as separate attestations, so they cannot
  change the digest of the artifact itself.

## Layout

```
tools/<tool>/       one moon project per tool: moon.yml, Dockerfile, chart/, sources
.moon/              workspace config and inherited task definitions
scripts/            all build/publish logic; CI only calls these through moon
mise.toml           pinned tool versions
```
