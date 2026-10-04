# 05: Retire the git model

**What to build:** With both path verbs live, remove everything the old on-ramp needed: the `pull` and `drop` verbs, the git clone and fast-forward flow, the fleet-home checkout convention, and the env override. The tracked set is now the `skillsDirs` list plus the unversioned fallback, and nothing else.

**Blocked by:** 03, 04.

**Status:** ready-for-agent

- [ ] `fleet skill pull` and `fleet skill drop` no longer exist.
- [ ] No code path shells out to git; a missing git binary affects no fleet command.
- [ ] `FLEET_REPO` is never read and the fleet-home checkout parent is never scanned.
- [ ] `skillsRepos` is never read and is preserved verbatim as an unknown field on save.
- [ ] The git runner seam and the drop flow seam are deleted, and the test suite uses `skillsDirs`.
- [ ] The build and the full test suite are green.
