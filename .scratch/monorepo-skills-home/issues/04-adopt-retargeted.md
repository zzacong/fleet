# 04: `adopt` retargeted to the resolved custom home

**What to build:** `fleet skill adopt` moves a skill into the home that will actually be scanned and wires/links that home for every installed harness.

**Blocked by:** 02: Explicit skills-repo resolution, no walk-up

**Status:** ready-for-agent

- [ ] Adopt resolves its target as: designated skills repo's `skills/` when a skills repo is set, otherwise `~/.config/fleet/skills`. The "no skills repo found" error is replaced by the new hint from 02; a missing fleet-home dir is created on demand
- [ ] Adopt scans canonical store plus both custom homes for the name (dir name first, then frontmatter name). A name existing in both canonical and either custom home, or in both custom homes, is a double-presence error to resolve by hand
- [ ] On a store hit, the directory is moved atomically from `~/.agents/skills/<name>` into the resolved target `.../skills/<name>`; on a repo/fleet-home hit, nothing moves but wiring/links are re-ensured so a partially failed run heals
- [ ] Wiring targets the resolved home: `opencode` and `pi` get the home path via `SourceWiring.WireSkillSource` in the dialect the config already speaks; link-based harnesses (`codex`, `claude code`, `cursor`, `bob`) get `LinkSkill` symlinks into the resolved home. No link ever points into the canonical store
- [ ] No state file write; enable/disable records keep applying by skill name wherever it lives; `adopt` remains a no-op for state
- [ ] Verified by fake-home adopt tests for both targets (repo set vs unset), collision errors, already-adopted re-ensure, and that the wired/linked paths point at the resolved home, not the old fleet repo path
