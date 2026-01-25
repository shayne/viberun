# Unify Dev + Release Workflow Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Publish dev and prod artifacts from a single trusted workflow file so npm OIDC works for both channels.

**Architecture:** Merge dev and prod workflows into `.github/workflows/release.yml`, with branch/tag triggers and `if:` gating for dev vs prod jobs. A `meta` job computes versions and booleans used by downstream jobs.

**Tech Stack:** GitHub Actions, Go toolchain via mise, npm OIDC trusted publishing, PyPI OIDC.

### Task 1: Add unified triggers and meta job

**Files:**
- Modify: `.github/workflows/release.yml`

**Step 1: Edit workflow triggers to include main pushes**

```yaml
on:
  push:
    branches:
      - "main"
    tags:
      - "v*"
```

**Step 2: Add a `meta` job that computes version outputs and flags**

```yaml
jobs:
  meta:
    runs-on: ubuntu-latest
    outputs:
      is_tag: ${{ steps.meta.outputs.is_tag }}
      is_main: ${{ steps.meta.outputs.is_main }}
      version: ${{ steps.meta.outputs.version }}
      pypi_version: ${{ steps.meta.outputs.pypi_version }}
    steps:
      - id: meta
        run: |
          ref="${GITHUB_REF}"
          is_tag=false
          is_main=false
          if [ "${GITHUB_REF_TYPE:-}" = "tag" ]; then is_tag=true; fi
          if [ "${GITHUB_REF_NAME:-}" = "main" ]; then is_main=true; fi
          ts=$(date -u +%Y%m%d%H%M%S)
          short_sha="${GITHUB_SHA::7}"
          if [ "$is_tag" = true ]; then
            version="${GITHUB_REF_NAME}"
            pypi_version="${GITHUB_REF_NAME#v}"
          else
            version="0.0.0-dev.${ts}+${short_sha}"
            pypi_version="0.0.0.dev${ts}"
          fi
          echo "is_tag=$is_tag" >> "$GITHUB_OUTPUT"
          echo "is_main=$is_main" >> "$GITHUB_OUTPUT"
          echo "version=$version" >> "$GITHUB_OUTPUT"
          echo "pypi_version=$pypi_version" >> "$GITHUB_OUTPUT"
```

**Step 3: Wire build jobs to `needs: [meta]` and use `needs.meta.outputs.version`**

```yaml
needs: [meta]
run: |
  VERSION=${{ needs.meta.outputs.version }}
```

**Step 4: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "release: add unified dev/prod triggers"
```

### Task 2: Split publish jobs with dev/prod gating

**Files:**
- Modify: `.github/workflows/release.yml`

**Step 1: Gate the existing release/publish jobs to tags**

```yaml
if: needs.meta.outputs.is_tag == 'true'
```

**Step 2: Add dev publish jobs gated to main**

```yaml
if: needs.meta.outputs.is_main == 'true'
```

**Step 3: Dev release behavior**

```yaml
- name: Update dev tag
  run: |
    git config user.name "github-actions[bot]"
    git config user.email "github-actions[bot]@users.noreply.github.com"
    git tag -f dev "$GITHUB_SHA"
    git push -f origin refs/tags/dev
```

```yaml
- name: Publish dev release
  uses: softprops/action-gh-release@v2
  with:
    tag_name: dev
    name: Dev
    prerelease: true
    make_latest: false
```

**Step 4: Dev npm/pypi/image settings**

```yaml
npm publish ./dist/npm --access public --tag dev
```

```yaml
env:
  IMAGE: ghcr.io/${{ github.repository }}/viberun:dev
```

```yaml
PYPI_PACKAGE_NAME=viberun-dev
PYPI_VERSION=${{ needs.meta.outputs.pypi_version }}
```

**Step 5: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "release: split dev/prod publish jobs"
```

### Task 3: Remove dev.yml and keep npm trusted publishing single-file

**Files:**
- Delete: `.github/workflows/dev.yml`

**Step 1: Remove the dev workflow file**

```bash
rm .github/workflows/dev.yml
```

**Step 2: Commit**

```bash
git add .github/workflows/dev.yml
git commit -m "release: remove dev workflow file"
```

### Task 4: Validate workflow locally and stage for review

**Files:**
- Modify: `.github/workflows/release.yml`

**Step 1: Sanity-check job conditions and outputs**

```bash
rg -n "is_tag|is_main|meta.outputs" .github/workflows/release.yml
```

**Step 2: Show the final workflow diff**

```bash
git status -sb
git diff --stat
```

**Step 3: Commit final fixups (if any)**

```bash
git add .github/workflows/release.yml
git commit -m "release: unify dev/prod workflow gating"
```
