---
title: Undo & Escape Hatches
description: Running skills by hand, resetting state, how sync decides.
---

# Undo and escape hatches

Fleet is designed to never become a hard dependency: the skills CLI keeps working when run by hand, the canonical store is never touched, and everything fleet writes is the harness's own native config format. This page covers the ways out.

## Run the `skills` CLI independently

The `skills` CLI stays the install and update backend. Run it exactly as you always did:

```sh
skills add -g -y -s <names>   # install, exactly as fleet never needs to change
skills update -g              # update everything, interactively
```

Fleet survives this by construction:

- it never writes the skills CLI lockfile, and never deletes entries it doesn't own
- the state file records intent, and nothing the skills CLI does erases it
- skills stay in the canonical store, where the skills CLI expects them

What you will see afterwards is sync at work. A hand-run `skills update` re-creates the per-agent symlinks and may resurrect enablement; the next fleet command (any of them — sync runs on every command) re-removes the redundant links and re-applies the recorded disables, printing one line per fix. To converge on demand instead of waiting for the next command, `fleet skill sync` is that repair as a verb:

```sh
$ fleet skill sync
sync: opencode: removed redundant link "tdd" — opencode scans the canonical store natively — this link double-covers the skill
sync: opencode: disabled "tdd" (was on)
```

`skills check` exists but is an undocumented alias of `update`; fleet doesn't use it.

## Re-enable a skill

`fleet skill on <name>` removes fleet's disable entries everywhere (or for the harnesses you name). This is the surgical undo, and it is deliberately lenient: it also cleans up entries for skills that were uninstalled while disabled.

```sh
fleet skill on tdd              # everywhere
fleet skill on tdd --harness pi # one harness
```

To clear every stale disable at once instead of naming each skill, `fleet skill prune` removes the config rules and state entries for skills that are installed nowhere (see [prune](cli.md#fleet-skill-prune)).

## Uninstall a skill that is disabled

Uninstalling a skill, whether you remove it from `~/.agents/skills` or from its custom home, does not erase its recorded disable. The state file keeps the disable as dormant intent: sync stops writing new off entries for it, so harness configs stop collecting rules for skills you removed, but reinstalling the skill comes back disabled without you setting it again.

Two leftovers can remain: a fleet-owned rule some harness config still carries, and the dormant state entry itself. `fleet skill prune` clears them:

```sh
fleet skill prune                     # list what it would remove
fleet skill prune --yes               # remove both leftovers
fleet skill prune --config-only --yes # drop the config rule, keep the disable-on-reinstall intent
```

Doctor reports both leftovers and points at prune (see [doctor](cli.md#fleet-skill-doctor)). Pruning a state entry loses the disable-on-reinstall behavior, so reach for `--config-only` when you want a clean config but plan to reinstall.

## Untrack a custom dir

`fleet skill remove-dir <path>` unlists a collection dir from `skillsDirs` and removes its managed links from every installed harness. It is the undo for `fleet skill add-dir`, and it never deletes the directory or any file in it:

```sh
fleet skill remove-dir ~/Developer/projects/agent-skills/skills   # stop tracking, keep the files
```

After it runs, the skills in that dir are no longer discovered or linked, but they still sit exactly where they did on disk. Run `add-dir` again to bring them back. A path that is tracked but already hand-deleted still unlists cleanly, since `remove-dir` never stats the target.

## Reset fleet's state

The state file is `~/.config/fleet/state.json` ([schema](state-file.md)). Deleting it resets fleet's memory:

```sh
rm ~/.config/fleet/state.json
```

After this, fleet treats every skill as enabled and stops writing disables. Note what does _not_ happen: the disable entries already written into harness configs stay there. Sync only writes disables the state records; it does not hunt down entries the state no longer mentions. Two clean ways to clear the leftovers:

- Run `fleet skill doctor -i`. Each leftover is a conflict prompt — `restore` strips the entry from the config, `keep` records it in the (now empty) state file.
- Edit the harness configs by hand. Fleet's entries are plain values in each harness's own format (a `deny` rule, a `"-skills/<name>/SKILL.md"` array entry, an `enabled = false` block, a `"skillOverrides"` key); removing them changes nothing else. A disabled custom skill's managed link is removed with `fleet skill on` or by deleting the symlink by hand.

The same applies to hand-edits generally: doctor surfaces them, sync never silently overwrites them. Manual edits you _keep_ become the state's new intent.

`~/.config/fleet/` also holds `config.json` (the machine-local customs settings: the `skillsDirs` tracked-dir list + adopt target), `skills/` (the unversioned fleet-home fallback), and `tree-cache.json` (the update-check cache). Deleting `tree-cache.json` costs a few GitHub API calls on the next `ls`, nothing more; `config.json` and `skills/` survive a state reset by design.

## How sync decides what to touch

Sync's rules, in the order it applies them:

1. **Load the state file fresh**, so commands that just wrote it sync their own change.
2. **Clean up legacy entries.** On the first sync after upgrading, remove the fleet-owned entries a previous release wrote: collection paths in OpenCode's and Pi's config, and fleet-shape exact-name off-entries on OpenCode, Codex, and Pi that name a current custom skill. Each removal prints as `sync: <harness>: removed legacy <collection path|disable entry> "<value>"`. Scope is strict, so config fleet did not write (a path you added yourself, a rule of another shape) is left alone, and later syncs find nothing.
3. **Make customs visible.** Every scanned custom home — each tracked collection plus the fleet-home fallback (a configured adopt target is included when it is one of those; a standalone target stays a doctor warning) — is linked into every installed harness, one managed symlink per skill. A custom skill the state disables on a native-scanning harness (OpenCode, Codex, Pi, Cursor, Bob) is the exception: the managed link is that harness's only lever for a custom, so it is left unlinked and any existing link is removed here (reported as `unlinked`). A visible home reports nothing, so this is idempotent; it is what keeps a hand-created custom skill reachable in every harness without running `adopt` again.
4. **Remove redundant links.** In each installed harness's skills dir, a symlink counts as redundant only when it provably resolves into the canonical store _and_ that harness scans the store natively (OpenCode, Pi, Codex, Cursor, Bob — never Claude Code, whose store links are its only discovery path). Everything else in those dirs survives: real directories and files, links into any tracked collection or the fleet-home fallback (including your own and fleet's managed adopt links), links pointing anywhere else, and broken links, which doctor reports instead. Sync never removes tracked-collection or fallback links.
5. **Project the disables.** For each installed harness with a config write side (OpenCode, Pi, Codex, Claude Code), write that harness's own off-entry for every disable the state records whose skill is installed somewhere, except a custom skill on OpenCode, Codex, or Pi, where the managed link from step 3 is the lever and no config entry is written. A disable for a skill installed nowhere is dormant: the state keeps the intent, sync writes nothing new, and a rule already in a config stays for `fleet skill prune`. Claude Code writes `skillOverrides` for custom and canonical skills alike. Cursor and Bob project through the managed link in step 3 instead. Nothing else. An absent entry means on; sync never writes "on" markers, because "on" is what every harness does by default. When the scan is incomplete, sync projects every disable anyway, because leaving an installed skill enabled is worse than a stale rule.
6. **Flag what it doesn't recognize.** Pattern and blanket rules, glob exclusions, Codex blocks with extra keys — anything that affects enablement but isn't fleet's exact shape is printed as a flag and left untouched. Sync reports; you decide.
7. **Never edit the state file.** Sync is one-directional on purpose: state is the source of truth, configs are outputs. (`~/.config/fleet/config.json` is the separate machine-local customs file. Sync reads the adopt target from it but never writes it — `fleet config` and `fleet skill add-dir`/`remove-dir` manage it.)

`fleet skill doctor` runs the same inspection read-only, so you can see all of it before any of it happens.

## Uninstall fleet

Fleet is one static binary plus a config dir. Remove the binary, then optionally `rm -rf ~/.config/fleet` to forget the state, the customs config (`config.json`), the fleet-home customs (`skills/`), and the update cache. Nothing else needs cleaning:

- harness configs keep fleet's entries, which are their own native format — every harness works without fleet knowing about it
- installed skills stay in the canonical store, updating through the skills CLI as always
- custom skills stay in their collections (a tracked collection dir or `~/.config/fleet/skills`), still linked wherever add-dir, adopt, or sync linked them (remove those links by hand if you want them gone; doctor would flag the leftovers as unknown entries, but nothing breaks)

## Undo an adoption

Adoption moves a skill directory into the adopt destination (a tracked collection or `~/.config/fleet/skills`) and links it into every installed harness. Reversing it is manual by design — the skill is yours now, and fleet never moves files back on its own:

1. Move the directory back into the canonical store (or `git mv` it, if it lived in a git-tracked collection):
   ```sh
   mv ~/Developer/projects/agent-skills/skills/my-skill ~/.agents/skills/my-skill
   mv ~/.config/fleet/skills/my-skill ~/.agents/skills/my-skill  # when the fallback was the destination
   ```
2. Remove the links if you want them gone: the per-harness symlinks named after the skill in `~/.config/opencode/skills`, `~/.pi/agent/skills`, `~/.codex/skills`, `~/.claude/skills`, `~/.cursor/skills`, and `~/.bob/skills`.

Doctor reports any leftovers as unknown entries and never deletes them. It also flags the two things an adoption leaves behind that nothing else reports:

- The skills CLI lockfile still carries the skill's install entry while the skill lives in a custom home, so the CLI keeps trying to update a skill that moved. Remove the entry from `~/.agents/.skill-lock.json` by hand; fleet reads the lockfile and never writes it.
- If the store copy comes back while a custom-home copy remains — a half-finished move in either direction — or a name exists in two scanned sources, a native-scanning harness would see the skill twice. Doctor flags the double presence; remove one of the copies by hand.

Once the skill is back in the store, the skills CLI and fleet treat it like any other installed (or custom, if it has no lock entry) skill.
