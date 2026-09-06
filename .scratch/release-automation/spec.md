# Spec: first release automation

Source of truth: `docs/adr/0003-release-strategy.md` (accepted). Full narrative proposal: https://av6rxm1oop4f.postplan.dev (v3).

## Goal

Ship fleet `v0.1.0` through four install doors (shell script, npx-compatible npm packages, `go install`, manual tarballs) with a pipeline where no human ever pushes a tag or publishes by hand after a one-time bootstrap.

## Pipeline

Conventional commits on `main` → release-please Release PR (version + `CHANGELOG.md`) → merge → tag `vX.Y.Z` + GitHub Release appear automatically → tag workflow runs GoReleaser (tarballs + `checksums.txt`) and publishes five OIDC npm packages (versions stamped from the tag).

## Settled decisions

- release-please (`release-type: go`), not changesets; content commits typed `docs(skills):` / `docs(www):` never open a Release PR.
- Go module at repo root (one plain `vX.Y.Z` for proxy + GoReleaser + future brew); `apps/` deleted (`apps/docs` → `www/`, desktop placeholder dropped).
- Bundled-binary npm shape (`@zzacong/fleet` + four platform packages); `0.0.0-dev` placeholders in git, tag-stamped at publish, never committed back.
- OIDC npm publish, no stored token; only secret is `RELEASE_PLEASE_TOKEN`. First publish is a one-time local bootstrap, then trusted publishers are attached.
- `install.sh` verifies SHA-256; `build.mjs` stays independent of GoReleaser `dist/`; Homebrew deferred to a personal tap; skills collection stays in-repo.
- Standing instruction: nothing pushes to the remote until the owner says so (gates the merge/push portions of tickets 03–04).
