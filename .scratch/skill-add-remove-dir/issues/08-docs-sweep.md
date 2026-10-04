# 08: Docs sweep

**What to build:** Update every user and contributor doc to describe the path-tracked model. The ADR and the glossary are already written; this ticket makes the README, the docs site, and the contributor notes agree with them.

**Blocked by:** 05.

**Status:** ready-for-agent

- [ ] The README quickstart documents `add-dir` and `remove-dir` and the `skillsDirs` list.
- [ ] The command reference has `fleet skill add-dir` and `remove-dir` sections and no `pull` or `drop` sections.
- [ ] The state and config explanation describes `skillsDirs` and the preserved `skillsRepos` unknown.
- [ ] The undo guide describes removing a tracked dir without deleting it from disk.
- [ ] The architecture and testing docs describe the path-tracked model and the deleted git seam.
- [ ] No doc presents `pull`, `drop`, `FLEET_REPO`, or the fleet-home checkout convention as current.
