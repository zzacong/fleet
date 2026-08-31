# 02: State file + enable/disable writes + sync

**What to build:** Fleet starts managing. A versioned state file under `~/.config/fleet/` becomes the single source of truth for per-harness enablement, and `fleet skill on/off <name> [--harness]` writes each harness's own native "off" setting through its adapter. Sync makes harness configs match the state file on every command, repairing drift while preserving everything it doesn't own. Skills' files are never moved, renamed, or frontmatter-edited.

**Blocked by:** 01.

**Status:** ready-for-agent

- [ ] State file written/read at `~/.config/fleet/state.json` (or XDG equivalent), versioned schema, forward-compatible (unknown fields survive round-trips).
- [ ] Write side for the four config adapters. opencode: detect the dialect already present in the file (V2 `permissions` array markers vs V1 `permission` object) and write that dialect — V2 auto-migrates V1 keys, V1 drops V2-only keys, so V1 shape is the safe default for files without V2 markers; handle the V1-detection edge case (a file with only `skills: {paths, urls}` is not detected as V1); comments preserved (JSONC). pi: force-exclude entries in the `skills` array of `~/.pi/agent/settings.json`, written in the exact form `pi config` produces: `-skills/<name>/SKILL.md` (strict JSON, unknown keys preserved; the read side also recognizes the `!<name>` glob form). codex: `[[skills.config]]` `name`/`enabled = false` (TOML, comments preserved). claude code: `skillOverrides: {"<name>": "off"}` (strict JSON).
- [ ] Cursor and Bob have no write side in this ticket (Cursor has no per-skill disable mechanism; Bob's disable path needs live verification because Bob empirically reads the canonical store). Toggling for them is a no-op with a clear message.
- [ ] `fleet skill on <name>` / `off <name>` with optional `--harness` (default: all installed harnesses); per-harness state recorded in the state file.
- [ ] Sync runs on every command: reads each installed harness's config, diffs against state, repairs what fleet owns, and **flags what it doesn't recognize instead of touching it**.
- [ ] Read-modify-write preserves unknown keys, comments (JSONC/TOML), and formatting; strict-JSON writers never emit comments.
- [ ] Disabling never moves, renames, or edits skill files; canonical store stays pristine.
- [ ] Golden-file tests per adapter: given state + fixture config, assert exact output config; drift scenarios (manual edit, missing marker) covered.
