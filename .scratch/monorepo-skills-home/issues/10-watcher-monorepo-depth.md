# 10: Watcher path depth fix after monorepo move

**What to build:** Watcher still finds the monorepo `skills/` collection after the CLI moves to `apps/cli`.

**Blocked by:** 08: Monorepo scaffolding — `apps/cli` + `go.work` + `pnpm-workspace` + `apps/desktop` placeholder

**Status:** resolved

- [x] `REPO` derivation in `watch.py` (currently `os.path.abspath(os.path.join(SCRIPT_DIR, "..", ".."))`) is adjusted for the new layout where Go code lives at `apps/cli` but watcher stays at `scripts/watcher/watch.py` — verify `repo-skills` (`<repo>/skills` at the monorepo root) still resolves correctly after the move (depth changes if watcher or repo root moves)
- [x] `.agents/skills/watcher/SKILL.md` watch table is re-verified: `repo-skills` documents the monorepo `skills/` publishable collection; `fleet-config` still covers fleet-home `skills/`; harness targets remain derived from `internal/paths`
- [x] Verified by re-running `python3 scripts/watcher/watch.py --initial --label=baseline` and a labeled diff after the monorepo scaffolding, with a temp skills repo and fleet-home fixture, asserting the table and `+`/`~` markers still land under the correct labels
