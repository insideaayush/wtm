# wtm (worktree manager)

`wtm` keeps repo-local config files (.env, .env.*) synchronized across your main checkout and any associated worktrees by storing the authoritative copies in a personal cache and letting each worktree link back to them.

## What it does
- Lists active worktrees (1-indexed) so you can pick the one that should receive the configs.
- Copies every match from your repo into `~/.wtm/configs/<repo>/<worktree>/…`, preserving the same relative tree as the main repository.
- Replaces the copies inside the chosen worktree with symlinks to the cached files so edits affect the central store.
- Offers `wtm push` to copy those saved files back into the repo so you can commit any changes you made in a worktree.

## Persistent cache
The shared store lives under `~/.wtm/configs/<repo>/<worktree>/`, where `<repo>` is the base name of the git root and `<worktree>` is a sanitized version of the worktree path. Every synced file keeps its relative path (e.g. `apps/api/.env`) so you can reason about the cache just as you would about the repo tree.

## Commands
### `wtm sync`
- Copies the configured files from the repo into the cache and replaces them inside the selected worktree with symlinks to the cached copy.
- When you edit a linked file in the worktree, the change lands in the store automatically.

### `wtm push`
- Copies files from `~/.wtm/configs/<repo>/<worktree>/…` back into the repo so you can stage and commit updates you made via a worktree.
- Honors the same include/exclude filters as `sync` and prompts before overwriting unless `--force` is supplied.

### `wtm version`
- Prints the embedded version string that was baked in by `make build-local` or `make build-release`.

## Config
Create `.worktree-manager.yml` at your repo root (optional).

Defaults if missing:
- include: `.env`, `.env.*`, `**/.env`, `**/.env.*`
- exclude: `**/*.example*`, `**/node_modules/**`, `**/.git/**`

Example:

```yaml
include:
  - .env
  - .env.*
  - apps/*/.env
exclude:
  - "**/*.example*"
  - "**/node_modules/**"
```

## Usage
From inside a git repo:

```bash
wtm sync
```

To reuse a specific worktree without prompts:

```bash
wtm sync --worktree 2 --yes --force
```

Push cached files back into the repo after editing them inside a worktree:

```bash
wtm push --worktree 2
```

Confirm the embedded version matches `VERSION`:

```bash
wtm version
```

## Installation
Releases are published to GitHub, and the binaries are built with `ldflags` so every artifact embeds the canonical version from the `VERSION` file. Once a release is published you can distribute it through Homebrew or apt:

### Homebrew
1. Keep a tap (e.g. `insideaayush/homebrew-wtm`) that points `url`/`sha256` to the tarball/zip for the current release.
2. After `make release` completes, update the formula with the new `version`, `url`, and `sha256`, then publish the tap.
3. Users install with `brew tap insideaayush/wtm` and `brew install insideaayush/wtm/wtm` (or `brew upgrade insideaayush/wtm/wtm`).

### Debian/Ubuntu (APT)
1. Publish a `.deb` built from the release artifacts (e.g. using `dpkg-deb` or `fpm`) and upload it as part of the GitHub release.
2. Host those `.deb`s in an apt repo or download them directly via `curl -LO https://github.com/insideaayush/wtm/releases/download/v<version>/wtm_<version>_amd64.deb`.
3. Users install with `sudo dpkg -i wtm_<version>_amd64.deb` (or configure the repo and run `sudo apt install wtm`).

## Release notes
See `RELEASE.md` for the workflow that builds the release artifacts, tags the repo, and publishes them with `gh`.
