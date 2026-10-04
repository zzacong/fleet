# 04: `fleet skill remove-dir <path>` end to end

**What to build:** Unregister a tracked collection dir and remove its managed links, leaving the directory and its files untouched. The inverse of `add-dir`, not of `adopt`.

**Blocked by:** 03.

**Status:** ready-for-agent

- [ ] `fleet skill remove-dir <path>` unlists the path from `skillsDirs`, preserving the order of the remaining entries.
- [ ] It removes the managed links that resolve under that dir from every installed harness, then syncs.
- [ ] It never deletes the directory or its contents.
- [ ] An untracked path errors and lists the tracked dirs.
- [ ] A tracked path that is missing from disk still unlists cleanly.
- [ ] `~` and relative paths resolve exactly as in `add-dir`.
