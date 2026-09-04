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

## Reset fleet's state

The state file is `~/.config/fleet/state.json` ([schema](state-file.md)). Deleting it resets fleet's memory:

```sh
rm ~/.config/fleet/state.json
```

After this, fleet treats every skill as enabled and stops writing disables. Note what does _not_ happen: the disable entries already written into harness configs stay there. Sync only writes disables the state records; it does not hunt down entries the state no longer mentions. Two clean ways to clear the leftovers:

- Run `fleet skill doctor -i`. Each leftover is a conflict prompt — `restore` strips the entry from the config, `keep` records it in the (now empty) state file.
- Edit the harness configs by hand. Fleet's entries are plain values in each harness's own format (a `deny` rule, a `"-skills/<name>/SKILL.md"` array entry, an `enabled = false` block, a `"skillOverrides"` key); removing them changes nothing else.

The same applies to hand-edits generally: doctor surfaces them, sync never silently overwrites them. Manual edits you _keep_ become the state's new intent.

`~/.config/fleet/` also holds `config.json` (the machine-local customs settings: explicit repo list + adopt target), `skills/` (the unversioned fleet-home fallback), `repos/` (auto-tracked customs checkouts), and `tree-cache.json` (the update-check cache). Deleting `tree-cache.json` costs a few GitHub API calls on the next `ls`, nothing more; `config.json`, `skills/`, and `repos/` survive a state reset by design.

## How sync decides what to touch

Sync's rules, in the order it applies them:

1. **Load the state file fresh**, so commands that just wrote it sync their own change.
2. **Remove redundant links.** In each installed harness's skills dir, a symlink counts as redundant only when it provably resolves into the canonical store _and_ that harness scans the store natively (opencode, pi, codex, Cursor, Bob — never claude code, whose store links are its only discovery path). Everything else in those dirs survives: real directories and files, links into any tracked collection or the fleet-home fallback (including your own and fleet's managed adopt/pull links), links pointing anywhere else, and broken links, which doctor reports instead. Sync never removes tracked-collection or fallback links.
3. **Project the disables.** For each installed harness with a write side (opencode, pi, codex, claude code), write that harness's own off-entry for every disable the state records. Nothing else. An absent entry means on; sync never writes "on" markers, because "on" is what every harness does by default.
4. **Flag what it doesn't recognize.** Pattern and blanket rules, glob exclusions, codex blocks with extra keys — anything that affects enablement but isn't fleet's exact shape is printed as a flag and left untouched. Sync reports; you decide.
5. **Never edit the state file.** Sync is one-directional on purpose: state is the source of truth, configs are outputs. (`~/.config/fleet/config.json` is the separate machine-local customs file — `fleet config` and `fleet skill pull` manage it, sync never touches it.)

`fleet skill doctor` runs the same inspection read-only, so you can see all of it before any of it happens.

## Uninstall fleet

Fleet is one static binary plus a config dir. Remove the binary, then optionally `rm -rf ~/.config/fleet` to forget the state, the customs config (`config.json`), the fleet-home customs (`skills/`), the pulled checkouts (`repos/`), and the update cache. Nothing else needs cleaning:

- harness configs keep fleet's entries, which are their own native format — every harness works without fleet knowing about it
- installed skills stay in the canonical store, updating through the skills CLI as always
- custom skills stay in their collections (a tracked repo's `skills/` or `~/.config/fleet/skills`), still linked or wired wherever adopt wired them (remove those entries or links by hand if you want them gone; doctor would flag the leftovers as unknown entries, but nothing breaks)

## Undo an adoption

Adoption moves a skill directory into the adopt destination (a tracked collection or `~/.config/fleet/skills`) and wires it in. Reversing it is manual by design — the skill is yours now, and fleet never moves files back on its own:

1. Move the directory back into the canonical store (or `git mv` it, if it lived in a customs repo):
   ```sh
   mv ~/.config/fleet/repos/my-customs/skills/my-skill ~/.agents/skills/my-skill
   mv ~/.config/fleet/skills/my-skill ~/.agents/skills/my-skill  # when the fallback was the destination
   ```
2. Remove the wiring and links if you want them gone: the destination's path entry in opencode's and pi's configs, and the per-harness symlinks named after the skill in `~/.codex/skills`, `~/.claude/skills`, `~/.cursor/skills`, `~/.bob/skills`.

Doctor reports any leftovers as unknown entries and never deletes them. It also flags the two things an adoption leaves behind that nothing else reports:

- The skills CLI lockfile still carries the skill's install entry while the skill lives in a custom home, so the CLI keeps trying to update a skill that moved. Remove the entry from `~/.agents/.skill-lock.json` by hand; fleet reads the lockfile and never writes it.
- If the store copy comes back while a custom-home copy remains — a half-finished move in either direction — or a name exists in two scanned sources, opencode and pi would see the skill twice. Doctor flags the double presence; remove one of the copies by hand.

Once the skill is back in the store, the skills CLI and fleet treat it like any other installed (or custom, if it has no lock entry) skill.
