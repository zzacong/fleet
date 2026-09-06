# 06: Final docs sweep

**What to build:** No doc anywhere in the repo contradicts the shipped reality — a last pass over everything installation-adjacent after the install pages land.

**Blocked by:** 05 (Installation docs).

**Status:** ready-for-agent

> **Agent note (06-sweep, pre-04):** Ticket 04 has not run (no tag, no npm
> publish) and ticket 05 stays drafted-but-unresolved pending verbatim runs,
> so the "matches what ticket 04 proved" clause cannot be closed yet. This
> sweep verified everything verifiable locally, changed no install/path/version
> content (no live stale references found), and stays **unresolved** pending
> post-04 verbatim verification. Do NOT mark resolved until every command in
> the root README + `www/src/content/docs/installation.md` has been run
> verbatim after the bootstrap publish (owned by 04/05).

- [ ] Every remaining install/path/version reference across the root README, the docs site, and contributor docs matches what ticket 04 proved; no stale `apps/` paths; no hand-maintained version numbers or changelog entries anywhere (CHANGELOG and release notes stay tool-owned).
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

PENDING-04 (run verbatim after the bootstrap publish, then delete this section
and the HTML comment in `installation.md`; owned by 04/05, not this ticket):

- `curl …/main/install.sh | sh`, `| sh -s -- 0.1.0`, `| INSTALL_DIR=… sh`.
- `npx -y @zzacong/fleet --version`, `pnpm dlx @zzacong/fleet --version`,
  `bunx @zzacong/fleet --version`.
- `npm i -g @zzacong/fleet`, `pnpm add -g @zzacong/fleet`,
  `bun add -g @zzacong/fleet`.
- `go install github.com/zzacong/fleet/cmd/fleet@latest`, `…@v0.1.0`.
- Then mark the first checkbox resolved and delete this note.
