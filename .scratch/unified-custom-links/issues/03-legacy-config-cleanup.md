# 03: One-time cleanup of legacy wiring and custom off-entries

**What to build:** On the first sync after upgrading, fleet removes the fleet-owned entries the old wiring model left behind and reports each removal by name: OpenCode and Pi collection paths, and fleet-shape exact-name off-entries on OpenCode, Codex, and Pi that name a current custom skill. Only entries resolving into a current custom home or naming a current custom skill are touched. Later syncs report nothing.

**Blocked by:** 02.

**Status:** ready-for-agent

- [ ] A legacy OpenCode collection-path entry in either config shape is removed when it resolves into a current custom home.
- [ ] A legacy Pi collection-path entry is removed when it resolves into a current custom home.
- [ ] A legacy OpenCode or Codex fleet-shape off-entry, and a legacy Pi exact exclusion, naming a current custom skill are removed.
- [ ] A user-added unrelated source entry, for example a `~/.claude/skills` path, and any entry fleet cannot identify are left untouched.
- [ ] Every removal is reported as an explicit per-entry line; a second sync of the same state reports nothing.
- [ ] No config key or state-file field is added.
