# Release process

## Versioning
- The `VERSION` file holds the current SemVer release (default `0.1.0`).
- Run `make bump-version` to choose a major/minor/patch bump or enter a custom tag interactively; the target validates SemVer before writing the file.
- Every build embeds `VERSION` via `-ldflags` so the CLI and release artifacts report the correct number.

## Building
- `make build-local` produces `bin/wtm` for the host platform.
- `make build-release` cross-compiles `wtm` for Linux, macOS, and Windows (amd64/arm64 where applicable) and packages each binary as `release/wtm-<os>-<arch>-<version>.(tar.gz|zip)`.

## Releasing
1. Commit your changes and ensure `git status` is clean so the release target will proceed.
2. `make build-release` (already a prerequisite for `make release`, but running it manually lets you inspect artifacts). 
3. `make release` will check that you are in a git worktree, tag the current HEAD as `v<version>`, push the tag, and call `gh release create` with the packaged artifacts.
   - The target requires the GitHub CLI (`gh`) and `gh auth status` to succeed.
   - If `gh` is missing, the target stops after building and tells you to publish the tarballs/zips from `release/` manually.
4. After the release completes, `gh release view v<version>` shows the published notes and uploaded binaries.

## Verification
- Run `wtm version` (bundled binary or `go run ./cmd/wtm version`) to confirm the embedded version matches `VERSION`.
