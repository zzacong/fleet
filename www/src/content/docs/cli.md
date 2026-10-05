---
title: Command Reference
description: Every verb, flag, and error with examples.
---

# Command reference

Fleet has one binary and one command family. Bare `fleet` opens the interactive matrix; everything scripted lives under `fleet skill`. Sync runs inside every command except the two read-only ones (`fleet skill doctor` and `fleet harness ls`), and `fleet skill sync` is that machinery as an explicit verb (see [Sync](#sync)).

Examples were run against a sandbox home (`FLEET_HOME=$(mktemp -d)`) with two skills: `tdd`, installed from a source repo, and `git-helper`, a custom skill. Paths are shortened to `~` for readability.

## fleet

```sh
fleet
```

Opens the skill × harness matrix: one row per skill, one column per installed harness. Toggles are staged with `space` and applied together with `enter` — the same state-file-then-sync write path the `on`/`off` verbs run.

Keys:

| Key               | Action                                                                 |
| ----------------- | ---------------------------------------------------------------------- |
| `j` / `k`, arrows | move the skill cursor                                                  |
| `h` / `l`, arrows | move the harness column                                                |
| `space`           | stage or unstage the selected cell                                     |
| `enter`           | apply staged changes                                                   |
| `u`               | update all installed skills (wrapped skills CLI, then sync)            |
| `U`               | update the selected skill (wrapped `skills update <skill>`, then sync) |
| `esc`             | discard staged changes, clear the filter, or close help                |
| `/`               | filter by name or description                                          |
| `r`               | refresh                                                                |
| `?`               | help overlay                                                           |
| `q`, `ctrl+c`     | quit (`q` is text while the filter holds input)                        |

Cells read `●` on, `○` off, `-` absent. Columns for Cursor and Bob render faint with a `!` in the header: they have no config off switch, so a stored skill can't be toggled there. A custom skill can — its managed link is the toggle — so its cell shows the real state.

`u` is the one-key update-all: it runs the same wrapped `skills update -g -y` the [update verb](#fleet-skill-update) runs, then sync, then reloads the matrix so the new badges and states show. `U` does the same for the selected skill only (`skills update -g -y <skill>`). A busy line takes over while either runs, the outcome notice reports fleet's own post-run state — the store scan and lockfile, never the skills CLI's prose — and a failed run shows the CLI's captured output raw.

The hero banner appears on launch only. `fleet --quiet` (or `-q`) keeps the matrix without it. With piped output fleet never enters the TUI: it prints the same listing as `fleet skill ls` instead, so `fleet | grep tdd` does what you mean.

## fleet skill ls

```sh
fleet skill ls [--json] [--quiet]
```

Lists every skill fleet discovers — the canonical store (`~/.agents/skills`) plus custom skills from the tracked set (the `skillsDirs` collection dirs in list order, and the unversioned fleet-home fallback `~/.config/fleet/skills`) — with a column per installed harness. A name present in more than one source appears once with precedence `skillsDirs` order, then the fallback, then the canonical store; the other copies are doctor drift, not a silent overwrite. Custom skills come first, then installed skills grouped by source repo.

```sh
$ fleet skill ls
NAME        OPENCODE  PI  CODEX  CLAUDE  CURSOR  BOB  UPDATE  SOURCE       DESCRIPTION
git-helper  on        on  on     -       on      on   —       custom       Hand-written commit-message helper.
tdd         on        on  on     -       on      on   ↑       example/tdd  Test-driven development discipline.
```

- The enablement columns sit right after the name — they are the table's point. The description goes last, truncated to whatever room the terminal has left, so no column ever wraps mid-word; piped output keeps the plain 60-column cap.
- `SOURCE` is `custom` (the skill lives in a tracked collection or the fleet-home fallback, or has no lockfile entry) or the source repo the lockfile records.
- An unversioned skill in the canonical store (no lockfile entry) also shows `custom`, but it has no managed link. A native-scanning harness discovers it natively, so it stays config-disabled there, not link-toggled. The link-toggle rule applies only to a custom that lives in a tracked collection or the fleet-home fallback.
- `UPDATE` is the outdated badge: `↑` update available, `✓` current, `?` unknown, `—` never checked (custom skills). Non-GitHub sources and failed checks are `?` — fleet checks GitHub directly, one API call per source repo, cached for an hour under the fleet config dir, and reports unknown rather than guessing.
- A harness column shows `on`, `off`, or `-`. `-` means the harness cannot discover the skill at all: for Claude Code that is the normal state until a link exists, and for a native-scanning harness it is a custom skill whose managed link was removed (see [Per-Harness Reference](harnesses.md#states-across-harnesses)).
- Color on a terminal only: `on` green, `off` dim, `↑` yellow, `✓` green, custom cyan. Piped output is plain.
- On a terminal a one-line summary prints above the table (`fleet · 2 skills · 1 installed · 1 custom · …`). Piped output skips it, and `--quiet` skips it everywhere.

Sync runs first. Its reports go to **stderr**, so stdout stays machine-readable; flags that share a message collapse into one line with the skill names:

```sh
$ fleet skill ls > /dev/null
sync: opencode: removed redundant link "tdd" — opencode scans the canonical store natively — this link double-covers the skill
sync: pi: disabled in config but not tracked by fleet's state — left alone (3 skills): tdd, git-helper, deploy-vercel
```

### --json

```sh
$ fleet skill ls --json
```

```json
{
  "harnesses": ["opencode", "pi", "codex", "claude", "cursor", "bob"],
  "skills": [
    {
      "name": "tdd",
      "custom": false,
      "source": "example/tdd",
      "sourceType": "git",
      "hash": "0123456789abcdef0123456789abcdef01234567",
      "installedAt": "2026-08-01T10:00:00Z",
      "updatedAt": "2026-08-01T10:00:00Z",
      "description": "Test-driven development discipline.",
      "states": {
        "opencode": "on",
        "pi": "on",
        "codex": "on",
        "claude": "absent",
        "cursor": "on",
        "bob": "on"
      },
      "outdated": null
    }
  ]
}
```

- `states` values are `"on"`, `"off"`, or `"absent"`, keyed by harness ID.
- `outdated` is the tri-state badge: `true` = update available, `false` = current, `null` = unknown. The key is always present so scripts can rely on it.
- Provenance fields (`source`, `sourceType`, `hash`, `installedAt`, `updatedAt`) are omitted for custom skills.
- Warnings (a failed update check, for instance) go to stderr as `warning: …` lines; the command still exits 0.

## fleet skill off

```sh
fleet skill off <name> [--harness <harness>]...
```

Disables a skill: records the toggle in the state file, then sync projects it into each harness. A canonical-store skill becomes that harness's own config off-entry where one exists; a custom skill on a native-scanning harness (OpenCode, Codex, Pi, Cursor, Bob) becomes a removed managed link. The skill's files stay where they are, so a stored skill keeps updating and a custom skill stays in its collection.

```sh
$ fleet skill off tdd
skill: opencode: disabled "tdd"
skill: pi: disabled "tdd"
skill: codex: disabled "tdd"
skill: cursor: no per-skill disable mechanism — disable "tdd" is a no-op
skill: bob: no per-skill disable mechanism — disable "tdd" is a no-op
```

- The outcome is one line per harness (`skill: opencode: disabled "tdd"`). On a terminal the `skill:` prefix is dim, the harness cyan, the verb green.
- Without `--harness`, every installed harness is targeted. `--harness` is repeatable and takes a harness ID: `opencode`, `pi`, `codex`, `claude`, `cursor`, `bob`.
- Cursor and Bob have no config per-skill disable. For a stored skill there is no lever at all, so toggling prints the no-op line and records nothing. A custom skill is different: their managed link (in `~/.cursor/skills` / `~/.bob/skills`) is the only path to it, so `off` removes that link and `on` restores it. The state file remembers the choice and sync never recreates a hidden link. `ls` then reports the custom `absent` for that harness.
- OpenCode, Codex, and Pi use the same link lever for a custom skill, because they link skills and scan the canonical store natively. `off` removes the managed link and `on` recreates it, and no config off-entry is written for a custom. Their config lever (OpenCode deny rule, Codex `[[skills.config]]`, Pi force-exclude) still applies to canonical-store skills. Claude Code is the exception: it links skills but does not scan the store, so it toggles both custom and canonical skills through `skillOverrides`.
- A harness where something else overrode the write (a foreign config entry that keeps the skill enabled) is left out of the outcome list; its flag line prints instead — `sync: codex/tdd: a skills.config entry fleet doesn't manage overrides fleet's disable — left alone`.
- Sync runs as part of the command and reports real repairs it made beyond the toggle: custom links, redundant-link removals, the one-time legacy cleanup, and drift fixes print as `sync:` lines. Ambient findings about other skills (untracked config disables, foreign rules) stay out of the report; `fleet skill sync` and `fleet skill doctor` are where they are listed.
- The name must exist in the canonical store or a custom home (a tracked collection or the fleet-home fallback). Disabling a typo would silently record state, so it fails loudly:

  ```sh
  $ fleet skill off typo-skill
  Error: skill "typo-skill" not found in ~/.agents/skills or a custom home
  ```

- The command is idempotent: disabling an already-disabled skill changes nothing, and the outcome says so — `skill: opencode: "tdd" is already disabled` (one line per harness, `already` dim, verb green).
- A disable outlives the skill. If you uninstall a disabled skill, the state file keeps the disable as dormant intent: sync stops writing new off entries for it, so harness configs stop collecting rules for skills you removed, but reinstalling the skill comes back disabled. A rule already written stays where it is; `fleet skill prune` removes it (see [prune](#fleet-skill-prune)).

## fleet skill on

```sh
fleet skill on <name> [--harness <harness>]...
```

Re-enables a skill by removing fleet's disable entries. Same targeting rules as `off`. For a custom skill disabled on a native-scanning harness (OpenCode, Codex, Pi, Cursor, Bob), it recreates the managed link the disable removed.

```sh
$ fleet skill on tdd --harness codex
skill: codex: enabled "tdd"
```

- The outcome is one line per harness (`skill: codex: enabled "tdd"`), and reads `skill: codex: "tdd" is already enabled` (`already` dim, verb green) when nothing had to change. A harness where a foreign rule still disables the skill stays out of the list; its flag line explains (`sync: pi/tdd: still excluded by a !glob entry — left alone`).

`on` is deliberately more lenient than `off`: it also cleans up entries for skills that were uninstalled while disabled, so stale state disappears instead of accumulating. Use `fleet skill prune` to clear every stale skill at once.

## fleet skill adopt

```sh
fleet skill adopt <name> [--into <skills-dir>]
```

Moves a custom skill from the canonical store (`~/.agents/skills`) into the adopt destination, where it stays versioned. The destination resolves as: `--into <skills-dir>` for this run, else the configured adopt target (`fleet config set adopt-target <skills-dir>`), else a numbered choice over the tracked collection dirs plus the always-offered fleet-home fallback (`~/.config/fleet/skills`). With no tracked dirs the fallback wins with no prompt. Then fleet links the skill into every installed harness:

```sh
$ fleet skill adopt git-helper
adopt: adopted "git-helper"
  from ~/.agents/skills/git-helper
    → ~/Developer/projects/agent-skills/skills/git-helper     # or ~/.config/fleet/skills/git-helper with no target set
adopt: opencode: linked "git-helper" → ~/Developer/projects/agent-skills/skills/git-helper
adopt: pi: linked "git-helper" → ~/Developer/projects/agent-skills/skills/git-helper
adopt: codex: linked "git-helper" → ~/Developer/projects/agent-skills/skills/git-helper
adopt: claude: linked "git-helper" → ~/Developer/projects/agent-skills/skills/git-helper
adopt: cursor: linked "git-helper" → ~/Developer/projects/agent-skills/skills/git-helper
adopt: bob: linked "git-helper" → ~/Developer/projects/agent-skills/skills/git-helper
```

- The first lines are the outcome: the skill now lives in the destination. On a terminal the `adopt:` prefix is dimmed, the verb is green and harness names are cyan; it is the lines to read. The headline breaks into three lines so both the original store path and the destination are visible. Managed links that were already correct stay quiet — only the links that actually changed print, with `adopt: codex: repointed "git-helper" (was ~/.agents/skills/git-helper) → ~/Developer/projects/agent-skills/skills/git-helper` for a skills CLI link taken over, or a `— left alone` note when a real directory is in the way.
- Sync runs as part of the command, but ambient findings about other skills stay out of the report; `fleet skill sync` and `fleet skill doctor` are where they are listed.

- `--into` takes a collection dir (not a repo root): it is created on demand, wins with no prompt, and is never saved. A choice picked from the prompt offers a yes/no follow-up (default No) to save it as the adopt target, so persisting is deliberate. Without a terminal, an ambiguous adopt fails listing the numbered candidates and the `--into` hint instead of blocking on stdin — automation never hangs.
- The prompt always includes the fleet-home fallback alongside the tracked collection dirs, so even a single tracked collection is an explicit choice against the default. The configured target may point somewhere unscanned — adopt still proceeds, and doctor surfaces an `unscanned adopt target` warning so the footgun is visible.
- Every harness gets a managed symlink named after the skill, pointing at the destination, in that harness's own skills directory. Managed links never point into the canonical store, so a native scanner never sees an adopted skill twice.
- Adoption records nothing in the state file: custom is defined by living in a tracked collection or the fallback. Disables recorded before adoption keep applying, because config rules target the skill's name wherever it lives.
- A name already present in the destination or in any other scanned source is a double-presence error — resolve by hand before adopting.
- Adopting an already-adopted skill moves nothing but re-ensures the links, so a partially failed run heals on the next `adopt`. The outcome line reads `adopt: "git-helper" is already adopted` followed by `  → ~/Developer/projects/agent-skills/skills/git-helper`.
- To undo, move the directory back into the store by hand; see [undo](undo.md#undo-an-adoption).

## fleet skill add-dir

```sh
fleet skill add-dir <path>
```

Registers an existing collection dir — a directory whose immediate children are skill dirs, each holding a `SKILL.md` — appending it to the `skillsDirs` list and linking every skill it holds into every installed harness:

```sh
$ fleet skill add-dir ~/Developer/projects/agent-skills/skills
add-dir: added "~/Developer/projects/agent-skills/skills"
add-dir: opencode: linked "my-notes" → ~/Developer/projects/agent-skills/skills/my-notes
add-dir: pi: linked "my-notes" → ~/Developer/projects/agent-skills/skills/my-notes
add-dir: codex: linked "my-notes" → ~/Developer/projects/agent-skills/skills/my-notes
```

- `~` expands to the home directory and a relative path resolves against the working directory; the cleaned absolute path is stored, so re-running on the same directory prints `add-dir: "<path>" is already tracked` and changes nothing. A new dir is appended once, at the end — the list is never reordered.
- The path must exist, be a directory, and hold at least one skill. It refuses the canonical store (`~/.agents/skills`), the fleet-home fallback (`~/.config/fleet/skills`), any path inside fleet home, a path already tracked, a path nested inside or containing a tracked dir, and a skill name that collides with another tracked dir or the fallback — naming the collision and its existing home.
- A name collision with the canonical store is allowed: custom outranks canonical, and `fleet skill doctor` reports the shadow. Fleet cannot rename an installed skill, so the shadow is deliberate rather than a refusal.
- After the append every skill in the dir is linked into every installed harness (one managed link per skill per harness), then sync runs. On a terminal the `add-dir:` prefix is dim, the verb green, harness names cyan.

## fleet skill remove-dir

```sh
fleet skill remove-dir <path>
```

Unregisters a collection dir from the `skillsDirs` list and removes its managed links — the inverse of `add-dir`, not of `adopt`. Fleet never deletes the directory or any file in it:

```sh
$ fleet skill remove-dir ~/Developer/projects/agent-skills/skills
remove-dir: removed "~/Developer/projects/agent-skills/skills"
remove-dir: opencode: unlinked "my-notes" → ~/Developer/projects/agent-skills/skills/my-notes
remove-dir: pi: unlinked "my-notes" → ~/Developer/projects/agent-skills/skills/my-notes
remove-dir: codex: unlinked "my-notes" → ~/Developer/projects/agent-skills/skills/my-notes
```

- `~` expands and a relative path resolves exactly like `add-dir`. The path is unlisted from `skillsDirs` — order of the rest preserved — with the disk untouched.
- A tracked path that is missing from disk still unlists cleanly, so a hand-deleted directory cannot wedge the config.
- An untracked path fails listing the tracked dirs (`remove-dir: "<path>" is not tracked: tracked dirs:` then one `- <path>` line each, or `no dirs are tracked`), so a typo never silently succeeds.
- After the unlink every managed link resolving under the dir is removed from all six harnesses, then sync runs. The headline (`remove-dir: removed "<path>"`) prints, then one line per unlinked managed link; on a terminal the `remove-dir:` prefix is dim, the verb green, harness names cyan.
- There is no bare mode: `remove-dir` always takes the path, since it addresses a directory by path only, never by a derived name.

## fleet skill doctor

```sh
fleet skill doctor
```

The read-only report of what's wrong. It inspects every installed harness, the canonical store, and every tracked custom home, and reports:

- **redundant links** — per-agent symlinks into the canonical store in harnesses that scan it natively; sync removes them on the next command
- **broken symlinks** — targets missing or looping; `fleet skill doctor -i` offers to remove them
- **unknown entries** — anything else in a skills dir (excluding managed custom-skill links into a tracked collection or the fallback); reported, never touched
- **manual edits fleet can't manage** — pattern or blanket rules that disable a skill
- **state drift** — the state disagreeing with a harness in a way sync will resolve: a config disable the state doesn't record (or vice versa), or, for a custom skill on a native-scanning harness (OpenCode, Codex, Pi, Cursor, Bob), a missing managed link while the state leaves it enabled (sync links it), or a link that outlived the disable (sync removes it). Drift is grouped by harness and direction: the cause and its fix print once, then every skill name follows, and a home-directory prefix in the directory annotation is shown as `~`
- **stale config rules** — a fleet-owned disable rule in a harness config for a skill that is installed nowhere; reported one line per harness with its skill names, capped with a `… (+N more)` summary when a harness has many, pointing at `fleet skill prune`, which removes them
- **stale state entries** — a state disable for a skill installed nowhere, kept as dormant intent; reported the same way, with the warning that pruning the entry loses the disable-on-reinstall behavior
- **incomplete scan** — a skill home is missing or unreadable, so fleet cannot trust "installed nowhere"; stale findings are suppressed and the home that blocked the scan is named
- **double presence** — a skill name that exists in more than one scanned source (canonical store, a tracked collection dir, the fallback), so a native-scanning harness would see it twice and one copy's rules may shadow the other; remove one of the copies by hand. Grouped by harness and by the pair of homes, like drift: the copies and the manual resolution print once, then every skill name, and a home-directory prefix is shown as `~`
- **unscanned adopt target** — the configured adopt target points outside the scanned homes, so adopted skills would not appear in `ls`; point it at a tracked collection or the fallback
- **tracked dir problems** — a `skillsDirs` entry that is missing from disk, is not a directory, or holds no skills, so the tracked set is incomplete or empty; restore the dir, add a skill, or run `fleet skill remove-dir <path>`
- **duplicate or nested tracked dirs** — a hand-edited `skillsDirs` list with the same dir twice or one dir nested inside another, which makes precedence ambiguous; fix the list by hand, since the verbs never create either
- **stale lockfile entries** — the skills CLI's lockfile still carries the install entry of a skill that was adopted into a custom home, so the skills CLI keeps trying to update a skill that moved; remove the entry by hand — fleet reads the lockfile and never writes it
- **missing directories** and **unreadable configs**

Findings that share an explanation are grouped so it prints once and only the names vary: redundant links, broken symlinks, unknown entries, and manual edits print one line per harness and cause, then the skill names under it; stale lockfile entries print one line per custom home; state drift groups by harness and direction; double presence groups by harness and by the pair of homes.

```sh
$ fleet skill doctor
⚠ redundant links (1) · sync removes them
  opencode  scans the canonical store natively — this link double-covers the skill
            tdd
◦ unknown entries (1) · reported, never touched
  codex  a real directory, not a symlink — left alone
         notes
⚠ manual edit conflicts (2) · left as is
  HARNESS  SKILL         DISAGREEMENT
  pi       git-helper    config off · state on
  pi       pdf-tools     config on · state off
  k keep my change · r restore — run `fleet skill doctor -i` to pick per skill
⚠ state drift (1)
  bob  enabled but not linked in ~/.bob/skills — sync links on the next command
       create-plan
⚠ double presence (1)
  opencode  duplicated in ~/.agents/skills and ~/customs/skills — remove one copy by hand
            create-plan
⚠ stale lockfile entries (1) · fleet never writes the lockfile — remove each entry by hand; the skills CLI keeps updating these moved skills
  the explicit repo  ~/customs/skills
                     git-helper
⚠ stale config rules (1) · run `fleet skill prune` to remove
  pi  ui-ux-pro-max
⚠ stale state entries (1) · run `fleet skill prune` to remove; pruning loses the disable-on-reinstall behavior
  pi  ui-ux-pro-max

1 redundant link, 1 unknown entry, 1 drift finding, 1 double-presence finding, 1 stale lockfile entry, 1 stale config rule, 1 stale state entry, 2 manual edits to resolve, run `fleet skill doctor -i` to resolve
```

Sections are marked by severity: ✖ red for breakage (broken symlinks, unreadable configs), ⚠ yellow for anything sync or you should act on, ◦ dim cyan for informational. Color only renders on a terminal; piped output is plain.

Manual-edit **conflicts** — when a config and the state file disagree about a skill — are listed in a table and left as is by default. To resolve them, pass `--interactive` (`-i`): doctor walks each conflict as a prompt:

```sh
$ fleet skill doctor -i
⚠ pi: "deploy-to-vercel" is disabled in the pi config, but fleet's state has it enabled
  [k] keep my change — record the disable in fleet's state
  [r] restore — sync fleet's state back into the config
  [s] skip — leave it as is
  [a] keep all 12 pi conflicts
  [x] skip all 12 pi conflicts
```

`keep` adopts your hand edit as the new intent; `restore` re-projects the recorded intent. When one harness has several conflicts, `[a]` and `[x]` apply the choice to all of that harness's conflicts at once — and never beyond it: the next harness's conflicts are prompted separately, so one keypress never adopts edits from a config you haven't been shown. There is no restore-all because that is exactly what `fleet skill sync` does. In interactive mode with piped input, conflicts are reported and left as is (`left as is (no input)`), and the command still exits 0.

Broken symlinks are handled the same way in interactive mode: each broken link is offered as `[r] remove` or `[s] skip`, with `[a] remove all` and `[x] skip all` per harness. A clean home reports `no problems found`.

Doctor never runs ambient sync — the point is to show what sync _would_ do before it does it.

## fleet skill prune

```sh
fleet skill prune [--yes] [--harness <harness>]... [--config-only | --state-only]
```

Removes the leftovers for skills that are installed nowhere: a fleet-owned disable rule a harness config still carries, and a dormant state disable no config can act on. With no flags it lists what it would remove and changes nothing; `--yes` applies. Sync runs after a successful apply, so configs and the state file agree when it finishes. Doctor's stale sections cap long harness lists and summarize the rest with `… (+N more)`; this command is where the full list lives.

```sh
$ fleet skill prune
prune: pi: would remove "ui-ux-pro-max" (config)
prune: pi: would remove "ui-ux-pro-max" (state)
prune: re-run with --yes to remove
$ fleet skill prune --yes
prune: pi: removed "ui-ux-pro-max" (config)
prune: pi: removed "ui-ux-pro-max" (state)
```

- The outcome is one line per removal, harness first, with the axis in parentheses: `(config)` for a harness rule, `(state)` for a state entry. On a terminal the `prune:` prefix is dim, the harness cyan, the verb green, and the axis faint. A clean home prints `prune: nothing to prune`.
- `--yes` (`-y`) applies the removals. Without it prune only reports, so a destructive run never happens by accident.
- `--harness` is repeatable and limits the work to the named harnesses: `opencode`, `pi`, `codex`, `claude`, `cursor`, `bob`. Unlike the toggle verbs it does not require the harness to be installed, since a missing config is simply nothing to remove and its state entries can still be cleared.
- `--config-only` prunes only harness config rules; `--state-only` prunes only state entries. They are mutually exclusive. Use `--config-only` to clear a config while keeping the disable-on-reinstall intent, and `--state-only` to drop the intent while leaving configs for doctor's manual-edit handling.
- Prune removes only fleet's own exact disable shapes: OpenCode's exact `permission.skill` deny (V1) or three-key deny rule (V2), Pi's exact `-skills/<name>/SKILL.md` entry, Codex's simple `[[skills.config]]` block with `enabled = false`, and Claude's `skillOverrides` value of `"off"`. Pattern rules, blanket rules, extra-key shapes, and foreign values are reported as `prune: <harness>: skipped "<skill>" — <reason>` and left untouched. Cursor and Bob have no config axis.
- A config rule for a skill the state file does not track is a manual edit, not a stale leftover; prune leaves it to `fleet skill doctor -i`.
- Pruning a state entry loses the disable-on-reinstall behavior. Reinstalling that skill comes back enabled, and you set the disable again by hand.
- Prune fails closed. When a skill home is missing or unreadable the scan is incomplete, so it removes nothing and names the home that blocked it:

  ```sh
  $ fleet skill prune --yes
  prune: nothing removed — the skill scan is incomplete
  prune: ~/Developer/customs
  prune: fix the home above, then re-run; `fleet skill doctor` reports the same blocker
  ```

  A name that looks uninstalled might live in the unscanned home, so prune refuses to remove anything until the scan is complete.

- Prune is idempotent: a second run after a clean prune reports `nothing to prune`.

## fleet skill sync

```sh
fleet skill sync
```

The explicit form of the sync that runs on every fleet command: link every custom home's skills into every installed harness, converge harness configs with the state file, and clean redundant links now, as a scriptable step. Two scenarios drive it: a hand-run `skills update`, which re-creates per-agent symlinks and may resurrect enablement, and custom skills that were added with `add-dir` or hand-created without a later `adopt` (sync links them into every installed harness). A custom skill disabled on a native-scanning harness (OpenCode, Codex, Pi, Cursor, Bob) is the flip side: its managed link is that harness's only lever for a custom, so sync removes it and never recreates it. It prints one line per fix:

```sh
$ fleet skill sync
sync: opencode: removed legacy collection path "~/Developer/projects/agent-skills/skills"
sync: opencode: removed redundant link "tdd" — opencode scans the canonical store natively — this link double-covers the skill
sync: opencode: linked "my-notes" → ~/Developer/customs/skills/my-notes
sync: bob: linked "my-notes" → ~/Developer/customs/skills/my-notes
sync: bob: unlinked "hidden-notes" → ~/Developer/customs/skills/hidden-notes
sync: opencode: disabled "tdd" (was on)
```

- The report goes to stdout, one line per fix, in sync's own order: legacy cleanup first, then link removals, then custom links (added and hidden), then enablement changes and flags.
- The first sync after an upgrade also removes the legacy fleet-owned entries the old model left behind: collection paths in OpenCode's and Pi's config, and fleet-shape exact-name off-entries on OpenCode, Codex, and Pi that name a current custom skill. Each prints as `sync: <harness>: removed legacy <collection path|disable entry> "<value>"`. The cleanup is scoped to current custom homes and names, so config fleet did not write is left alone, and later syncs report nothing.
- Nothing to repair is not an error: sync is idempotent, so a converged home prints nothing and exits 0.
- Unknown entries and manual edits are reported and left as is — doctor explains them, and `fleet skill doctor -i`'s conflict prompts are how a kept edit becomes state.
- The state file is never edited.

## fleet skill update

```sh
fleet skill update [skill]
```

Runs `skills update -g -y` — the skills CLI stays the update backend — then syncs, so disabled skills stay disabled and cleaned links stay clean no matter what the wrapped run re-created. With a skill name, only that skill is updated (`skills update -g -y <skill>`).

On success it reports from fleet's own post-run state — `skills update -g -y` runs first, then sync — so the `sync:` lines are the auto-sync repairing what the wrapped run re-created:

```sh
$ fleet skill update
sync: opencode: removed redundant link "tdd" — opencode scans the canonical store natively
1 skill in ~/.agents/skills (1 installed, 0 custom)
update: no skills updated
update: verified disabled: "tdd" for opencode, pi, claude
```

The census line (`1 skill in …`) is dim context; the headline is the green outcome — `update: no skills updated` when nothing changed, otherwise `update: updated 2 skills: tdd, foo` (`update:` dim, verb green). The `update: verified disabled:` line re-reads each harness's config after sync and confirms the recorded disables still hold (harness list cyan). If one didn't survive the update, the line reads `update: verified: "<skill>" for <harness> did not stay disabled` instead (`verified:` red). A disable for a skill installed nowhere is skipped: there is no installed skill to verify, so it is neither counted as held nor reported as lost. Like `skill: opencode: enabled` the verb carries the weight so a no-op is not mistaken for silence after `sync:` noise.

On failure the skills CLI's captured output is shown raw and the command stops — a half-finished update is yours to resolve before anything else runs:

```sh
$ fleet skill update
skills: network unreachable
Error: skills update -g -y: exit status 1
```

The wrapped call is fully explicit and non-interactive: stdin is piped closed, so an unexpected prompt fails fast instead of hanging. Fleet never parses the CLI's prose, never writes the skills CLI lockfile, and `skills check` (an undocumented alias of `update`) is not used.

## fleet config

```sh
fleet config get <key>
fleet config set <key> <value>
fleet config unset <key>
fleet config list [--json]
```

Manages fleet's machine-local config at `~/.config/fleet/config.json` (`FLEET_HOME`-aware). The file holds the tracked dirs (the `skillsDirs` list, managed by `fleet skill add-dir` and `fleet skill remove-dir`) and the adopt-target collection dir. One settable key: `adopt-target` (alias `adoptTarget`), an absolute path to the collection dir where `fleet skill adopt` lands; unset means the fleet-home fallback. The tracked dir list is shown by `list` and managed by `add-dir`/`remove-dir`, never by `set`.

```sh
$ fleet skill add-dir ~/Developer/projects/agent-skills/skills
$ fleet config set adopt-target ~/Developer/projects/agent-skills/skills
$ fleet config get adopt-target
/home/you/Developer/projects/agent-skills/skills
$ fleet config list
skills-dirs = /home/you/Developer/projects/agent-skills/skills
skills-dirs = /home/you/Developer/team-customs/skills
adopt-target = /home/you/Developer/projects/agent-skills/skills
$ fleet config list --json
{
  "skillsDirs": [
    "/home/you/Developer/projects/agent-skills/skills",
    "/home/you/Developer/team-customs/skills"
  ],
  "adoptTarget": "/home/you/Developer/projects/agent-skills/skills"
}
$ fleet config unset adopt-target
```

- `get <key>` prints the value, or empty when unset. Exit 0 even when unset; unknown keys fail with `unknown config key "foo" (want adopt-target)`. The retired keys (`skills-repo`/`skillsRepo`, `skills-repos`/`skillsRepos`) fail with a hint toward the `skillsDirs` list and `adopt-target`.
- `set <key> <value>` validates the path is absolute (`~/` expands against the user's home); the directory is created on demand by adopt, so `set` does not require it to exist. Writes atomically (`temp + rename`), preserving unknown fields.
- `unset <key>` clears the target and writes atomically (unknown fields stay).
- `list` prints one `skills-dirs = <path>` line per tracked dir in precedence order plus `adopt-target = <path>` when set (nothing when neither is set); `list --json` emits `{"skillsDirs": [...], "adoptTarget": "…"}` with absent keys omitted, two-space indent.

The file `~/.config/fleet/config.json` sits beside `state.json` and `tree-cache.json` and is `FLEET_HOME`-aware; a missing file means nothing is set, not an error. Custom skills without any tracked dir live in `~/.config/fleet/skills/` — the unversioned fleet-home fallback — and `fleet skill ls` / `adopt` target that home until a dir is added or a target is set. The retired `skillsRepos` key (and the older `skillsRepo`), if still present from a pre-public checkout, loads as a preserved unknown field and is never read — the path-tracked ADR at the repo root (`docs/adr/0007-path-tracked-custom-dirs.md`) records the model and the manual migration.

## fleet harness ls

```sh
fleet harness ls [--json]
```

Lists the six harnesses fleet supports, marking each installed (its config directory exists, the same probe every command uses) or not, with the directory itself and whether fleet can write a per-skill off switch into its config. Cursor and Bob have no config off switch; for a custom skill they disable by removing its managed link instead, the same link lever OpenCode, Codex, and Pi use for customs. A canonical-store skill keeps each harness's own config lever where one exists.

Read-only: unlike most fleet commands, no sync runs — listing what is installed must not converge anything. Nothing is filtered: you see all six even when none is installed.

```sh
$ fleet harness ls
NAME      INSTALLED  CONFIG DIR                      OFF SWITCH
opencode  ✓          /home/you/.config/opencode      yes
pi        ✓          /home/you/.pi                   yes
codex     ✓          /home/you/.codex                yes
claude    -          /home/you/.claude               yes
cursor    ✓          /home/you/.cursor               no
bob       ✓          /home/you/.bob                  no
```

- `INSTALLED` is `✓` when the harness's config directory exists, `-` when it does not. A `-` harness is skipped by every other command.
- `CONFIG DIR` is the probed directory itself.
- `OFF SWITCH` is `yes` when fleet can write a per-skill disable into that harness's config, `no` where no config lever exists. It describes canonical-store skills. A custom skill has a second lever on OpenCode, Codex, Pi, Cursor, and Bob: its managed link.
- On a terminal a one-line summary prints above the table (`fleet · 5 of 6 harnesses installed`). Piped output skips it.

### --json

```sh
$ fleet harness ls --json
```

```json
{
  "harnesses": [
    {
      "name": "opencode",
      "installed": true,
      "configDir": "/home/you/.config/opencode",
      "canDisable": true
    }
  ]
}
```

- Rows appear in fleet's fixed harness order: opencode, pi, codex, claude, cursor, bob.
- `canDisable` is false exactly for cursor and bob: no config lever. Both still toggle a custom skill through its managed link.

## fleet completion

```sh
fleet completion bash|zsh|fish|powershell
```

Prints the shell completion script. Source it, or drop it in your shell's completion directory:

```sh
source <(fleet completion zsh)
```

## fleet --version

```sh
$ fleet --version
fleet version 31d13e4
```

Release builds stamp the version at build time; `go install` builds report `dev`.

## Sync

Sync runs on every fleet command and after every wrapped `skills` call, and [`fleet skill sync`](#fleet-skill-sync) runs the same machinery on demand. Its decision rules are short and [documented in full](undo.md#how-sync-decides-what-to-touch):

1. Load the state file fresh.
2. Clean up legacy entries: the first sync after an upgrade removes the fleet-owned collection paths and custom off-entries a previous fleet release wrote, scoped to current custom homes and names. Later syncs find nothing.
3. Remove redundant links: symlinks that provably resolve into the canonical store in harnesses that scan it natively (`nativeScanHarnesses`: opencode, pi, codex, cursor, bob — never claude code). Links into any tracked collection or the fleet-home fallback are never redundant and are never removed; broken links, real dirs/files, and foreign links stay.
4. Make customs visible: link every skill in every scanned custom home (each tracked collection and the fleet-home fallback; a configured adopt target counts when it is one of those) into every installed harness, one managed link per skill. Idempotent: an already-visible home reports nothing.
5. Hide disabled customs: on a native-scanning harness (OpenCode, Codex, Pi, Cursor, Bob), remove the managed link of a custom the state disables, since there the link is the lever. A custom's disable never becomes a config off-entry.
6. Project the disables: for each installed harness with a config write side (OpenCode, Pi, Codex, Claude Code), write the harness's own off-entry for each canonical-store disable the state records whose skill is installed somewhere. A disable for a skill installed nowhere is dormant: sync keeps the name in the state but creates no new config entry, and leaves any rule already there for `fleet skill prune` to remove. Claude Code writes `skillOverrides` for custom and canonical skills alike. Nothing else — an absent entry means on, and sync never writes "on" markers. When the scan is incomplete sync projects every disable anyway, since leaving an installed skill enabled is worse than a stale rule.
7. Flag (never touch) entries it doesn't recognize: patterns, blankets, foreign shapes.
8. Never edit the state file.

`fleet skill doctor` previews all of it without changing anything.

## Error reference

| Situation                                | Message                                                                                                                               | Exit |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- | ---- |
| Unknown `--harness` value                | `unknown harness "emacs" (want one of: opencode, pi, codex, claude, cursor, bob)`                                                     | 1    |
| `--harness` names an uninstalled harness | `opencode is not installed on this machine`                                                                                           | 1    |
| `off` for a skill in no home             | `skill "typo-skill" not found in ~/.agents/skills or a custom home`                                                                   | 1    |
| `adopt` for a missing skill              | `skill "git-helper" not found in ~/.agents/skills`                                                                                    | 1    |
| `adopt` double presence                  | `skill "x" exists in … — resolve by hand before adopting` (names every copy)                                                          | 1    |
| `adopt` ambiguous, no terminal           | `adopt: multiple destinations available — re-run with --into <skills-dir> or from a terminal:` + numbered candidates                  | 1    |
| `add-dir` missing path                   | `add-dir: "<path>" does not exist`                                                                                                    | 1    |
| `add-dir` not a directory                | `add-dir: "<path>" is not a directory`                                                                                                | 1    |
| `add-dir` empty dir                      | `add-dir: "<path>" holds no skills — a collection dir's immediate children must each hold a SKILL.md`                                 | 1    |
| `add-dir` canonical store or fallback    | `add-dir: "<path>" is the canonical store — installed skills are not custom` (or `is the fleet-home fallback — it is always tracked`) | 1    |
| `add-dir` inside fleet home              | `add-dir: "<path>" is inside fleet home — fleet's config dir cannot be a collection`                                                  | 1    |
| `add-dir` nested                         | `add-dir: "<path>" is inside tracked dir "<dir>"` (or `contains tracked dir "<dir>"`)                                                 | 1    |
| `add-dir` name collision                 | `add-dir: "<path>" collides with already tracked skills: <name> (in <home>), …`                                                       | 1    |
| `remove-dir` untracked                   | `remove-dir: "<path>" is not tracked: tracked dirs:` + one `- <path>` line each (or `no dirs are tracked`)                            | 1    |
| Unknown config key                       | `unknown config key "foo" (want adopt-target)`                                                                                        | 1    |
| Retired config key                       | `unknown config key "skills-repos" (the repo-root list is retired; tracked dirs live in the `skillsDirs` list …)`                     | 1    |
| `config set` with non-absolute path      | `path must be absolute: "relative/path"`                                                                                              | 1    |
| `prune` incomplete scan                  | `prune: nothing removed — the skill scan is incomplete` + the blocking home + a re-run hint                                           | 0    |
| `prune` with both axis flags             | `if any flags in the group [config-only state-only] are set none of the others can be`                                                | 1    |
| Wrapped `skills` failure                 | CLI's raw output, then `Error: skills update -g -y: …`                                                                                | 1    |

There is no "run inside the repo" requirement and no walk-up to `.git`: customs reach fleet through `fleet skill add-dir` (register an existing collection dir) and leave it through `fleet skill remove-dir` or the adopt destination (`--into` for one run, `adopt-target` for the default, prompt otherwise). When nothing is tracked, `adopt` lands in `~/.config/fleet/skills/` (created on demand) and `ls` shows canonical plus fleet-home customs alone.
