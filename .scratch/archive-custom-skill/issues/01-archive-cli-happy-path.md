# 01: Archive a custom skill from the CLI

**What to build:** `fleet skill archive <name>` moves a custom skill out of its collection dir into the sibling `archived/` folder, unlinks it from every harness, drops its state entries, and runs sync. The fleet-home fallback archives into the fleet home's `archived/` folder by the same rule. The headline and one unlink line per link print in the `remove-dir` style. The archive is one-way.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] `archive <name>` on a custom skill in a tracked collection moves its folder to `<parent of collection>/archived/<name>`
- [ ] The same command on a fallback skill moves it to the fleet home's `archived/` folder
- [ ] Its managed links are gone from every installed harness afterwards
- [ ] Its enablement entries are gone from the state file afterwards
- [ ] Output shows the new path and one unlinked line per removed link
- [ ] `ls` and doctor do not show the archived skill, and do not report it as missing or unlinked
- [ ] When the move fails, nothing is unlinked and the state file is unchanged
- [ ] Sync runs after a successful archive, as on every command
