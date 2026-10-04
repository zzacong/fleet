# ADR 0005: Custom skills toggle by managed link, canonical skills by config

Date: 2026-10-04
Status: Accepted

## Context

Fleet exposes custom skills to six harnesses. Before this decision it used two mechanisms. OpenCode and Pi received the custom collection directory as an extra skill source in their own config, while Codex, Claude Code, Cursor, and Bob received one symlink per custom skill in their own skills directory. Disable was uneven in the same way: on the config-path harnesses fleet wrote the harness's own off-entry, while on the link-based harnesses the managed link was the only lever.

The split produced a bug. Pi's force-exclude entry (`-skills/<name>/SKILL.md`) is matched against Pi's auto-scanned skills roots, each with its own base directory. A wired collection lives outside those roots, so disabling a wired custom skill on Pi silently did nothing. The split also meant two code paths, two test surfaces, and two mental models for one idea: this custom skill is installed, enable or disable it per harness.

## Decision

1. Every custom skill is exposed by one managed symlink per harness, in that harness's own skills directory, pointing at the skill directory in its custom home. The config-path writing seam is deleted: fleet no longer writes collection paths into OpenCode's or Pi's config. All six harnesses link customs.

2. The toggle lever is source-aware, not harness-wide. On harnesses that link skills and scan the canonical store natively (OpenCode, Codex, Pi, Cursor, Bob), a custom skill toggles by its managed link. `off` removes the link, `on` recreates it, and `ls` reports `absent` when the link is missing. A canonical-store skill keeps that harness's own config lever where one exists (OpenCode deny rule, Codex `[[skills.config]]`, Pi force-exclude) and stays an explicit no-op on Cursor and Bob.

3. Claude Code is exempt. It is a linker but not a native canonical-store scanner, so every skill reaches it through a link and a link cannot distinguish a disabled custom from an absent one. Claude Code keeps `skillOverrides` for both custom and canonical skills.

4. A symlink whose target is missing reports through the existing broken-link diagnostics. It is not a new state.

5. An unversioned canonical-store skill shown as custom (no lockfile entry) has no managed link and is discovered natively, so it stays config-disabled. It is not link-toggled.

6. The first sync after upgrade removes the legacy fleet-owned entries the old model left behind: OpenCode and Pi collection paths, and fleet-shape exact-name off-entries on OpenCode, Codex, and Pi that name a current custom skill. Each removal is reported by name. The cleanup is scoped to entries resolving into a current custom home or naming a current custom skill, so config fleet did not write is left alone, and later syncs report nothing. No config key or state-file field is added.

## Consequences

- One mechanism and one mental model for custom exposure. The Pi disable bug is fixed because the managed link lands in a root Pi already scans.
- The asymmetry between custom and canonical toggles is deliberate. A canonical skill is installed and discoverable by definition, so hiding it keeps it discoverable-but-hidden through the harness's own lever. A custom skill reaches a native scanner only through its link, so the link is the honest state.
- Claude Code's exemption is forced by its discovery model, not a preference. Moving it to link toggling would make disabled and not-linked indistinguishable.
- Doctor's link-drift analysis covers every native-scanning linker, so a custom's link disagreeing with the state is visible on five harnesses instead of two. Canonical-store skills are never link-drift checked.
- Upgrading needs no manual edit. The first sync cleans the old entries and reports them, and the state schema and config keys are unchanged.

## Alternatives considered

- Keep the config-path wiring for OpenCode and Pi. Rejected: the Pi force-exclude bug, plus a second code path and a second mental model for the same idea.
- Toggle customs by config on OpenCode, Codex, and Pi as before. Rejected: a config off-entry can hide a canonical skill of the same name, and a custom home's location would keep leaking into harness config.
- Move Claude Code's custom toggle to link removal. Rejected: Claude Code reads no store natively, so its link is discovery, and removing it would erase the on/off distinction.
- Add a new state-file field or config key to mark custom versus canonical. Rejected: the skill index already resolves the source, so no schema change is needed.
