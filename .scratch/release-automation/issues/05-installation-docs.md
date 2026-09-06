# 05: Installation docs

**What to build:** A user landing on the README or the docs site finds every supported install method in one place, each one tested exactly as written.

**Blocked by:** 04 (First release, bootstrap publish, verification — docs must describe verified reality, not intentions).

**Status:** ready-for-agent

> **Agent note (05-draft, pre-04):** Ticket 04 has not run (no tag, no npm
> publish), so registry-dependent commands cannot be executed verbatim yet.
> This draft describes the intended post-04 reality per ADR 0003 and tickets
> 01–03, tests everything testable locally, and stays **unresolved** pending
> post-04 verbatim verification. Do NOT mark resolved until every command in
> README + `www/src/content/docs/installation.md` has been run verbatim
> (clean machine where feasible) after the bootstrap publish.

- [x] New Installation section in the root README and a matching page on the user docs site, in this order: script install to `~/.fleet/bin` (with `INSTALL_DIR` override and version pin), `npx`, `pnpm dlx`, `bunx`, global installs (`npm i -g`, `pnpm add -g`, `bun add -g`), `go install`. No yarn and no project-local install instructions anywhere.
- [ ] Every documented command is executed verbatim as written (clean machine where feasible), including the override and pin variants.
- [x] Work stays on a local branch until the owner lifts the no-push instruction.

## Verification log (pre-04 draft)

Local PASS (2026-09-06, darwin/arm64 worktree `ticket/05-installation-docs`):

- `sh install.sh --help` → prints `usage: sh install.sh [VERSION]
  (default: latest)` + `env: VERSION, INSTALL_DIR (default ~/.fleet/bin),
  REPO`. Documented flags are exactly the implemented ones (positional
  `[VERSION]`, env `VERSION`/`INSTALL_DIR`/`REPO`); nothing invented.
- Script post-download steps run verbatim against a locally built tarball
  (`fleet_darwin_arm64.tar.gz` + `checksums.txt`): SHA-256 match, extract,
  `cp` to scratch `INSTALL_DIR`, `chmod 755` — both default-layout and
  `INSTALL_DIR` override variants, `fleet --version` OK in each. (The
  `curl …/releases/…` download step itself is PENDING-04: no release exists.)
- `node npm/build.mjs 0.0.0-test` → 4/4 platform targets built and stamped.
- npm launcher through a faithful local `node_modules/@zzacong/{fleet,
  fleet-darwin-arm64}` layout: `node …/launch.mjs --version` → `fleet
  version 0.0.0-test` (exit 0).
- `GOBIN=<scratch> go install ./cmd/fleet` → installs, `--version` reports
  `dev` as documented.
- `npm pack --dry-run` in `npm/fleet` → packs `launch.mjs` only
  (`zzacong-fleet-0.0.0-dev.tgz`, 2 files).
- `pnpm --filter www build` → 8 pages incl. `/installation/`; `oxlint` clean.

PENDING-04 (run verbatim after bootstrap publish, then delete this section
and the HTML comment in `installation.md`):

- `curl …/main/install.sh | sh`, `| sh -s -- 0.1.0`, `|
  INSTALL_DIR=… sh` (fetch step also unverifiable until this branch merges —
  `install.sh` is not on the remote yet).
- `npx -y @zzacong/fleet --version`, `pnpm dlx @zzacong/fleet --version`,
  `bunx @zzacong/fleet --version`.
- `npm i -g @zzacong/fleet`, `pnpm add -g @zzacong/fleet`,
  `bun add -g @zzacong/fleet`.
- `go install github.com/zzacong/fleet/cmd/fleet@latest`,
  `…@v0.1.0` (local-path variant passed; proxy resolution needs the tag).
