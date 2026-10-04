# 04: `fleet skill prune`

**What to build:** `fleet skill prune` removes the stale config rules and stale state entries for skills installed nowhere. With no flags it lists what it would remove and changes nothing. `--yes` applies. `--harness` is repeatable and limits the work to named harnesses. `--config-only` and `--state-only` select one axis. After a successful prune it runs sync, as every command does.

Removals go through the existing enable write path, so only fleet's own exact disable shapes are removed: the OpenCode V1 exact deny, the OpenCode V2 three-key deny rule, the Pi exact exclusion entry, the Codex simple disabled block, and the Claude `"off"` override. Patterns, blankets, extra-key shapes, and foreign values survive. Removing a state entry uses the existing state enable path, which deletes a skill entry once nothing remains. When the scan is incomplete, prune removes nothing and names the home that blocked it. Output follows the `drop:` and `adopt:` palette. The command is idempotent.

This ticket is gated on 02 so that a `--config-only` prune is not immediately re-written by sync.

**Blocked by:** 01 (Skill index completeness and installed names), 02 (Dormant disables in sync and update).

**Status:** ready-for-agent

- [ ] Without `--yes`, prune lists the stale config rules and state entries and changes nothing.
- [ ] `--yes` removes them.
- [ ] `--harness` limits to named harnesses; `--config-only` and `--state-only` select an axis.
- [ ] Only fleet's own exact disable shapes are removed; a pattern rule, a blanket rule, and a foreign value survive.
- [ ] Removing a state entry deletes the skill entry once nothing remains.
- [ ] An incomplete scan removes nothing and names the blocking home.
- [ ] Sync runs after a successful prune.
- [ ] A second run reports nothing to do.
- [ ] Output follows the `drop:` and `adopt:` palette.
- [ ] Package tests cover the operation; CLI tests cover flags, confirmation, and output over a fake home.
