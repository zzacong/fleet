# 06: Final docs sweep

**What to build:** No doc anywhere in the repo contradicts the shipped reality — a last pass over everything installation-adjacent after the install pages land.

**Blocked by:** 05 (Installation docs).

**Status:** resolved

- [x] Every remaining install/path/version reference across the root README, the docs site, and contributor docs matches what ticket 04 proved; no stale `apps/` paths; no hand-maintained version numbers or changelog entries anywhere (CHANGELOG and release notes stay tool-owned).
- [x] Known wart stands as-is: root `docs/` (contributor markdown) versus `www/` (Starlight site) naming is confusing but out of scope — no rename in this ticket.
- [x] Work stays on a local branch until the owner lifts the no-push instruction.

## Verification log (pre-04 sweep, 2026-09-06, worktree `ticket/06-final-docs-sweep`, base `3509a70`)
Fixed in this sweep: nothing — no live stale references existed, so no
install/path/version content was changed. Only this ticket file is committed.

- `apps/` grep (excl. `.git`, `node_modules`, `.worktrees`): live tree clean.
  Remaining hits exist ONLY in ADR historical records (`docs/adr/0001`, `docs/adr/0003`,
  which document the old layout) and `.scratch/` history (old tickets/specs) — no
  live references in code, workflows, README, `www/`, `docs/architecture.md`,
  `docs/testing.md`, `skills/`, `scripts/watcher/`, `Makefile`, `.goreleaser.yaml`,
  `package.json`, or `pnpm-workspace.yaml`.
- Stale install paths: none live. `apps/cli/cmd/fleet`, `apps/docs`,
  `pnpm --filter docs` / `-F docs`, `go.work` (file absent), unscoped `npm i -g fleet`,
  `docs:*` scripts — all appear only in ADR/`.scratch` history. The
  `.goreleaser.yaml` `^docs:` line is a changelog-exclude regex, not a script.
  Root `package.json` `"name": "fleet"` is `private: true` (workspace root, never
  published); all five published packages are `@zzacong/*` at `0.0.0-dev`.
- Hand-maintained versions: none. `0.1.0` / `v0.1.0` appear only as illustrative pin
  examples in `install.sh` comments and `www/src/content/docs/installation.md`
  (the spec's first-release version, per ADR 0003) plus ADR/`.scratch` history.
  `0.0.0` appears only as `0.0.0-dev` npm placeholders (correct in git) and
  third-party pseudo-versions in `go.mod`/`go.sum`. `internal/buildinfo` fallback
  is `dev` (correct for `go install`). No root `CHANGELOG.md` exists (correct —
  tool-owned via release-please); no hand-written release notes anywhere.
- README ↔ `installation.md` ↔ `install.sh` consistency (static, pre-04): install
  URL, `~/.fleet/bin` default, `INSTALL_DIR`/`VERSION`/`REPO` overrides, `latest`
  default with optional leading `v`, SHA-256 vs `checksums.txt`, no shell-rc edits
  + `export PATH` hint, `@zzacong/fleet` wrapper + four optional platform packages,
  `go install github.com/zzacong/fleet/cmd/fleet@latest` (matches `go.mod` module +
  `cmd/fleet`), method order script → run-once → global → go, no yarn and no
  project-local instructions. README doc-table targets all exist; `www` sidebar
  includes the Installation slug.
- `docs/` vs `www/` wart: untouched — both dirs stand as-is, no rename.
- Light verification (docs-only sweep, no Go files touched): `pnpm --filter www build`
  → 8 pages incl. `/installation/`, exit 0; `oxlint` → 0 warnings 0 errors.

## Verification log (post-04 sweep, 2026-09-07, local `main` rebased onto `origin/main` @ 5ed58a3)

No install/path/version content changed in this pass — the pre-04 findings
all still hold, now checked against the shipped 0.1.0/0.1.1 reality:

- `apps/` grep (excl. `.git`, `node_modules`, `.worktrees`, ADR/`.scratch`
  history): live tree still clean. The only new hit is inside
  `CHANGELOG.md`'s 0.1.0 entry (a historical commit subject) — tool-owned,
  untouchable by design.
- Stale install paths: none live. `~/.fleet/bin` now appears nowhere in
  README, `www/`, or `install.sh` (commit b5bfe01 moved all three to
  `~/.local/bin`); grep confirms zero remaining references outside ADR/
  `.scratch` history.
- Hand-maintained versions: none added. `CHANGELOG.md` carries only the
  release-please-written 0.1.0 and 0.1.1 sections; no hand-written release
  notes. `0.1.0` still appears only as illustrative pin examples in
  `install.sh` comments and `installation.md` (now a real released version).
- README ↔ `installation.md` ↔ `install.sh` consistency re-checked: install
  URL, `~/.local/bin` default, override/pin variants, `REPO` env, PATH-hint
  behavior, package names, `go install` module path, method order. README
  doc-table targets all exist; `www` sidebar includes the Installation slug.
- Light verification: `pnpm --filter www build` + `oxlint` re-run after
  removing the PENDING-04 comment from `installation.md` (see below).

Every command the install pages document was executed verbatim post-04 on
2026-09-07 — the command-by-command log lives in ticket 05.
