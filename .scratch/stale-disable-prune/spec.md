# stale-disable-prune

Status: resolved

Stop projecting disables for skills that are installed nowhere, report the stale disable rules and state entries that remain, and add `fleet skill prune` to remove them.

## Problem Statement

When a skill is uninstalled, fleet keeps treating its recorded disable as live. On every command sync projects that disable into each harness's config, so configs accumulate off entries for skills that no longer exist. A concrete case: `ui-ux-pro-max` was uninstalled, but `~/.pi/agent/settings.json` still holds `-skills/ui-ux-pro-max/SKILL.md`, and the state file still lists it. There is no way to see the problem or clear it, short of running `fleet skill on <name>` once per stale name.

The user is stuck with three symptoms. Sync writes disable rules for skills that are gone. Doctor does not report the stale config rules. Doctor does not report the stale state entries. Nothing prunes either.

## Solution

Sync only writes a disable rule for a skill that is installed somewhere. A disable for an uninstalled skill stays in the state file as dormant intent, so a reinstall is still disabled on the next command, but it is not projected into any harness config.

Doctor reports the two leftovers: a fleet-owned disable rule already written into a harness config for an uninstalled skill, and a state entry for an uninstalled skill. Both point at a new `fleet skill prune` command that removes them.

Prune removes only what fleet owns, and only when every configured skill home is present and scans cleanly. When a home is missing or unreadable, prune does nothing and names the home that blocked it.

## User Stories

1. As a fleet user, I want sync to stop writing a disable rule for a skill that is installed nowhere, so that harness configs stop accumulating entries for skills I removed.
2. As a fleet user, I want the state file to keep the disable for a skill I uninstalled, so that reinstalling it comes back disabled without me setting it again.
3. As a fleet user, I want sync to stay quiet about a dormant disable, so that my normal commands are not noisy.
4. As a fleet user, I want sync to leave an already-written disable rule for an uninstalled skill alone, so that nothing is removed behind my back.
5. As a fleet user, I want sync to still flag a disable rule in a config that the state file does not track, so that manual edits stay visible.
6. As a fleet user, I want doctor to report a fleet-owned disable rule for an uninstalled skill, so that I can see the leftover.
7. As a fleet user, I want doctor to report a state entry for an uninstalled skill, so that I can see the stale record.
8. As a fleet user, I want doctor to say which skill and which harness each stale finding concerns, so that I can judge it.
9. As a fleet user, I want doctor to name `fleet skill prune` in the finding, so that I know the fix.
10. As a fleet user, I want doctor to warn that pruning a state entry loses the disable-on-reinstall behavior, so that I make the choice knowingly.
11. As a fleet user, I want doctor to stay read-only, so that a checkup never changes anything.
12. As a fleet user, I want `fleet skill prune` to list what it would remove before removing anything, so that I can review it.
13. As a fleet user, I want `fleet skill prune` to require a confirmation flag before it acts, so that a destructive command never runs by accident.
14. As a fleet user, I want `fleet skill prune` to remove the stale disable rules from harness configs, so that the configs stop denying skills that are gone.
15. As a fleet user, I want `fleet skill prune` to remove the stale state entries, so that the state file stops carrying dead records.
16. As a fleet user, I want to limit prune to one harness, so that I can clean a single config without touching the rest.
17. As a fleet user, I want to prune only the config rules or only the state entries, so that I can keep the disable-on-reinstall intent while clearing a config.
18. As a fleet user, I want prune to remove only fleet's own exact disable shapes, so that pattern rules, blanket rules, and my own values are never touched.
19. As a fleet user, I want prune to fail closed when a skill home is missing or unreadable, so that it never removes an entry for a skill that is actually installed in a home it could not scan.
20. As a fleet user, I want prune to name the home that blocked it, so that I can fix the scan and rerun.
21. As a fleet user, I want prune to run sync afterward, so that the configs and the state file agree once it finishes.
22. As a fleet user, I want prune to be idempotent, so that a second run reports nothing to do.
23. As a fleet user with custom skills in tracked repos, I want a skill in any tracked collection to count as installed, so that a disable is not called stale while the skill is still there.
24. As a fleet user, I want a configured repo that is missing from disk to block prune, so that a moved or deleted checkout does not make its skills look uninstalled.
25. As a Claude Code user, I want the existing link-based discoverability rule to keep working, so that a disable for a skill Claude cannot reach is still handled as it is today.
26. As a fleet user, I want `fleet skill update` to stop reporting an uninstalled disabled skill as "did not stay disabled", so that the update report is not full of false failures.
27. As a fleet user, I want the update report to still verify a disable for a skill that is installed, so that real failures are not hidden.
28. As a fleet user, I want doctor to suppress stale findings when a scan is incomplete, so that healthy skills are never flagged as stale.
29. As a fleet user, I want doctor to surface the home that made the scan incomplete, so that I know why stale findings are absent.
30. As a fleet user, I want the new command to follow the existing `drop:` and `adopt:` output style, so that the CLI reads consistently.
31. As a fleet user, I want the docs site to describe the new command and the dormant-disable behavior, so that the feature is discoverable.

## Implementation Decisions

The shared notion of "installed". A skill is installed when it exists in the canonical store or any tracked custom home (the explicit repo list, the fleet-home checkouts, or the fleet-home fallback), matched by directory name or frontmatter name. This reuses the skill index, the single scanner. Claude's link-based discoverability stays a separate, per-harness dimension layered on top as it is today.

The completeness signal. The skill index result gains a `Complete` flag: true only when the canonical store exists and scans, and every tracked collection is present on disk and scans without error. A missing fleet-home fallback is not incomplete, since it is optional by design. A tracked repo root that is missing from disk is incomplete. Any scan error is incomplete. "Uninstalled" may only be asserted when `Complete` is true.

Sync behavior. Sync keeps passing every state-disabled name for a harness in its write batch, but marks each name as installed or not. A disable for a name that is installed nowhere is dormant: the adapter never creates a new off entry for it, but the name still counts as fleet-owned, so the existing unmanaged-entry sweep does not flag a rule that is already there. This generalizes the branch Claude already has for a skill it cannot discover. When the scan is incomplete, sync falls back to projecting every disable, since leaving an installed skill enabled is worse than a stale rule.

The dormant marker on a write. A skill write carries whether the skill is installed. A dormant disable is counted by the flag sweep's written set but skipped by the projection loop. This is the smallest change that keeps the existing flag semantics without teaching every adapter to scan.

Doctor findings. Two new finding kinds: one for a fleet-owned config rule whose skill is installed nowhere, and one for a state entry whose skill is installed nowhere. Each names the skill and harness and points at `fleet skill prune`. The state-entry finding states that pruning it loses the disable-on-reinstall behavior. Doctor stays read-only.

Doctor enablement comparison. The config-versus-state comparison must account for dormancy. For an uninstalled skill, a config that does not disable it is expected, not drift, and a config that still disables it is the new stale-config finding. For an installed skill, the existing conflict and drift cases are unchanged. When the scan is incomplete, doctor suppresses both stale findings and keeps the existing comparison, because a name that looks uninstalled might be installed in the unscanned home.

The prune operation. A new package exposes one operation that takes a `*paths.Paths` and returns a report of what it removed and what it skipped. It removes stale config rules only in the exact shapes the write side owns: the OpenCode V1 exact `permission.skill` deny, the OpenCode V2 three-key deny rule for the exact resource, the Pi exact `-skills/<name>/SKILL.md` entry, the Codex simple `[[skills.config]]` block with `enabled = false`, and the Claude `skillOverrides` value of `"off"`. Patterns, blankets, extra-key shapes, and foreign values are never touched. It removes stale state entries with the existing state enable path, which deletes a skill entry once nothing remains. It refuses to act when the scan is incomplete and reports the blocking home.

The prune scope. Prune's config axis covers only fleet-owned rules for skills that are state-disabled and installed nowhere. A config rule for a skill the state file does not track stays doctor's existing manual-edit business. The state axis covers every state entry whose skill is installed nowhere, including entries for the link-toggleable harnesses.

The prune command. `fleet skill prune` is one verb. With no flags it lists the stale config rules and stale state entries and does nothing. `--yes` applies. `--harness` is repeatable and limits the work to named harnesses. `--config-only` and `--state-only` select one axis. After a successful prune it runs sync, as every command does. Output follows the `drop:` and `adopt:` palette.

The update report. The disable verification skips names that are installed nowhere when the scan is complete, instead of counting them as held or lost. Installed names keep the current verification. This retires the assertion that an uninstalled disable is "verified disabled".

The state file is unchanged. No schema change. The dormant intent lives in the existing sparse entry. Sync still never edits the state file.

## Testing Decisions

Good tests here drive a fake home and assert observable behavior: config file bytes, the state file bytes, and the returned reports or findings. They do not assert internal call counts or private helpers.

Sync tests reuse the existing `sync.Run` seam and the fake-home pattern in the sync tests. Cases: an uninstalled disabled skill is not written to any config lever; an existing off entry for an uninstalled skill is left in place and not flagged; a disable rule for a name the state does not track is still flagged; an incomplete scan falls back to projecting the disable; sync still never edits the state file.

Doctor tests reuse the existing `doctor.Analyze` seam. Cases: a fleet-owned config rule for an uninstalled skill produces the stale-config finding; a state entry for an uninstalled skill produces the stale-state finding with the reinstall warning; an installed skill with a missing rule still produces the existing drift or conflict; an incomplete scan suppresses both stale findings and surfaces the blocking home.

Prune tests live in the new package and drive the operation over a fake home. Cases: only fleet-owned exact shapes are removed; a pattern rule, a blanket rule, and a foreign value survive; state entries are removed and an emptied entry disappears; an incomplete scan removes nothing and names the blocker; a second run is a no-op.

CLI tests reuse the fake-home pattern from the toggle and drop command tests. Cases: no `--yes` lists and changes nothing; `--yes` applies; `--harness` limits; `--config-only` and `--state-only` select an axis; output follows the existing palette.

Update tests adjust the existing disable-outlives-skill case to assert the disable is dormant rather than held, and keep a case where an installed disabled skill is still verified.

Skill index tests cover the completeness flag: a missing store, a missing tracked repo, and a scan error each make it incomplete; an absent fleet-home fallback does not.

## Out of Scope

- Removing stale config rules during sync. Sync stops writing new ones; removal is explicit through prune.
- Pruning config entries fleet does not own, such as pattern rules, blanket rules, and manual entries.
- Changing Claude's link-based discoverability model.
- Automatic garbage collection of state entries. The state entry survives until the user prunes it.
- Any change to how the skills CLI installs or updates skills.
- Updating the docs site content, which is a separate ticket.

## Further Notes

This changes documented intent. The state package comments and the update report currently say a disable may outlive its skill and count it as held. That stays true for the state file, but config projection and the update report change. The test `TestUpdateReportsADisableThatOutlivedItsSkill` encodes the old behavior and must be rewritten to assert dormancy.

The user's phrase "fleet config" in the originating report meant the state file. This spec uses "state file" throughout, since the fleet config file holds repo paths and the adopt target, not skill names.

No ADR is contradicted. The completeness check leans on the tracked set defined by ADR 0001 and ADR 0002.

The main risk is a false stale call from an incomplete scan. The fail-closed rule is the guard: no stale assertion without a complete scan. The store being absent blocks prune, and doctor's existing missing-store finding explains why.
