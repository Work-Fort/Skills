---
name: go-github-workflows
description: GitHub Actions workflow templates for Go services using Mise. Use when setting up CI/CD, Docker publishing, releases, pre-releases, or monorepo package tagging. Use when user says "set up CI", "create GitHub workflow", "add release pipeline", "publish Docker image", "pre-release build", or "monorepo tagging". Includes CI, GHCR Docker builds, cross-compiled binary releases, pre-release from any branch, and monorepo workflows for Go modules and npm packages.
license: MIT
metadata:
  author: Kaz Walker
  version: "1.0"
---

# Go GitHub Workflows

GitHub Actions workflows for Go services using Mise for toolchain management.

All workflows assume `mise.toml` pins the Go version and `.mise/tasks/` contains namespaced tasks (`build:go`, `test:unit`, `lint:go`, etc.). See go-service-architecture skill for the full task layout.

## Conventions

- Default branch is `master`
- Auto-tagging via `Work-Fort/github-tag-action@v6.3` using conventional commits
- Binaries compressed with `xz -9`, checksummed with SHA256
- Windows binaries compressed with `zip`
- Actions: `checkout@v6`, `setup-go@v5`, `mise-action@v3`
- Cross-compilation with `CGO_ENABLED=0` for all platforms

## Single-Service Workflows

| File | Trigger | Purpose |
|------|---------|---------|
| `ci.yml` | Push/PR to master | Lint, test, build |
| `docker-publish.yml` | Version tag (`v*`) | Build and push Docker image to GHCR |
| `release.yml` | Push to master | Auto-tag, cross-compile, create GitHub release with assets |
| `pre-release.yml` | Manual (workflow_dispatch) | Same as release but from any branch, marked pre-release |

## Monorepo Workflows

| File | Use case | Tag pattern |
|------|----------|-------------|
| `monorepo-release-go-module.yml` | Go library in a monorepo | `go/<pkg>/v1.2.3` |
| `monorepo-release-npm.yml` | TypeScript package in a monorepo | `sdk/ts/<pkg>-v1.2.3` |

### Monorepo Tagging Strategy

Each independently releasable package gets its own workflow with:

1. **Path filter** on `push` — only triggers when that package's files change
2. **Scoped `tag_prefix`** — tells the tag action which tags belong to this package
3. **Scoped `paths`** — tells the tag action which commits to consider for bumping

The `Work-Fort/github-tag-action@v6.3` (fork of `mathieudutour/github-tag-action` with `paths` support) looks at conventional commits that touch the specified paths and bumps the version for that prefix independently.

### Go Module Tags

Go requires that submodule tags match the directory path within the repo. For a module at `go/mypackage/`, the tag must be `go/mypackage/v1.2.3`. This is what `go get` resolves:

```
go get github.com/Org/Repo/go/mypackage@go/mypackage/v1.2.3
```

Each Go submodule needs its own `go.mod` in its directory.

### npm Package Tags

npm packages use a descriptive prefix: `sdk/ts/<name>-v1.2.3`. The prefix is arbitrary (npm doesn't resolve from git tags), but the convention keeps tags readable. The workflow extracts the version via `sed`, sets it in `package.json`, and publishes with `--provenance`.

### Example: Mixed Go + TypeScript Monorepo

A project with a Go service, Go SDK, and TypeScript SDK uses three workflows:

| Component | Workflow | Tag prefix | Path filter |
|-----------|----------|------------|-------------|
| Service | `release.yml` | `v` | `paths-ignore: packages/**, go/**` |
| Go SDK | `release-sdk-go.yml` | `go/mypackage/v` | `paths: go/mypackage/**` |
| TS SDK | `release-sdk-ts.yml` | `sdk/ts/mypackage-v` | `paths: packages/mypackage/**` |

The main service release uses `paths-ignore` to exclude SDK directories, while SDK releases use `paths` to include only their directory.

## Use Cases

### Testing a feature branch before merge

A product manager or QA engineer wants a build from an in-progress feature branch. Go to the repo's **Actions** tab → **Pre-release** → **Run workflow**. Enter the feature branch name (e.g. `feature/new-auth`) and hit run. The workflow auto-generates a tag from the branch name and timestamp (e.g. `v0.0.0-feature-new-auth.20260410T153045`), cross-compiles binaries for all platforms, and creates a GitHub pre-release with downloadable assets. No manual tag needed — you can trigger multiple builds from the same branch in the same day.

Workflow: `pre-release.yml`

## Setup

1. Copy the needed workflow files from `scripts/` into `.github/workflows/`
2. Replace `myservice` with your binary name in `BINARY_NAME` env var
3. Replace `mypackage` with your package name in monorepo workflows
4. Ensure `mise.toml` exists at repo root with Go version pinned
5. For Docker workflows: ensure `Dockerfile` exists at repo root

## Build Targets

Release and pre-release workflows build for:

| OS | Arch | Format |
|----|------|--------|
| linux | amd64 | `.xz` |
| linux | arm64 | `.xz` |
| darwin | amd64 | `.xz` |
| darwin | arm64 | `.xz` |
| windows | amd64 | `.zip` |

## Workflow Details

See the workflow files in `scripts/` for the full implementations:
- @scripts/ci.yml — CI pipeline
- @scripts/docker-publish.yml — Docker image publishing
- @scripts/release.yml — Release with cross-compiled binaries
- @scripts/pre-release.yml — Pre-release from any branch
- @scripts/monorepo-release-go-module.yml — Go module release in a monorepo
- @scripts/monorepo-release-npm.yml — npm package release in a monorepo
