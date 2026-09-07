# 05: Installation docs

**What to build:** A user landing on the README or the docs site finds every supported install method in one place, each one tested exactly as written.

**Blocked by:** 04 (First release, bootstrap publish, verification — docs must describe verified reality, not intentions).

**Status:** resolved

- [x] New Installation section in the root README and a matching page on the user docs site, in this order: script install to `~/.local/bin` (with `INSTALL_DIR` override and version pin), `npx`, `pnpm dlx`, `bunx`, global installs (`npm i -g`, `pnpm add -g`, `bun add -g`), `go install`. No yarn and no project-local install instructions anywhere.
- [x] Every documented command is executed verbatim as written (clean machine where feasible), including the override and pin variants.
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

## Verification log (post-04, 2026-09-07, darwin/arm64, node 22 / npm 10.9.9 / pnpm 12.3.4 / bun 1.4.2 / go 1.27.1)

Every command on the README Installation section and
`www/src/content/docs/installation.md` executed verbatim against the live
registry (`latest` = 0.1.1). Same machine, not literally clean — global
installs were removed after each verification so PATH lookups never crossed.

Run-once doors (all → `fleet version 0.1.1`):

- `npx -y @zzacong/fleet --version` ✅
- `pnpm dlx @zzacong/fleet --version` ✅ — first attempt failed with EACCES
  from a stale dlx cache entry pinned to 0.1.0 whose binary had lost its
  exec bit; after clearing the cache entry it passed. The 0.1.1 launcher
  already restores the exec bit (release-please entry
  `fix(npm): launcher restores exec bit on platform binary`), so fresh
  installs are unaffected; published tarball confirmed `755`.
- `bunx @zzacong/fleet --version` ✅

Global installs (each installed → `fleet version 0.1.1` → removed):

- `npm i -g @zzacong/fleet` / `npm rm -g @zzacong/fleet` ✅
- `pnpm add -g @zzacong/fleet` / `pnpm rm -g @zzacong/fleet` ✅
- `bun add -g @zzacong/fleet` / `bun remove -g @zzacong/fleet` ✅
  (bun prints its own `export PATH=.../.bun/bin` hint when the dir is
  missing from PATH; with it on PATH `fleet` resolves normally)

Go install (proxy resolves both tags):

- `go install github.com/zzacong/fleet/cmd/fleet@latest` → `fleet version dev` ✅
- `go install github.com/zzacong/fleet/cmd/fleet@v0.1.0` → `fleet version dev` ✅

Install script (default `~/.local/bin`, per commit b5bfe01; left installed
and on PATH afterwards):

- `sh install.sh` → 0.1.1 into `~/.local/bin` ✅
- `sh install.sh 0.1.0` → pinned 0.1.0 ✅; re-ran `sh install.sh` to restore
  latest ✅
- `sh install.sh --help` → usage + env lines with the new
  `~/.local/bin` default ✅
- Piped verbatim against the remote:
  `curl -fsSL https://raw.githubusercontent.com/zzacong/fleet/main/install.sh | INSTALL_DIR=<scratch> sh`
  → 0.1.1 with PATH hint ✅ (proves raw-URL fetch + pipe + override)
- Piped verbatim:
  `curl -fsSL https://raw.githubusercontent.com/zzacong/fleet/main/install.sh | sh -s -- 0.1.0`
  → 0.1.0 ✅ (installed into the *remote* script's default `~/.fleet/bin`,
  then cleaned up — see caveat below)

**One open thread (owner action):** remote `main` predates b5bfe01
(`feat(install): default INSTALL_DIR to ~/.local/bin`), so the bare piped
command `curl …/main/install.sh | sh` currently installs into
`~/.fleet/bin`, not the documented `~/.local/bin`. The default-dir variant
was verified against the local script bytes (identical to post-push `main`)
and the piped mechanics against the live URL; once b5bfe01 is pushed the
documented command behaves exactly as written. All other commands are
unaffected.
