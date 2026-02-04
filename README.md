# wtm (worktree manager)

`wtm` syncs repo-local config files (e.g. `.env*`) into a selected `git worktree`.

## What it does
- Lists active worktrees (1-indexed).
- Lets you select a worktree (interactive or `--worktree N` / `--dest PATH`).
- Prints a copy plan (`src -> dst`) and asks for confirmation.
- Copies files, preserving relative paths.

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
go run ./cmd/wtm sync
```

Non-interactive selection/confirmation:

```bash
go run ./cmd/wtm sync --worktree 2 --yes
```

To inspect the embedded version before releasing, run `go run ./cmd/wtm version` (or the built binary) and compare it to the `VERSION` file.
