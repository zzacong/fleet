# 09: User docs to Starlight at `apps/docs`

**What to build:** User-facing docs become a Starlight site at `apps/docs`, while contributor docs stay at the repo root.

**Blocked by:** 08: Monorepo scaffolding — `apps/cli` + `go.work` + `pnpm-workspace` + `apps/desktop` placeholder

**Status:** resolved

- [x] `apps/docs` is an Astro + Starlight app (`astro.config.mjs` with Starlight, `src/content/docs/` collection, `package.json` for the workspace). `pnpm -F docs dev` and `pnpm -F docs build` work
- [x] User-facing `docs/cli.md`, `docs/harnesses.md`, `docs/state-file.md`, `docs/undo.md` migrate from root `docs/` to `apps/docs/src/content/docs`; root `docs/` keeps `architecture.md`, `testing.md`, `adr/`, `agents/` as contributor docs
- [x] Starlight sidebar reflects the migrated docs; a skills catalog page reads `../../skills/*/SKILL.md` frontmatter at build and lists the collection
- [x] Root `README` links point at the new site paths; old root doc paths either redirect or have a stub pointer to `apps/docs`
- [x] CI builds `apps/docs` via `pnpm -F docs` alongside `apps/cli` via `go.work`; no CLI behavior change in this ticket beyond the doc move
