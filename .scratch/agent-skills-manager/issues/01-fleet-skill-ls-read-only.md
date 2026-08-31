# 01: `fleet skill ls` — read-only tracer bullet

**What to build:** The first working slice of fleet: `fleet skill ls` runs against the real home directory, reads everything, writes nothing, and prints the full picture — every skill in the canonical store, marked custom or installed, grouped by source repo, with a per-harness on/off column for each installed harness. `--json` emits the same data for scripts. This ticket stands up the whole skeleton the rest builds on.

**Blocked by:** None (can start immediately).

**Status:** resolved

- [x] Go module skeleton exists: `cmd/fleet` entrypoint, `internal/` packages (harness, scan, state placeholder, cli), Cobra command tree with `fleet skill ls`.
- [x] Canonical store scan lists every skill directory with its `SKILL.md` name and description.
- [x] Lockfile (`~/.agents/.skill-lock.json`) parsed for provenance: source, sourceType, hash, timestamps. A skill with no lock entry is **custom**; with one is **installed**, subgrouped by source repo.
- [x] Harness detection by config-dir probes; only installed harnesses appear (opencode, pi, codex, claude code, Cursor, Bob on this machine).
- [x] Adapter read side implemented for all six harnesses: opencode (JSONC; both dialects — V1 `permission.skill` object map and V2 `permissions` rules array — plus `skills` paths in either dialect), pi (settings `skills` exclusions in both forms: `-skills/<name>/SKILL.md` exact-path and `!<name>` glob), codex (`[[skills.config]]` enabled flags), claude code (`skillOverrides` + `~/.claude/skills` link presence), Cursor (derived: native canonical reader, no disable mechanism), Bob (`~/.bob/skills` link presence).
- [x] `fleet skill ls` prints a table: name, custom/installed, source repo, description, per-harness state. `fleet skill ls --json` emits the same structurally.
- [x] Every path derives from an injected home root (constructor parameter + `FLEET_HOME` env override). No adapter calls `os.UserHomeDir()` directly.
- [x] Unit tests per adapter read side with fixture configs in `t.TempDir()`; nothing outside the project is touched by tests.
- [x] `gofmt`/`golangci-lint` pass (Makefile targets exist from this ticket on).

## Comments

- 2026-08-31 (implementing agent): done on branch `ticket/01-fleet-skill-ls-read-only`. Probe-based detection decides which harness columns appear; on this machine `~/.claude` and `~/.cursor` do not exist, so those columns are absent from live output (the adapter read sides for both are implemented and unit-tested). Cursor and Bob report every canonical skill `on` (locked spec decision: native canonical readers, no per-skill disable); Bob's read side still reports `~/.bob/skills` link presence in its `ReadResult.Linked`, which the redundant-link and custom-skill tickets build on. opencode dialect detection implements the spec's guard: a file whose only marker is the V1-shaped `skills: {paths, urls}` object classifies as V2, never V1. Verified live: the table matches the machine's actual pi exclusions (12 skills off), opencode/codex (no skill rules), and the lockfile provenance.
