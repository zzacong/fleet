# 05: Doctor double-presence for the new custom homes

**What to build:** Doctor reports the same drift it always did, but now across the three scanned sources, and sync still only cleans what it should.

**Blocked by:** 03: Fleet-home fallback and snapshot union with skills-repo precedence, 04: `adopt` retargeted to the resolved custom home

**Status:** ready-for-agent

- [ ] Doctor flags double presence for any skill name appearing in more than one of: canonical `~/.agents/skills`, fleet-home `~/.config/fleet/skills`, skillsRepo `.../skills` (when set). Report groups by name with paths and says "resolve by hand" — it does not pick a winner beyond the ls precedence
- [ ] Sync's redundant-link removal still only removes per-agent symlinks that provably resolve into the canonical store in harnesses that scan it natively (`nativeScanHarnesses`); fleet-home or skills-repo links are never classified redundant and are never removed
- [ ] Broken, foreign, expected, untracked classifications stay unchanged; doctor still groups findings by severity and never writes
- [ ] `fleet skill ls` continues to show one row per name with skills-repo precedence from 03; doctor is the place that surfaces the collision
- [ ] Verified by fake-home doctor tests for two-way and three-way double presence (canonical vs fleet-home, fleet-home vs skillsRepo, all three), and that sync with a fleet-home link does not delete it
