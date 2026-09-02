# 03: Fleet-home fallback and snapshot union with skills-repo precedence

**What to build:** Custom skills are discovered from the two explicit custom homes plus the canonical store and shown as one list, with a clear precedence when the same name appears twice.

**Blocked by:** 02: Explicit skills-repo resolution, no walk-up

**Status:** resolved

- [x] Fleet home `~/.config/fleet/skills` exists as the unversioned custom fallback; `ScanStore` on it lists every child dir containing `SKILL.md` (frontmatter `name` falls back to dir name, like canonical). Missing dir is not an error
- [x] `snapshot.Build` unions the sources that exist: `~/.agents/skills` (canonical) + `~/.config/fleet/skills` (fleet-home) + `<skillsRepo>/skills` when a skills repo is set. Absent sources are simply not scanned
- [x] Display uses skills-repo precedence `skillsRepo > fleet-home > canonical`: a name present in more than one source appears once, from the highest-precedence source. The other copy is not shown — its existence is doctor drift, not a silent overwrite
- [x] When no skills repo is set, `fleet skill ls` (and the TUI) shows canonical plus fleet-home customs alone and still works
- [x] Outdated badge stays tri-state and reports unknown for any custom skill (fleet-home or skills-repo) and for non-GitHub sources; update checks still group by canonical lockfile only, fleet-home/skills-repo customs are the zero entry
- [x] Filesystem is the registry: a dir with `SKILL.md` in either custom home appears with `custom: true`, no lockfile entry, no `state.json` registry
- [x] Verified by fake-home snapshot tests: canonical-only, fleet-home-only, skillsRepo-only, two-way and three-way collisions asserting precedence and that `ls --json` reports `custom` and null `outdated` for customs
