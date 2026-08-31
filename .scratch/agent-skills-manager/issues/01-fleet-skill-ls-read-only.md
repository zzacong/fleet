# 01: `fleet skill ls` — read-only tracer bullet

**What to build:** The first working slice of fleet: `fleet skill ls` runs against the real home directory, reads everything, writes nothing, and prints the full picture — every skill in the canonical store, marked custom or installed, grouped by source repo, with a per-harness on/off column for each installed harness. `--json` emits the same data for scripts. This ticket stands up the whole skeleton the rest builds on.

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] Go module skeleton exists: `cmd/fleet` entrypoint, `internal/` packages (harness, scan, state placeholder, cli), Cobra command tree with `fleet skill ls`.
- [ ] Canonical store scan lists every skill directory with its `SKILL.md` name and description.
- [ ] Lockfile (`~/.agents/.skill-lock.json`) parsed for provenance: source, sourceType, hash, timestamps. A skill with no lock entry is **custom**; with one is **installed**, subgrouped by source repo.
- [ ] Harness detection by config-dir probes; only installed harnesses appear (opencode, pi, codex, claude code, Cursor, Bob on this machine).
- [ ] Adapter read side implemented for all six harnesses: opencode (JSONC; both dialects — V1 `permission.skill` object map and V2 `permissions` rules array — plus `skills` paths in either dialect), pi (settings `skills` exclusions in both forms: `-skills/<name>/SKILL.md` exact-path and `!<name>` glob), codex (`[[skills.config]]` enabled flags), claude code (`skillOverrides` + `~/.claude/skills` link presence), Cursor (derived: native canonical reader, no disable mechanism), Bob (`~/.bob/skills` link presence).
- [ ] `fleet skill ls` prints a table: name, custom/installed, source repo, description, per-harness state. `fleet skill ls --json` emits the same structurally.
- [ ] Every path derives from an injected home root (constructor parameter + `FLEET_HOME` env override). No adapter calls `os.UserHomeDir()` directly.
- [ ] Unit tests per adapter read side with fixture configs in `t.TempDir()`; nothing outside the project is touched by tests.
- [ ] `gofmt`/`golangci-lint` pass (Makefile targets exist from this ticket on).
