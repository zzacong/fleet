# Spec: Unified managed links for custom skills

Status: ready-for-agent

## Problem Statement

Fleet exposes custom skills to the six harnesses in two different ways. OpenCode and Pi get a custom-skill **collection directory** written into their own config (`skills.paths` / `skills` array), while Codex, Claude Code, Cursor, and Bob get a **symlink per skill** in their own skills directory. Disabling a custom skill is equally uneven: on the config-path harnesses fleet writes the harness's own off-entry, while on the link-based harnesses the managed link is the only lever.

The split produces a real bug. Pi's force-exclude entry (`-skills/<name>/SKILL.md`) is a path matched against Pi's auto-scanned skills roots; a wired collection lives outside those roots, so disabling a wired custom skill on Pi silently does nothing. More broadly, two mechanisms mean two code paths, two test surfaces, and two mental models for the same idea ("this custom skill is installed; enable or disable it per harness").

I want one mechanism for every harness: each custom skill is exposed by a managed symlink in the harness's own skills directory, and that link is its enable/disable state.

## Solution

Custom-skill exposure becomes uniform across all six harnesses, and disable becomes source-aware.

- Every custom skill is exposed as **one managed symlink per harness** in that harness's skills directory, pointing at the skill directory in its custom home. The config-path wiring seam is deleted entirely: fleet no longer writes collection paths into OpenCode's or Pi's config.
- For harnesses that scan the canonical store natively (OpenCode, Codex, Pi, Cursor, Bob), a **custom** skill's enable/disable *is* its managed link: fleet removes the link to disable it and recreates it to enable it. A **canonical-store** skill keeps using that harness's own config lever where one exists (OpenCode deny rule, Codex `[[skills.config]]`, Pi force-exclude), and remains a no-op on Cursor and Bob.
- Claude Code stays on its config toggle (`skillOverrides`). It is the only harness that is not a native canonical-store reader — every skill reaches it through a link — so the "customs toggle by link" rule does not apply to it.
- On the next sync, fleet removes legacy fleet-owned entries left by the old wiring model and reports each one explicitly.
- CONTEXT.md, the user docs, and a new ADR record the single model.

## User Stories

1. As a fleet user, I want every harness to discover each custom skill through one managed symlink in its own skills directory, so that custom exposure works the same way everywhere.
2. As a fleet user, I want fleet to stop writing custom-skill collection paths into OpenCode's and Pi's config, so that a custom home's location never leaks into a harness config again.
3. As a fleet user, I want `fleet skill off <custom> --harness <h>` on OpenCode, Codex, Pi, Cursor, and Bob to remove that harness's managed link, so that hiding a custom skill uses one predictable lever.
4. As a fleet user, I want `fleet skill on <custom> --harness <h>` to recreate the managed link, so that re-enabling a custom skill is the exact inverse.
5. As a fleet user, I want disabling a custom skill on Pi to actually work (today it silently does nothing when the custom was exposed by a wired path), so that Pi honors the state I recorded.
6. As a fleet user, I want disabling a canonical-store skill to keep using each harness's own config lever (OpenCode deny, Codex `[[skills.config]]`, Pi force-exclude), so that installed skills stay discoverable-but-hidden rather than disappearing.
7. As a fleet user, I want disabling a canonical-store skill on Cursor or Bob to stay an explicit no-op, so that fleet never pretends to have a lever a harness doesn't have.
8. As a fleet user, I want `fleet skill ls` to show `absent` (`-`) for a custom skill whose managed link is missing on a native-scanning harness, so that a disabled custom reads exactly like a disabled custom on Cursor and Bob.
9. As a fleet user, I want `fleet skill ls` to show `on`/`off` for canonical-store skills, read from each harness's config, so that stored-skill state is unchanged from today.
10. As a fleet user, I want Claude Code to keep reporting `on`/`off` from `skillOverrides` for both custom and canonical skills, so that its link-based discovery and config disable are untouched.
11. As a fleet user, I want a custom skill exposed by a symlink whose target is missing to report through the existing broken-link path rather than a new state, so that diagnostics stay consistent.
12. As a fleet user, I want fleet to write no config off-entry for a custom skill on OpenCode, Codex, or Pi, so that a line meant for a custom can never accidentally hide a canonical skill of the same name.
13. As a fleet user upgrading from an earlier release, I want the next sync to remove the collection paths fleet previously wired into OpenCode's and Pi's config, so that my config converges on the new model without manual editing.
14. As a fleet user upgrading, I want the next sync to remove fleet-written off-entries that targeted custom skills on OpenCode, Codex, and Pi, so that those skills are governed by their links instead.
15. As a fleet user upgrading, I want each removed legacy entry reported as a named line, so that I can see exactly what fleet changed in my config and why.
16. As a fleet user, I want the migration to leave entries fleet did not write alone — including a path I added myself such as `~/.claude/skills` — so that fleet never deletes config it does not own.
17. As a fleet user, I want the migration to be idempotent, so that the second and later syncs report nothing.
18. As a fleet user, I want `fleet skill sync` to create missing managed links for enabled customs and remove links for disabled customs on all six harnesses, so that one verb reconciles the whole picture.
19. As a fleet user, I want `fleet skill doctor` to report a missing link for an enabled custom skill (and a present link for a disabled one) on OpenCode, Codex, Pi, Cursor, and Bob, so that drift is visible on every native-scanning harness.
20. As a fleet user, I want doctor to keep exempting canonical-store skills from link-drift checks, so that a store skill with no link is never reported.
21. As a fleet user, I want `adopt`, `pull`, and `drop` to link and unlink custom skills uniformly across all six harnesses, so that every lifecycle verb uses the same primitive.
22. As a fleet user, I want `drop` to remove every managed link into the dropped collection across all six harnesses, so that a dropped home leaves no dangling links.
23. As a fleet user, I want `adopt`/`pull`/`drop` output to say "linked"/"unlinked" per harness per skill instead of "wired"/"unwired", so that the vocabulary matches the mechanism.
24. As a fleet user, I want a harness's managed custom links to be recognized as fleet's own and never flagged as foreign entries, so that doctor and sync stay quiet about intended state.
25. As a fleet user, I want a symlink into the canonical store in `~/.config/opencode/skills` or `~/.pi/agent/skills` to keep being removed as redundant, so that the skills CLI's link spam stays cleaned up.
26. As a fleet user, I want a real directory or a foreign link in any harness skills directory left untouched, so that fleet only ever manages its own symlinks.
27. As a fleet reader of the docs, I want CONTEXT.md, the per-harness reference, the CLI reference, and the undo guide to describe the single link model and the custom-versus-canonical lever rule, so that the documented behavior matches the code.
28. As a future maintainer, I want an ADR recording why customs toggle by link while canonicals toggle by config, so that the asymmetry does not read as an accident.
29. As a fleet user, I want no new config keys or state-file fields from this change, so that upgrading requires nothing beyond the automatic one-time cleanup.

## Implementation Decisions

**Exposure: one managed link per custom skill, for all six harnesses.**

- The config-path writing seam is removed in full: the `SourceWiring` and `SourceUnwiring` interfaces, their fan-out helpers, and their `WireResult`/`UnwireResult` report types are deleted. There are no implementers left once OpenCode and Pi stop writing collection paths.
- `OpenCodeAdapter` and `PiAdapter` gain the link-writer capability the other four already have, so all six harnesses reach custom skills through a managed symlink in their own skills directory. The link target is the skill directory inside its custom home, matching the existing managed-link convention; the existing low-level link primitive (create / repoint / leave a real dir alone / report unchanged) is reused unchanged.
- Because every harness now links customs, the fan-out that makes a custom home visible and the inverse used by drop become link-only. Their report shapes lose the "wired"/"unwired" sections; `sync`'s report loses the same, and the CLI prints "linked"/"unlinked" only.

**Toggle: source-aware, not harness-wide.**

- A custom skill's lever is its managed link on every harness that both links skills and natively scans the canonical store: OpenCode, Codex, Pi, Cursor, Bob. This is expressed as a single capability predicate (a harness is a skill linker **and** a native store scanner). It deliberately does not include Claude Code, which is a linker but not a native scanner.
- Sync's projection into harness config becomes source-aware: it consults the skill index and skips writing config off-entries for names the index resolves as custom, but only for harnesses in that predicate set. Canonical-store names (including unversioned store skills with no lockfile entry) still project to config as today.
- The toggle entry point routes a custom skill on a predicate-set harness to the link branch even when the harness can otherwise write config. Canonical skills and Claude Code keep the config branch. This preserves the existing rule that a harness with no lever at all records the state without writing config.
- Read side: OpenCode, Codex, and Pi begin reporting which requested names currently have a link in their skills directory, mirroring Cursor and Bob, so the snapshot can render `absent`. Their config-derived `Disables` are unchanged.
- Snapshot's custom-skill state override switches from the old "no config lever" predicate to the new capability predicate, so a custom with no link renders `absent` on OpenCode, Codex, and Pi as well as Cursor and Bob. Canonical-store skills are never overridden.
- Doctor's link-toggle drift analysis uses the same predicate, so it reports custom link drift on the expanded set while still exempting canonical-store skills.
- The `Disables` list each harness reports continues to drive doctor's config-versus-state comparison for canonical skills; the migration below removes fleet's legacy custom off-entries so they don't surface as conflicts.

**Migration: automatic, scoped, reported once.**

- On sync, before projection, fleet removes legacy fleet-owned entries whose target resolves into a current custom home or whose selected name is a current custom skill:
  - OpenCode: collection path entries in the `skills` config (both the V1 `skills.paths` object and the V2 flat array) and fleet-shape exact-name deny rules selecting a custom name.
  - Pi: collection path entries in the `skills` array and `-skills/<name>/SKILL.md` exact exclusions naming a custom skill.
  - Codex: fleet-shape `[[skills.config]]` blocks (simple name/enabled tables) selecting a custom name.
- Scope is strict: only entries matching a current custom home path or a current custom skill name are touched. A user's own additions to those arrays — for example a `~/.claude/skills` source entry — point elsewhere and are left alone, as is any entry fleet cannot identify.
- Each removal is reported as an explicit named line. After the first successful sync there is nothing left to remove, so later syncs are silent.
- No config key or state-file field is added; the state file schema is unchanged.

**Documentation and decision record.**

- CONTEXT.md gains a **Managed custom link** term (the symlink fleet creates into a custom home; for native-scanning link harnesses it is the custom skill's enable/disable lever), retires the "config-path harnesses" versus "link-based harnesses" exposure split, and rewrites the Sync, Enable/Disable, and Custom-skill entries around exposure-by-link plus the custom-versus-canonical lever rule.
- The per-harness reference, CLI reference, and undo guide are updated to drop the wiring narrative and describe uniform linking.
- A new ADR records the decision (customs toggle via managed links; canonical skills via harness config; Claude Code exempt), because it reverses a previously deliberate split and the asymmetry is surprising without context. An existing ADR is not rewritten: ADR 0001 only mentions wiring in a partly-superseded historical note, and ADR 0002 references "wire/link" generically.

**Cleanup.**

- `ReadResult.SkillSources` and its OpenCode parser become dead once OpenCode stops wiring; they are removed. Doctor does not currently consume them, so no reporting capability is lost.

## Testing Decisions

Good tests here assert external, user-visible behavior through the highest available seam and never encode internal structure. A test should be able to fail if the mechanism is wrong while staying silent if implementation details move.

**Primary seam — the CLI command layer with an injected temp home.** This is the highest existing seam and drives harness, customs, sync, snapshot, toggle, and doctor through one path. New coverage:

- `fleet skill off <custom> --harness opencode|codex|pi` removes the managed link and writes no config off-entry; the canonical-skill counterpart writes the harness's own off-entry.
- `fleet skill on <custom>` recreates the link and removes any legacy off-entry.
- `fleet skill ls` (and its JSON) reports `absent` for a custom with no link and unchanged `on`/`off` for canonical skills.
- `fleet skill sync` removes each legacy entry family (OpenCode paths and custom denies, Pi paths and custom exclusions, Codex custom config blocks), reports them by name, leaves a user-added unrelated source entry alone, and is a no-op on the second run.
- `fleet skill drop` unlinks the dropped collection across all six harnesses; `adopt` and `pull` link across all six.
- A symlink into the canonical store in a native scanner's skills directory is still removed as redundant; a real directory or foreign link is still untouched.

**Secondary seam — existing adapter-level tests in the harness package**, used only for primitives the end-to-end tests show indirectly: a custom link lands in the harness's own skills directory with the custom-home target, and `Read` reports link presence. This follows the existing link and Pi-config test patterns and introduces no new seam.

**Updated (not newly bounded) tests.** Existing unit tests in the customs, sync, snapshot, and doctor packages currently assert the old "two wired harnesses" split and the old predicate; they are updated to the unified-link expectations. Existing wiring tests are deleted with the seam.

## Out of Scope

- Changing how canonical-store skills are disabled per harness. OpenCode deny rules, Codex `[[skills.config]]`, and Pi force-exclude stay exactly as they are.
- Moving Claude Code's custom-skill disable from `skillOverrides` to link removal. Claude is intentionally exempt.
- Any use of `disable-model-invocation` or other cross-harness frontmatter levers.
- Reworking Pi's exclusion-path base or the directory-name-equals-frontmatter-name assumption already present across link handling.
- A migration for pre-public checkouts beyond the config-entry cleanup above (there is no config key or state schema change to migrate).

## Further Notes

- The Pi behavior this fixes is code-verified against earendil-works/pi: global settings paths resolve relative to `~/.pi/agent`, and a `-skills/<name>/SKILL.md` force-exclude is matched against every auto-scanned skills root with that root's own base directory. It therefore matches both `~/.agents/skills/<name>/SKILL.md` and `~/.pi/agent/skills/<name>/SKILL.md` — which is why a linked custom becomes disableable by a config entry, and why a wired custom never was.
- Pi follows directory symlinks in `~/.pi/agent/skills`, so a managed link there is discovered as a normal skill.
- The "custom" flag shown in `fleet skill ls` also covers an unversioned skill sitting in the canonical store (no lockfile entry). Such a skill has no managed link and is discovered natively, so it stays config-disabled; it is not link-toggled. This is mechanically forced, not a choice.
- The existing directory-name-equals-frontmatter-name assumption continues to underlie link naming and lookup for every harness; it is not changed here.
