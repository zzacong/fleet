# 02: Custom skills toggle by their managed link on every native-scanning harness

**What to build:** For every harness that links custom skills and scans the canonical store natively (OpenCode, Codex, Pi, Cursor, Bob), a custom skill's enable/disable is the presence of its managed link. Disabling a custom removes the link and `fleet skill ls` shows `absent`; enabling recreates it. Canonical-store skills keep their harness config lever (OpenCode deny rule, Codex `[[skills.config]]`, Pi force-exclude) and still read `on`/`off`. Claude Code remains config-toggled for customs.

**Blocked by:** 01.

**Status:** resolved

- [x] `fleet skill off <custom> --harness <h>` removes the managed link for h in OpenCode, Codex, Pi, Cursor, Bob, and writes no config off-entry for it; `fleet skill on <custom>` recreates the link.
- [x] `fleet skill off <canonical> --harness opencode|codex|pi` still writes that harness's own config off-entry; Cursor and Bob remain explicit no-ops.
- [x] `fleet skill ls`, table and JSON, shows `absent` for a custom with no link on any of those five harnesses, and unchanged `on`/`off` for canonical skills.
- [x] Disabling a custom on Claude Code still writes its `skillOverrides` entry and reads `off`; Claude is not link-toggled.
- [x] Sync never writes a config off-entry for a custom name on a native-scanning link harness, even if an old one is recorded; canonical names still project.
- [x] `fleet skill doctor` reports drift when an enabled custom's link is missing and when a disabled custom's link is present, for the five harnesses, and never reports a canonical skill for link drift.
- [x] A custom skill with a broken (dangling) link surfaces through the existing broken-link diagnostics, not a new state.
- [x] A user-added non-custom path entry and any foreign config entry are left untouched.
