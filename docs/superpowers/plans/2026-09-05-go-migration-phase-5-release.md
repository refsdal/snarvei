# Go Migration Phase 5: Release Pipeline, Remaining E2E Flows and Docs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make merging to `main` a release (svu + GoReleaser + cosign, multi-arch image on ghcr.io), close the Playwright coverage the spec lists, clear the phase-4 deferred frontend minors, and rewrite the documentation for the Go/container era so the first merge of this PR to `main` produces `v0.1.0`.

**Architecture:** Copied from Pjokk with names swapped and the landing-site pieces removed: `.goreleaser.yaml` releases from a tag that `release.yml` creates after svu computes the version from Conventional Commits; the existing `test.yml` gains `goreleaser check`; `ci.yml` keeps building the PR preview. Playwright gets a shared support module (recipient-keyed mailbox reads, no mailbox clearing) and one more browser spec. Docs: `AGENTS.md`, `README.md`, `docs/runbook.md`, `CONTRIBUTING.md` rewritten for the container era; the migration banners go.

**Tech Stack:** GoReleaser 2.18 (`dockers_v2`), svu 3.4, cosign 3.1 (keyless via GitHub OIDC), syft 1.51, GitHub Actions, Playwright, bun, TanStack Router.

**Spec:** `docs/superpowers/specs/2026-09-04-go-backend-migration-design.md` — sections 7 (build, image, release), 9 (testing), 10 (documentation), phase 5 in section 11. Read section 7 before Task 1 and section 10 before Task 5.

## Global Constraints

- Branch `feat/go-migration-phase-5` is stacked on `feat/go-migration-phase-4` (PR #82 → #81 → #80). The PR targets `feat/go-migration-phase-4` until that merges, then `main`. The first merge to `main` after all five phases must produce `v0.1.0` (no `v*` tag exists yet; `svu next --v0` on `feat` commits yields `v0.1.0`).
- Release model (spec §7, verbatim where it matters): `release.yml` on `push: main` and `workflow_dispatch` (`dry_run` default true, `allow_major` default false); calls `test.yml`; svu computes the next version (`--v0` unless `allow_major`); nothing releasable → job ends green; otherwise tag `vX.Y.Z`, `goreleaser release --clean`, delete the tag (and the GitHub release) if publishing fails. Permissions on the publish job: `contents: write`, `packages: write`, `id-token: write`. Concurrency group `release`, no cancel. No `environment:` block (the repo's old `Dev`/`Production` environments belong to the Workers deploys and would add an approval gate the spec does not want).
- `.goreleaser.yaml` (spec §7): `version: 2`, `project_name: snarvei`, `dist: dist/goreleaser`, before hook `bash scripts/spa-embed-overlay.sh`, one build (`dir: apps/server`, `main: ./cmd/snarvei`, `binary: snarvei`, `CGO_ENABLED=0`, linux amd64/arm64, `-trimpath`, ldflags `-s -w -X main.version={{.Version}}`), tar.gz archives `snarvei_{{ .Version }}_{{ .Os }}_{{ .Arch }}`, `checksums.txt`, syft SBOM per archive, keyless cosign `sign-blob --yes --bundle=${signature} ${artifact}` on the checksum file (`signature: "${artifact}.sigstore.json"`), `dockers_v2` image `ghcr.io/refsdal/snarvei` with tags `{{ .Version }}`, `{{ .Major }}.{{ .Minor }}`, `{{ .Major }}`, `latest`, `sha-{{ .FullCommit }}`, platforms linux/amd64 + linux/arm64, `build_args BINARY_ROOT: "."`, `extra_files: [docker/data-skel]`, `docker_signs` cosign `sign --yes ${artifact}@${digest}`, Conventional-Commits changelog groups (Features/Bug fixes/Performance/Other, excludes docs/test/chore/ci/style/merge), release footer naming the image, snapshot template `{{ incpatch .Version }}-snapshot.{{ .ShortCommit }}`.
- Dockerfile contract (already in the repo, do not change): `ARG TARGETPLATFORM`, `ARG BINARY_ROOT=dist/server`, `COPY ${BINARY_ROOT}/${TARGETPLATFORM}/snarvei /app/snarvei`, `COPY docker/data-skel/ /data/`. GoReleaser's context holds binaries at `linux/<arch>/snarvei`, hence `BINARY_ROOT: "."`.
- `test.yml` gains `goreleaser check` (mise `install_args` adds `aqua:goreleaser/goreleaser`); `.mise.toml` gains `[tasks.snapshot]` (`goreleaser release --snapshot --clean --skip=sign` with the overlay restored on exit) and `mise run check` runs `goreleaser check` too.
- Playwright (spec §9) flows that must exist after this phase, in a browser: sign up, sign in, wrong password rejected, create organization, create team, invite member, accept invitation and see only the team's links, assign member to team, create link, visit `/l/{slug}`, edit target and see history, view analytics, delete link and confirm removal, update profile name, forgot-password flow, Scalar page renders. Already covered by `e2e/app.spec.ts`: sign up, create org/team/link, redirect, analytics, retarget, wrong password, invitee registration, account deletion. This phase adds the rest in `e2e/flows.spec.ts`.
- Mailbox discipline: `GET /api/_test/mail` is one process-wide recording shared by every spec file, and files run in parallel workers. Every mailbox read is keyed by recipient (`lastMailTo(request, email)` in `e2e/support.ts`); no spec calls `DELETE /api/_test/mail`. `signUp` (throttle retry) and `workspace` live in `e2e/support.ts`; the existing specs are refactored onto it (no behaviour change, no weakened assertion).
- Frontend deferred minors closed here: the `router.tsx` ↔ `routes/landing/page.tsx` static import cycle (pages use `getRouteApi` instead of importing route objects), `NotFound` renders full-height inside the app shell (`fullScreen` prop), the drawer's organization `Select` both switches and navigates (navigate only; the route switches). Not in scope: bundle-size work, `openapi-typescript` as a devDependency (bun hoists one TypeScript; `bunx --package` isolation stays).
- Docs (spec §10): `AGENTS.md` rewritten (locked decisions become the spec's table; Cloudflare, D1, Drizzle, Better Auth and Vitest sections go; new sections on the Deps rule, the Limen boundary, the spec-first workflow, migrations under goose, the release model and Conventional Commits). `README.md`: product text kept; stack, quickstart, self-hosting (tag ladder, configuration table, compose), upgrading, backups, behind a proxy, verifying a release (cosign commands), repository layout, CI/CD, development rewritten. `docs/runbook.md`: environments become "wherever the container runs", secrets become environment variables, deploy/verify, rollback (previous tag; migrations forward-only; Postgres backups are the operator's), common failures updated. `CONTRIBUTING.md` created: Conventional Commits, `mise install`, `bun install`, `mise run test`, `go generate` after spec or query changes, `bun run gen:client`. Every "Migration in progress" banner is removed. Facts for the docs come from `.env.example`, spec §8 (configuration table), spec §7 (release), `docker-compose.selfhost.yml`, `apps/server/cmd/snarvei/main.go` (dispatch modes: none → migrate then serve; `server`; `migrate`/`migrations`; `healthcheck`; else usage exit 2), `/healthz` (`{"ok":true,"service":"snarvei","version":…}`).
- Cosign verification identity: `https://github.com/refsdal/snarvei/\.github/workflows/release\.yml@.*`, issuer `https://token.actions.githubusercontent.com`.
- `bun run check`, `bun run test`, `bun run build`, `goreleaser check`, `go vet`/`go test -p 1 -count=1 ./...` green at every commit; `E2E_REBUILD=1 mise run e2e` green at the end of Task 3 (expected 19 + the new flows). Disk on the dev machine is ~83% full: never prune Docker images or volumes.
- Conventional Commits with the two trailers `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01UdGgRFBUoiwkd9PLH7zUJE`. Go and bun via `mise exec --` when not on PATH; run from the repo root.

---

## File Structure

```
.goreleaser.yaml                     Task 1 (new)
.mise.toml                           Task 1 (snapshot task; check runs goreleaser check)
.github/workflows/test.yml           Task 1 (goreleaser check step + install arg; header comment)
.github/workflows/release.yml        Task 2 (new)
.github/workflows/ci.yml             Task 2 (drop the unused buildx action; comment about release.yml)
e2e/support.ts                       Task 3 (new: signUp, workspace, lastMailTo, PASSWORD, unique, headers)
e2e/auth-api.spec.ts, links-api.spec.ts, app.spec.ts   Task 3 (use support.ts; recipient-keyed mail; no DELETE)
e2e/flows.spec.ts                    Task 3 (new browser flows)
apps/frontend/src/router.tsx, routes/landing/page.tsx, routes/reset-password/page.tsx, routes/links/page.tsx, routes/link-details/page.tsx, routes/invitation/page.tsx, routes/settings/page.tsx, routes/dashboard/page.tsx, routes/organization/page.tsx   Task 4 (getRouteApi where a page imports a route object)
apps/frontend/src/components/route-error.tsx, components/app-shell.tsx   Task 4
AGENTS.md, README.md, docs/runbook.md, CONTRIBUTING.md   Task 5
```

---

### Task 1: GoReleaser configuration, snapshot task, config check in CI

**Files:**
- Create: `.goreleaser.yaml`
- Modify: `.mise.toml`, `.github/workflows/test.yml`

**Interfaces:**
- Produces: `goreleaser check` passing; `mise run snapshot` producing `dist/goreleaser/` with two archives, `checksums.txt`, two SPDX SBOMs and a local multi-arch image build (not pushed; signing skipped); Task 2's `release.yml` runs `goreleaser release --clean` against this file.

- [ ] **Step 1: Write `.goreleaser.yaml`**

```yaml
# GoReleaser owns everything downstream of a version tag: binaries,
# archives, checksums, SBOMs, cosign signatures, per-arch images + the
# multi-arch manifest, and the GitHub Release with a Conventional-Commits
# changelog. The version DECISION stays outside (svu, in release.yml) —
# GoReleaser releases *from* the tag it finds.
#
# Local dry run: `mise run snapshot` (goreleaser release --snapshot --clean
# --skip=sign, then the SPA embed overlay is restored). Signing is skipped
# locally because keyless cosign needs an OIDC identity (GitHub Actions).
version: 2

project_name: snarvei

# Not the default ./dist: the repo's own build outputs live there
# (dist/client from Vite, dist/server from build-artifacts.sh), and the
# before hook below writes into it — which would trip GoReleaser's
# dist-must-be-empty check even under --clean.
dist: dist/goreleaser

before:
  hooks:
    # The SPA must sit in its go:embed directory before the build runs.
    # Callers restore the overlay afterwards (scripts/restore-embed-overlay.sh)
    # — GoReleaser OSS has no global after-hooks.
    - bash scripts/spa-embed-overlay.sh

builds:
  - id: snarvei
    dir: apps/server
    main: ./cmd/snarvei
    binary: snarvei
    env:
      - CGO_ENABLED=0
    goos: [linux]
    goarch: [amd64, arm64]
    flags: [-trimpath]
    ldflags: ["-s -w -X main.version={{ .Version }}"]

archives:
  - formats: [tar.gz]
    # snarvei_0.1.0_linux_amd64.tar.gz — binary only; the SPA, the OpenAPI
    # spec, the migrations and tzdata are embedded in it.
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}

checksum:
  name_template: checksums.txt

# One SBOM per archive (syft, SPDX JSON) — uploaded to the release.
sboms:
  - artifacts: archive

# Keyless cosign signature over the checksum file: verifying it plus the
# checksums transitively verifies every archive. cosign 3.x signs into a
# single Sigstore bundle (signature + certificate in one JSON file).
# README documents the `cosign verify-blob --bundle` invocation.
signs:
  - cmd: cosign
    artifacts: checksum
    output: true
    signature: "${artifact}.sigstore.json"
    args:
      - sign-blob
      - --yes
      - --bundle=${signature}
      - ${artifact}

# Multi-platform image from the same COPY-only Dockerfile the rest of the
# repo uses. GoReleaser's build context holds the binaries at
# linux/<arch>/snarvei, hence BINARY_ROOT=. (the Dockerfile's default serves
# the scripts/build-artifacts.sh layout). data-skel must ride along
# explicitly — the context is a temp dir, not the repo.
dockers_v2:
  - images:
      - ghcr.io/refsdal/snarvei
    # A pinning ladder: the full version never moves, major.minor moves with
    # patches, major with minors (pre-1.0 minors may break under --v0),
    # latest with every release. sha-<commit> and @digest stay for
    # byte-exact pinning.
    tags:
      - "{{ .Version }}"
      - "{{ .Major }}.{{ .Minor }}"
      - "{{ .Major }}"
      - latest
      - sha-{{ .FullCommit }}
    dockerfile: Dockerfile
    platforms:
      - linux/amd64
      - linux/arm64
    build_args:
      BINARY_ROOT: "."
    extra_files:
      - docker/data-skel

# Keyless cosign signatures on the pushed images/manifests.
docker_signs:
  - cmd: cosign
    artifacts: all
    output: true
    args:
      - sign
      - --yes
      - "${artifact}@${digest}"

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs"
      - "^test"
      - "^chore"
      - "^ci"
      - "^style"
      - "merge conflict"
      - Merge pull request
      - Merge branch
  groups:
    - title: Features
      regexp: '^.*?feat(\([[:word:]]+\))??!?:.+$'
      order: 0
    - title: Bug fixes
      regexp: '^.*?fix(\([[:word:]]+\))??!?:.+$'
      order: 1
    - title: Performance
      regexp: '^.*?perf(\([[:word:]]+\))??!?:.+$'
      order: 2
    - title: Other
      order: 999

release:
  github:
    owner: refsdal
    name: snarvei
  footer: |
    **Container image:** `ghcr.io/refsdal/snarvei:{{ .Version }}` (linux/amd64 + linux/arm64, cosign-signed)

    **Verify a download:** see the "Verifying a release" section of the README.

snapshot:
  version_template: "{{ incpatch .Version }}-snapshot.{{ .ShortCommit }}"
```

Run: `mise exec -- goreleaser check` → "1 configuration file(s) validated". If `dockers_v2`'s schema differs in the pinned 2.18.0 (e.g. `platforms` naming), adapt to what `goreleaser check` accepts and note it.

- [ ] **Step 2: mise tasks and the CI check**

`.mise.toml`: change `[tasks.check]` to `description = "Lint, typecheck, goreleaser config"` / `run = ["bun run check", "goreleaser check"]` and add:

```toml
[tasks.snapshot]
description = "Full GoReleaser dry run: archives, SBOMs, local image (no publish, no signing)"
run = """
trap 'bash scripts/restore-embed-overlay.sh' EXIT
goreleaser release --snapshot --clean --skip=sign
"""
```

`.github/workflows/test.yml`: `install_args: "go bun aqua:goreleaser/goreleaser go:github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen go:github.com/sqlc-dev/sqlc/cmd/sqlc"` and, after "Lint + typecheck":

```yaml
      # The release pipeline's config is code too — a broken .goreleaser.yaml
      # must fail the PR, not the release.
      - name: GoReleaser config
        run: goreleaser check
```

Update the file's header comment: "Reusable: ci.yml (PRs) and release.yml (pushes to main) both call it."

- [ ] **Step 3: Run the snapshot locally**

`mise run snapshot` (needs Docker with buildx; several minutes; do not prune). Expected in `dist/goreleaser/`: `snarvei_<v>-snapshot.<sha>_linux_amd64.tar.gz`, `..._arm64.tar.gz`, `checksums.txt`, two `*.sbom.spdx.json` (or `.sbom.json`) files, and a local image `ghcr.io/refsdal/snarvei:<v>-snapshot.<sha>` (or GoReleaser's note that the multi-platform manifest was built and not loaded). `tar tzf` one archive → `snarvei`. `git status` clean afterwards (overlay restored; `dist/` is git-ignored). Paste the tail in the report. If `dockers_v2` cannot build locally without pushing, run with `--skip=docker` as well and say so.

- [ ] **Step 4: Commit**

```bash
git add .goreleaser.yaml .mise.toml .github/workflows/test.yml
git commit -m "build: GoReleaser config (archives, SBOMs, cosign, multi-arch image), snapshot task, config check in CI"
```

---

### Task 2: The release workflow

**Files:**
- Create: `.github/workflows/release.yml`
- Modify: `.github/workflows/ci.yml` (remove the unused `docker/setup-buildx-action` step — `ci.yml` builds a single-arch image with plain `docker build`; add one comment line pointing at `release.yml` for what happens on merge)

**Interfaces:**
- Produces: `release.yml` per spec §7 and the Global Constraints. Consumes `test.yml` (reusable) and `.goreleaser.yaml` (Task 1).

- [ ] **Step 1: Write `release.yml`**

```yaml
name: Release

# Merging IS releasing: every push to main with releasable commits
# (feat/fix/perf/breaking since the last tag) tags and publishes
# automatically. The PR review that approved the merge is the approval
# gate. Docs/chore/ci-only merges end green without releasing.
#
# The manual dispatch remains for exactly two things:
#   - dry_run: the full pipeline as a snapshot, nothing pushed/tagged/signed
#   - allow_major: permit 1.0.0 — reaching a new major stays a deliberate
#     human act, never a side effect of merging
#
# The version is NOT typed in by hand. svu computes it from Conventional
# Commits since the last v* tag, which is why AGENTS.md mandates the commit
# format — the changelog (GoReleaser) and the version number (svu) are both
# downstream of it.
#
# This workflow TAGS, then GoReleaser BUILDS AND PUBLISHES everything from
# the tag: binaries, archives, checksums, SBOMs, cosign signatures, the
# multi-arch image, and the GitHub Release with changelog. The tag comes
# FIRST because GoReleaser releases from a tag; a failed publish deletes it
# again, so a tag never points at a release that does not exist.
#
# It does not roll out: where the container runs is the operator's. Rolling
# out means `snarvei migrate` (or the default mode, which migrates under an
# advisory lock) and then pointing the deployment at the new tag.

on:
  push:
    branches: [main]
  workflow_dispatch:
    inputs:
      dry_run:
        description: "Compute and build, but do not push or tag"
        type: boolean
        default: true
      allow_major:
        description: "Permit a 1.0.0 release (breaking changes bump minor while the major is 0)"
        type: boolean
        default: false

permissions:
  contents: read

# Rapid merges serialize: the second run computes its version from the tag
# the first one just created.
concurrency:
  group: release
  cancel-in-progress: false

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  version:
    runs-on: ubuntu-latest
    outputs:
      version: ${{ steps.v.outputs.version }}
      tag: ${{ steps.v.outputs.tag }}
      releasable: ${{ steps.v.outputs.releasable }}
      publish: ${{ steps.mode.outputs.publish }}
      dry: ${{ steps.mode.outputs.dry }}
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
      - uses: jdx/mise-action@c2a87611a18de5b3828c5652fe268e992400cb5c # v4.3.0
        with:
          install_args: "aqua:caarlos0/svu"
      # svu reads Conventional Commits since the last v* tag. --v0 keeps
      # breaking changes bumping the minor while the major is 0; the
      # allow_major input drops it to permit 1.0.0.
      - name: Compute version
        id: v
        run: |
          current=$(svu current)
          if [ "${{ inputs.allow_major }}" = "true" ]; then
            tag=$(svu next)
          else
            tag=$(svu next --v0)
          fi
          version="${tag#v}"
          releasable=true
          if [ "$tag" = "$current" ]; then releasable=false; fi
          {
            echo "tag=$tag"; echo "version=$version"; echo "releasable=$releasable"
          } >> "$GITHUB_OUTPUT"
          {
            echo "### Version"
            echo "- current: \`$current\`"
            echo "- next:    \`$tag\` (releasable: $releasable)"
          } | tee "$GITHUB_STEP_SUMMARY"
      # On a push, an unreleasable merge is a normal, green outcome. On a
      # manual dispatch it is a refusal: a human pressing the button expects
      # a release to exist.
      - name: Decide what this run does
        id: mode
        run: |
          dry=false
          if [ "${{ github.event_name }}" = "workflow_dispatch" ] && [ "${{ inputs.dry_run }}" = "true" ]; then
            dry=true
          fi
          publish=false
          if [ "${{ steps.v.outputs.releasable }}" = "true" ] && [ "$dry" = "false" ]; then
            publish=true
          fi
          echo "dry=$dry" >> "$GITHUB_OUTPUT"
          echo "publish=$publish" >> "$GITHUB_OUTPUT"
          if [ "$publish" = "false" ] && [ "$dry" = "false" ]; then
            echo "Nothing releasable since ${{ steps.v.outputs.tag }} — skipping." | tee -a "$GITHUB_STEP_SUMMARY"
          fi
      - name: Refuse an empty manual release
        if: github.event_name == 'workflow_dispatch' && steps.v.outputs.releasable != 'true' && steps.mode.outputs.dry != 'true'
        run: |
          echo "No feat/fix/perf/breaking commits since the last tag — nothing to release."
          exit 1

  # The merge commit is not the PR head the PR's CI tested, so the suite
  # runs once more on exactly what ships.
  verify:
    needs: version
    if: needs.version.outputs.publish == 'true' || needs.version.outputs.dry == 'true'
    uses: ./.github/workflows/test.yml
    permissions:
      contents: read

  publish:
    needs: [version, verify]
    if: needs.version.outputs.publish == 'true' || needs.version.outputs.dry == 'true'
    runs-on: ubuntu-latest
    permissions:
      contents: write   # the git tag + the GitHub Release
      packages: write   # ghcr.io
      id-token: write   # keyless cosign signing (GitHub OIDC)
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
      - uses: jdx/mise-action@c2a87611a18de5b3828c5652fe268e992400cb5c # v4.3.0
        with:
          install_args: "go bun aqua:goreleaser/goreleaser aqua:sigstore/cosign aqua:anchore/syft"
      - uses: actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9 # v6.1.0
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
          key: ${{ runner.os }}-go-${{ hashFiles('apps/server/go.sum') }}
          restore-keys: ${{ runner.os }}-go-
      # GoReleaser's before hook builds the SPA; that needs node_modules.
      - name: Install
        run: bun install --frozen-lockfile
      # dockers_v2 builds both platforms with an SBOM attestation in one
      # buildx run — that needs the docker-container driver.
      - uses: docker/setup-buildx-action@37fe631027851001ddb9b187196cc803df7f5f0e # v4.3.0
      - name: Log in to the container registry
        if: needs.version.outputs.publish == 'true'
        uses: docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      # Dry run: the full pipeline with nothing pushed, tagged or signed.
      - name: GoReleaser (snapshot)
        if: needs.version.outputs.dry == 'true'
        run: |
          trap 'bash scripts/restore-embed-overlay.sh' EXIT
          goreleaser release --snapshot --clean --skip=sign
      # GoReleaser auto-skips signing in snapshot mode; exercise the same
      # sign + verify invocations on a throwaway blob so the dry run proves
      # the cosign flags too (OIDC is available here; nothing uploads).
      - name: Cosign flags smoke
        if: needs.version.outputs.dry == 'true'
        run: |
          echo smoke > /tmp/cosign-smoke.txt
          cosign sign-blob --yes --bundle=/tmp/cosign-smoke.sigstore.json /tmp/cosign-smoke.txt
          cosign verify-blob \
            --bundle /tmp/cosign-smoke.sigstore.json \
            --certificate-identity-regexp "https://github.com/${{ github.repository }}/\.github/workflows/release\.yml@.*" \
            --certificate-oidc-issuer https://token.actions.githubusercontent.com \
            /tmp/cosign-smoke.txt
      # The real thing. GoReleaser releases FROM a tag, so the tag is
      # created first — and deleted again below if anything after it fails.
      - name: Tag the release
        if: needs.version.outputs.publish == 'true'
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git tag -a "${{ needs.version.outputs.tag }}" -m "${{ needs.version.outputs.tag }}"
          git push origin "${{ needs.version.outputs.tag }}"
      - name: GoReleaser (publish)
        if: needs.version.outputs.publish == 'true'
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          trap 'bash scripts/restore-embed-overlay.sh' EXIT
          goreleaser release --clean
      - name: Delete the tag on failure
        if: failure() && needs.version.outputs.publish == 'true'
        run: |
          gh release delete "${{ needs.version.outputs.tag }}" --yes || true
          git push --delete origin "${{ needs.version.outputs.tag }}" || true
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      - name: Summary
        run: |
          {
            if [ "${{ needs.version.outputs.dry }}" = "true" ]; then
              echo "### Dry run — nothing pushed, nothing tagged"
            else
              echo "### Released ${{ needs.version.outputs.tag }}"
            fi
            echo
            echo "- version: \`${{ needs.version.outputs.version }}\`"
            echo "- image:   \`${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:${{ needs.version.outputs.version }}\` (amd64+arm64, cosign-signed)"
            echo "- release: binaries, checksums, SBOMs and signatures on the GitHub Release"
          } >> "$GITHUB_STEP_SUMMARY"
```

The action SHAs above are the ones `ci.yml`/`test.yml` already pin; reuse those exact lines (copy from the existing workflows) so Dependabot's grouped action bumps stay consistent.

- [ ] **Step 2: `ci.yml` tidy**

Remove the `docker/setup-buildx-action` step (unused: `docker build` builds one arch). Add above the `image` job: `# Merging the PR then releases automatically — see release.yml.`

- [ ] **Step 3: Validate and commit**

Both workflows must parse: `bunx --package yaml@2 yaml --help >/dev/null 2>&1 || true; python3 -c "import yaml,sys; [yaml.safe_load(open(f)) for f in ['.github/workflows/release.yml','.github/workflows/ci.yml','.github/workflows/test.yml']]; print('ok')"`. Then `svu current` and `svu next --v0` from the repo root (expect `v0.0.0` and `v0.1.0`) and paste the output. `bun run check` (biome ignores workflows, but run it).

```bash
git add .github/workflows/release.yml .github/workflows/ci.yml
git commit -m "ci: release workflow (svu version, tag, GoReleaser publish, tag rollback on failure); drop unused buildx step from ci.yml"
```

---

### Task 3: Playwright support module and the remaining browser flows

**Files:**
- Create: `e2e/support.ts`, `e2e/flows.spec.ts`
- Modify: `e2e/auth-api.spec.ts`, `e2e/links-api.spec.ts`, `e2e/app.spec.ts` (import helpers from `./support`; replace every `mail.messages[0]` read with `lastMailTo`; delete every `request.delete("/api/_test/mail")` call)

**Interfaces:**
- Produces (`e2e/support.ts`):
  ```ts
  export const PASSWORD = "Playwright123";
  export const ORIGIN: string;                          // process.env.E2E_BASE_URL ?? "http://127.0.0.1:3300"
  export const headers: { origin: string; "content-type": string };
  export const unique: () => string;
  export async function signUpApi(request: APIRequestContext, name: string, email: string): Promise<void>;   // retry on 429
  export async function signUpUi(page: Page, name: string, email: string): Promise<void>;                     // landing form, retry on the throttle alert
  export async function workspace(request: APIRequestContext): Promise<{ orgId: string; slug: string; teamId: string; ownerEmail: string }>;  // sign up owner, create org (unique slug), switch, create team "Marketing"
  export async function lastMailTo(request: APIRequestContext, email: string): Promise<{ to: string; subject: string; text: string }>;  // polls GET /api/_test/mail up to 10 s for the newest message to `email`
  export async function createOrganizationUi(page: Page, name: string, slug: string): Promise<void>;
  ```

- [ ] **Step 1: `support.ts`**

Move the existing `signUp` (API, retry-on-429) from `auth-api.spec.ts` and the `signUp`/`createOrganization` (UI) helpers from `app.spec.ts` into `support.ts` as `signUpApi`/`signUpUi`/`createOrganizationUi`; `workspace` from `links-api.spec.ts` (extend it to return `slug` and `ownerEmail`). `lastMailTo`:

```ts
export async function lastMailTo(request: APIRequestContext, email: string) {
  const deadline = Date.now() + 10_000;
  for (;;) {
    const res = await request.get("/api/_test/mail");
    const { messages } = (await res.json()) as { messages: { to: string; subject: string; text: string }[] };
    const mine = messages.find((m) => m.to === email);   // newest first
    if (mine) return mine;
    if (Date.now() > deadline) throw new Error(`no mail for ${email}`);
    await new Promise((r) => setTimeout(r, 250));
  }
}
```

Refactor the three existing specs onto these helpers; every assertion stays; no spec calls `DELETE /api/_test/mail` any more. Run `bun run check`.

- [ ] **Step 2: `flows.spec.ts`**

```ts
import { expect, test } from "@playwright/test";
import { createOrganizationUi, lastMailTo, PASSWORD, signUpApi, signUpUi, unique, workspace, headers } from "./support";

test("a member sees only their team's links, and an admin can add them to another team", async ({ page, request, browser }) => {
  // Owner (API): org with teams Marketing (from workspace) and Sales; one link in each.
  const ws = await workspace(request);
  const sales = await (await request.post(`/api/organizations/${ws.orgId}/teams`, { headers, data: { name: "Sales" } })).json();
  await request.post("/api/links", { headers, data: { teamId: ws.teamId, targetUrl: "https://example.com/marketing", title: "Marketing launch" } });
  await request.post("/api/links", { headers, data: { teamId: sales.id, targetUrl: "https://example.com/sales", title: "Sales deck" } });
  // Invite a member into Marketing only.
  const member = `member-${unique()}@example.com`;
  const inv = await (await request.post(`/api/organizations/${ws.orgId}/invitations`, { headers, data: { email: member, role: "member", teamId: ws.teamId } })).json();
  // The member has an account already: sign up in the browser, then accept through the emailed link.
  await signUpUi(page, "Member", member);
  const mail = await lastMailTo(request, member);
  const link = mail.text.match(/\/app\/invitations\/[A-Za-z0-9-]+/)?.[0];
  expect(link).toBe(`/app/invitations/${inv.id}`);
  await page.goto(link!);
  await expect(page.getByTestId("invitation-organization")).toHaveText("Acme");
  await page.getByTestId("invitation-accept-button").click();
  await page.waitForURL(/\/app(\?|$|\/)/);
  await page.getByRole("button", { name: "Open workspace" }).click();
  await page.waitForURL(`**/app/${ws.slug}/dashboard`);
  await expect(page.getByTestId("dashboard-links-count")).toHaveText("1");
  await page.goto(`/app/${ws.slug}/links`);
  await expect(page.getByText("Marketing launch")).toBeVisible();
  await expect(page.getByText("Sales deck")).toHaveCount(0);
  await expect(page.getByTestId("links-team-filter")).toHaveCount(0);   // one team: no filter

  // Owner (browser, second context) adds the member to Sales through the team dialog.
  const ownerCtx = await browser.newContext();
  const owner = await ownerCtx.newPage();
  await owner.goto("/");
  await owner.getByTestId("auth-email-input").fill(ws.ownerEmail);
  await owner.getByTestId("auth-password-input").fill(PASSWORD);
  await owner.getByTestId("sign-in-button").click();
  await owner.waitForURL(/\/app/);
  await owner.goto(`/app/${ws.slug}/organization`);
  await owner.getByTestId("manage-team-Sales").click();
  await owner.getByTestId("add-team-member-select").click();
  await owner.getByTestId(`add-team-member-option-${member}`).click();
  await owner.getByTestId("add-team-member-button").click();
  await expect(owner.getByTestId("team-members-list")).toContainText(member);
  await ownerCtx.close();

  // The member now sees both links.
  await page.reload();
  await expect(page.getByText("Sales deck")).toBeVisible();
  await expect(page.getByTestId("links-team-filter")).toBeVisible();
});

test("profile name update is reflected in the shell", async ({ page }) => {
  const email = `kari-${unique()}@example.com`;
  await signUpUi(page, "Kari", email);
  await page.goto("/app/settings");
  await page.getByTestId("settings-name-input").fill("Kari Nordmann");
  await page.getByRole("button", { name: /save/i }).first().click();
  await expect(page.getByRole("alert")).toContainText(/updated|saved/i);
  await page.reload();
  await expect(page.getByText("Kari Nordmann")).toBeVisible();
});

test("forgot-password flow through the mailbox", async ({ page, request }) => {
  const email = `reset-${unique()}@example.com`;
  await signUpApi(request, "Ola", email);
  await page.goto("/");
  await page.getByRole("button", { name: /forgot/i }).click();
  await page.getByTestId("forgot-password-email-input").fill(email);
  await page.getByTestId("forgot-password-button").click();
  const mail = await lastMailTo(request, email);
  const url = mail.text.match(/\/reset-password\?token=[^\s]+/)?.[0];
  expect(url).toBeTruthy();
  await page.goto(url!);
  await page.getByTestId("reset-password-input").fill("Newpass456!");
  await page.getByTestId("reset-password-confirm-input").fill("Newpass456!");
  await page.getByTestId("reset-password-button").click();
  await page.waitForURL(/\/\?reset=done/);
  await page.getByTestId("auth-email-input").fill(email);
  await page.getByTestId("auth-password-input").fill("Newpass456!");
  await page.getByTestId("sign-in-button").click();
  await page.waitForURL(/\/app/);
});

test("the Scalar page renders the API reference", async ({ page }) => {
  const res = await page.goto("/scalar");
  expect(res?.status()).toBe(200);
  await expect(page.locator("script#api-reference")).toBeAttached();
  // The bundle comes from cdn.jsdelivr.net (allowed by the page's own CSP);
  // give it time, then expect the spec's title to be rendered.
  await expect(page.getByText("Snarvei API").first()).toBeVisible({ timeout: 20_000 });
});
```

Adapt selectors to the real markup (the profile "Save" button label, the "Forgot password" control on the landing page — read `routes/landing/page.tsx` and `routes/settings/components/profile-section.tsx`); never weaken an assertion. Note: `signUpUi` leaves the page signed in; the first test relies on that (the member is signed in when opening the invitation link).

- [ ] **Step 3: Run, commit**

```bash
bun run check
E2E_REBUILD=1 mise run e2e 2>&1 | tail -15     # expect 23 passed (7 smoke + 4 auth-api + 4 links-api + 4 app + 4 flows)
git add e2e
git commit -m "test(e2e): shared support module with recipient-keyed mailbox reads; team scoping, profile, forgot-password and Scalar flows"
```

If the Scalar test fails only because the CDN is unreachable from the machine, say so in the report (the assertion stays).

---

### Task 4: Frontend deferred minors

**Files:**
- Modify: `apps/frontend/src/router.tsx`, every page under `apps/frontend/src/routes/**` that imports a route object from `../../router` (landing, reset-password, links, link-details, invitation, settings, dashboard, organization — grep `from "../../router"`), `apps/frontend/src/components/route-error.tsx`, `apps/frontend/src/routes/link-details/page.tsx`, `apps/frontend/src/components/app-shell.tsx`

- [ ] **Step 1: Break the import cycle with `getRouteApi`**

In each page replace `import { xRoute } from "../../router"` + `xRoute.useSearch()/useParams()/useRouteContext()` with `const route = getRouteApi("/app/$org/links")` (the route's full path id: `"/"`, `"/reset-password"`, `"/app/invitations/$invitationId"`, `"/app/settings"`, `"/app/$org"` for the org context, `"/app/$org/links"`, `"/app/$org/links/$linkId"`) and `route.useSearch()` etc. `router.tsx` keeps exporting the routes (tests/other modules may use them) but no page imports from it any more: `grep -rn 'from "../../router"\|from "../router"' apps/frontend/src/routes apps/frontend/src/components` → nothing. `bun run check` must pass (the `Register` module augmentation makes `getRouteApi` typed).

- [ ] **Step 2: `NotFound` inside the shell; drawer select**

`route-error.tsx`: `ErrorLayout`/`NotFound`/`RouteError` take `fullScreen?: boolean` (default `true`); `link-details/page.tsx` renders `<NotFound fullScreen={false} />`. `app-shell.tsx`: the organization `Select`'s `onChange` only navigates (`navigate({ to: "/app/$org/dashboard", params: orgParams(next) })`); remove the `useSwitchOrganization` call there (the org route switches server-side).

- [ ] **Step 3: Verify, commit**

`bun run check && bun run test && bun run build`; then `E2E_REBUILD=1 mise run e2e` once more (the shell and every page changed): 23 passed.

```bash
git add apps/frontend
git commit -m "refactor(frontend): pages use getRouteApi; NotFound fits the shell; drawer select navigates only"
```

---

### Task 5: Documentation rewrite

**Files:**
- Rewrite: `AGENTS.md`, `README.md`, `docs/runbook.md`
- Create: `CONTRIBUTING.md`
- Remove the "Migration in progress" banner from every file that carries one (`grep -rln "Migration in progress"`).

Facts to draw on (read them; do not invent): spec §7 and §8, `.env.example`, `docker-compose.selfhost.yml`, `docker-compose.yml`, `docker-compose.test.yml`, `.mise.toml`, `scripts/*.sh`, `apps/server/cmd/snarvei/main.go` (dispatch modes), `apps/server/internal/auth/routes.go` (the Limen allowlist), `apps/server/internal/api/api.go` (the `Deps` struct), `openapi/snarvei.yaml`, `apps/frontend/src/router.tsx` (route table), `e2e/*.spec.ts` (what is covered), `.github/workflows/*.yml`. Pjokk's README/CONTRIBUTING (`~/projects/refsdal/pjokk`) are the structural model; copy structure and tone, never Pjokk-specific facts (no Google sign-in, no VAPID, no landing site, no scheduled work, no family).

- [ ] **Step 1: `README.md`**

Keep the product text (the intro list, "What It Is", "V1 Scope" with "advanced analytics infrastructure beyond D1" → "advanced analytics infrastructure", "Product Rules"). Replace everything from "Architecture" down with these sections, in this order: **Architecture** (one Go binary serving the embedded SPA and the API; Postgres; Limen for auth; the route table: `/`, `/reset-password`, `/app`, `/app/invitations/{id}`, `/app/settings`, `/app/{org}/dashboard|links|links/{id}|organization`, plus `/l/{slug}`, `/api/*`, `/healthz`, `/readyz`, `/openapi.json`, `/scalar`, `/images/profile/*`, `/robots.txt`), **Quick start** (`docker-compose.selfhost.yml` one-liner with `AUTH_SECRET=$(openssl rand -base64 32)`, first account with `OPEN_SIGNUP`, where to sign in), **Self-hosting** (image name, tag ladder table, what is inside the image, **Configuration** table split into required and optional exactly per spec §8 with the notes from `.env.example`, **Email**, **Behind a proxy** (`TRUSTED_PROXY_HOPS`, Cloudflare = 1, `CF-IPCountry`), **Upgrading** (run `migrate` or rely on default mode; then point at the new tag; migrations forward-only), **Backups** (both volumes: Postgres and `snarvei-data`; S3 alternative), **Where your data lives**), **Versioning and images** (svu, `--v0`, preview images `<next>-pr.<n>`, merge-is-release, tags carry no `v`, local build commands `scripts/build-artifacts.sh`, `docker build`, `scripts/build-image.sh`, `mise run snapshot`), **Running without Docker** (the bare binary from the release), **Verifying a release** (the three cosign commands with `refsdal/snarvei` and the release.yml identity regexp), **Development** (`mise install`, `bun install`, compose test Postgres on 55432 with DB `snarvei_test`, `bun run dev` + `bun run dev:server` with `APP_URL=http://localhost:5173`, `mise run test|check|e2e|snapshot`, `go generate` and `bun run gen:client`, the drift guards), **API and Scalar** (spec-first, `/openapi.json`, `/scalar`), **Repository layout** (tree of `apps/server`, `apps/frontend`, `openapi/`, `e2e/`, `scripts/`, `docker/`, `docs/`, workflows), **CI/CD** (ci.yml, test.yml, release.yml in two paragraphs), **License** (AGPL-3.0).

- [ ] **Step 2: `AGENTS.md`**

Rewrite as the contributor deep-dive for agents and humans: Product summary (keep), **Locked decisions** table (from the spec's "Decisions taken during brainstorming": Go + stdlib net/http, Postgres via pgx/sqlc/goose, Limen behind `internal/auth` with the allowlist, teams are Snarvei's own tables, TanStack Router/Query + openapi-fetch + limen-auth, bun + biome, SMTP, storage port fs/s3, distroless static image built natively, merge-is-release, AGPL-3.0, behind a trusted proxy via `TRUSTED_PROXY_HOPS`), **The Deps rule** (handlers are methods on `api.Deps`; `NewHandler` asserts every collaborator; tests build `Deps` through `internal/testrig`), **The Limen boundary** (only `internal/auth` imports Limen; the allowlist in `routes.go`; sessions/orgs/invitations are Snarvei routes; sign-up throttle; in-memory auth rate limiter per replica), **Spec-first workflow** (`openapi/snarvei.yaml` → `go generate` → strict server + embedded copy + `bun run gen:client`; drift guards; `tiers.go` coverage assertion), **Migrations under goose** (`internal/db/migrations`, embedded, applied under an advisory lock at startup or via `snarvei migrate`, forward-only, sqlc queries in `internal/db/queries`), **Authorization rules** (keep the existing table, updated wording), **Redirect and analytics privacy** (what a click row stores), **Frontend conventions** (session truth = `['me']`, query keys, typed navigation with `orgParams`, `getRouteApi`, test ids are a contract with `e2e/`), **Testing expectations** (Go against real Postgres `-p 1`; bun tests; Playwright against the image; the mailbox discipline), **Release model and Conventional Commits** (svu/GoReleaser; commit types decide the version; trailers policy is the session's, not the repo's — do not add trailer rules), **Operations pointers** (runbook). Delete every Cloudflare/D1/Drizzle/Better Auth/Vitest/wrangler section.

- [ ] **Step 3: `docs/runbook.md` and `CONTRIBUTING.md`**

Runbook: **Where it runs** ("wherever the container runs"; the image and tag ladder; one process; `/healthz` liveness, `/readyz` readiness, `PORT`), **Configuration and secrets** (environment variables; link to `.env.example`; rotating `AUTH_SECRET` signs everyone out and changes IP hashes unless `IP_HASH_PEPPER` is set), **Deploy and verify** (`snarvei migrate` then the new tag; `curl /healthz` shows the version; open `/scalar`; follow a short link → 302 + `no-store`), **Rollback** (previous tag; migrations forward-only; Postgres backup/restore is the operator's; `snarvei-data` volume), **Common failures** table updated to the Go log events: config errors at boot (all at once), `/readyz` 503 (Postgres), `email.not_configured`/`email.send_failed`, 429s (`Retry-After`; auth limits per replica), `click.record_failed`, `request.error`, `click.drain_timeout`), **Where to look** (container logs, filter by `event`; `/healthz` version), **Local development quickstart** (mise/bun/compose commands).

`CONTRIBUTING.md`: Pjokk's structure with Snarvei facts: ground rules (discuss features first; Conventional Commits are load-bearing; generated code is committed — `go generate ./...` from `apps/server` and `bun run gen:client`), getting set up (`mise install`, `bun install`, `docker compose -f docker-compose.test.yml up -d`, `mise run test|check|e2e`), running locally (`bun run dev` + `bun run dev:server`, `OPEN_SIGNUP=1`), notes that bite (`-p 1`; real Postgres; COPY-only image needs `scripts/build-artifacts.sh` first; `APP_URL` must equal the browser origin), pull requests (CI, preview image `ghcr.io/refsdal/snarvei:<next>-pr.<number>`).

- [ ] **Step 4: Verify, commit**

`grep -rn "Migration in progress\|Cloudflare Workers\|Better Auth\|Drizzle\|wrangler\|Vitest\|pnpm" README.md AGENTS.md docs/runbook.md CONTRIBUTING.md` → only historical mentions allowed ("moved off Cloudflare Workers in 2026-09" style), no instructions. `bun run check` (biome does not lint markdown; run it anyway). Every command in the docs must exist (`mise run <task>` names in `.mise.toml`, script names in `scripts/`).

```bash
git add README.md AGENTS.md docs/runbook.md CONTRIBUTING.md
git commit -m "docs: rewrite README, AGENTS, runbook and CONTRIBUTING for the Go/container era"
```

Do not push; the controller runs the whole-branch review and opens the PR (base `feat/go-migration-phase-4` until #82 merges).

---

## Self-review

**Spec coverage:** §7 build/image/release — `.goreleaser.yaml` (T1), `release.yml` (T2), `test.yml` `goreleaser check` (T1), `ci.yml` unchanged in behaviour (T2 tidy); §9 Playwright list — T3 adds the missing flows and the mailbox discipline; §10 — T5 rewrites all four documents; phase-4 deferred frontend minors — T4; section 11 phase 5 "the first merge produces v0.1.0" — Global Constraints + T2's svu check.

**Deviations decided here:** (1) no `environment:` block in `release.yml` (the repo's old Workers-era `Dev`/`Production` environments would gate releases; the spec does not ask for a gate); (2) `openapi-typescript` stays on `bunx --package` (TypeScript hoisting); (3) `ci.yml` loses the unused buildx step (phase-1 deferred minor); (4) the Scalar browser test depends on the CDN being reachable.

**Placeholder scan:** none; T5 lists the sections and the facts' sources rather than prose, which is the deliverable the implementer writes.

**Type consistency:** `e2e/support.ts` exports (T3) match their uses in `flows.spec.ts` and the refactored specs; `getRouteApi` ids in T4 match the route paths registered in `router.tsx` (phase 4); `mise run snapshot` (T1) is referenced by README (T5) and `release.yml` mirrors its command (T2).
