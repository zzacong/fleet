# 01: Link every custom skill into every harness (retire config-path wiring)

**What to build:** Custom skills reach every harness through one managed symlink per skill in that harness's own skills directory. OpenCode and Pi stop receiving a custom-home collection path in their config; the config-path wiring seam and its reports are removed. `adopt`, `pull`, `drop`, and `sync` create and remove links uniformly and say "linked"/"unlinked". Disabling a custom skill still uses the harness's config lever for now (the link-based custom toggle arrives in ticket 02), so `ls` continues to render `off` for a disabled custom.

**Blocked by:** None (can start immediately).

**Status:** resolved

- [x] After adopt, pull, or sync, a custom skill has a managed symlink in each installed harness's skills directory (OpenCode, Pi, Codex, Claude Code, Cursor, Bob), targeting the skill directory in its custom home.
- [x] OpenCode's and Pi's configs receive no custom-home collection path entry; entries fleet did not write, such as a user-added `~/.claude/skills` source, are left unchanged.
- [x] `drop` removes every managed link resolving under the dropped collection across all installed harnesses.
- [x] CLI output and help for adopt/pull/drop/sync use "linked"/"unlinked"; no "wired"/"unwired" vocabulary or report sections remain.
- [x] A disabled custom skill still reads `off` on OpenCode, Codex, and Pi, and its managed link remains present.
- [x] A symlink into the canonical store in OpenCode's or Pi's skills directory is still removed as redundant; a real directory, a file, or a foreign symlink is still left untouched.
- [x] The OpenCode skill-sources read field and its parser, now unused, are removed.
- [x] The suite builds and passes with the wiring tests deleted and the remaining tests updated to the unified-link expectations.
