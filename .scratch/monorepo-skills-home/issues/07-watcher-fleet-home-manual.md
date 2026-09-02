# 07: Watcher fleet-home coverage — manual verification

**What to build:** `scripts/watcher/watch.py` and `.agents/skills/watcher/SKILL.md` watch the new fleet-home customs so you can manually verify snapshots before any monorepo move.

**Blocked by:** 03: Fleet-home fallback and snapshot union with skills-repo precedence

**Status:** ready-for-agent

- [ ] `WATCH_TARGETS` in `watch.py` is verified to cover fleet-home: `fleet-config` (`~/.config/fleet` dir walk) already includes `state.json`, `config.json`, `skills/` fleet-home and `tree-cache.json` — confirm that a populated `~/.config/fleet/skills/<name>/SKILL.md` appears under the `fleet-config` target, not missed
- [ ] `repo-skills` target (`<repo>/skills` at the current repo root) still resolves correctly at this point (no monorepo depth change yet); table in `.agents/skills/watcher/SKILL.md` is updated to note fleet-home customs are covered by the `fleet-config` dir walk and that `repo-skills` is the designated skills repo's `skills/` when one is set
- [ ] Manual verification: run `python3 scripts/watcher/watch.py --initial --label=baseline`, then populate `~/.config/fleet/skills/<test>/SKILL.md` in a fake home and a temp skills repo, run `python3 scripts/watcher/watch.py --label=go-1` and confirm diff shows `+` under the expected target and that `fleet skill ls` and watcher agree
- [ ] `SKILL.md` watch table count and labels are kept in sync; no harness targets change in this ticket
