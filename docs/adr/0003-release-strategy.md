# ADR 0003: Release automation via release-please, GoReleaser, and OIDC npm publish

Date: 2026-09-06
Status: Accepted

## Context

Fleet has no releases, no git remote, and no tags. Two plans were prototyped (changesets-first npm rollout; one-tag GoReleaser rollout with a downloader npm wrapper) and two sandbox repos proved the mechanics (mr-hulla: bundled-binary npm packages; mr-papaya: release-please Version PR → tag → GoReleaser). Constraints fixed during review: no human ever pushes a tag, npm publishes with no stored token, macOS+Linux only, Homebrew deferred.

Two structural facts forced the shape. First, `apps/cli/go.mod` declares `module github.com/zzacong/fleet` while living in `apps/cli/`, so the Go proxy cannot resolve any install path and the README's `go install` line cannot work under any tag scheme. Second, the unscoped npm name `fleet` is taken (0.1.6), so the npm door is `@zzacong/fleet`.

## Decision

1. **release-please (`release-type: go`) is the only version driver.** Conventional commits on `main` → Release PR (version bump + root `CHANGELOG.md`) → merge → tag `vX.Y.Z` + GitHub Release appear automatically. Changesets was rejected: it versions publishable npm packages and fleet has one binary, so it would add a second version source plus per-change file discipline with nothing to curate. Only `feat`/`fix`/`deps` commits open a Release PR; content work lands as `docs(skills):` / `docs(www):` / `chore:` and is invisible to versioning.
2. **Go module moves to the repo root.** `go.mod` (path unchanged), `cmd/fleet`, `internal/*` at top level; `go.work` deleted. One plain `vX.Y.Z` serves the Go proxy, GoReleaser, and the future brew formula — no dual or directory-prefixed tags, ever. `apps/desktop/` (one README) is deleted; `apps/docs` moves to `www/` and `apps/` disappears.
3. **npm is five bundled-binary packages under `npm/`** (`@zzacong/fleet` wrapper + four `fleet-<os>-<arch>` platform packages, mr-hulla shape). The postinstall-downloader shape was rejected: it breaks under `--ignore-scripts` and needs network at install. In git every `package.json` sits at `0.0.0-dev` forever; the tag workflow stamps all five to the tag's `X.Y.Z` (rewriting `workspace:*` to exact versions) after install/build and immediately before publish — platforms first, wrapper last. Stamped versions are never committed back.
4. **npm publishes via OIDC trusted publishing, no token.** `id-token: write`, npm ≥ 11.5.1, provenance automatic. One-time setup on npmjs.com per package pins user `zzacong` → repo `fleet` → workflow `release.yml`; the filename must never be casually renamed afterwards.
5. **GoReleaser owns tag builds** (four tarballs + `checksums.txt`, frozen `name_template`); `install.sh` at root (OS/arch detect, SHA-256 verified against `checksums.txt`, no shell-rc edits); `fleet --version` via ldflags plus a `debug.ReadBuildInfo` fallback for `go install` builds. A ~30-line `build.mjs` cross-compiles the npm binaries independently of GoReleaser's `dist/` layout so npm packaging never parses GoReleaser internals.
6. **Homebrew is a future personal tap** (`brews:` block + `homebrew-fleet` repo), not homebrew-core. v0.1.0's only obligation to brew is the frozen archive template and published checksums.
7. **The skills collection stays in this repo.** Split signals, if they ever appear: skills commits dominating release history despite the commit convention, the collection needing version semantics of its own, or skills.sh requiring a repo-root shape. Exit path is `git subtree split`, per ADR 0001.

## Consequences

- The only stored CI secret is `RELEASE_PLEASE_TOKEN` (PAT: tags created with `GITHUB_TOKEN` do not trigger the tag workflow). A future cross-repo brew tap will need a second PAT.
- Conventional commits are load-bearing (release-please infers semver from them); a mistyped `feat(skills):`/`feat(www):` costs at most a cheap empty-bump release, not breakage. No commit lint until a repeat offense.
- Vocabulary (ADR-local, deliberately not in `CONTEXT.md`, which is product language): canonical term **Release PR** ("Version PR" noted as alias); canonical term **install door** for the npx / go-install / script / tarball surfaces.
- `release-please.yml` (push to `main`) and `release.yml` (on `v*.*.*`, GoReleaser + OIDC npm jobs) replace the scaffolded workflows entirely.

## Alternatives considered

- Changesets version-or-publish flow (mr-hulla shape end to end). Rejected: second version source, per-change files, no Go-side tagging.
- Nested module + fixed path (`github.com/zzacong/fleet/apps/cli`) with dual tags. Rejected: permanent per-release tax to avoid a one-commit move.
- Nested module + abandoning `go install`. Rejected: documenting a dead primary channel for a Go CLI.
- Plain `go build` matrix instead of GoReleaser. Rejected: re-implements checksums, archive naming, and the future brew step the existing config already covers.
- Splitting `skills/` or `www/` into separate repos now. Rejected: no measured pain; premature extraction pre-first-release.
