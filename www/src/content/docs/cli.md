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

Cells read `●` on, `○` off, `-` absent. Columns for Cursor and Bob render faint with a `!` in the header: they have no per-skill off switch, so toggles there are no-ops.

`u` is the one-key update-all: it runs the same wrapped `skills update -g -y` the [update verb](#fleet-skill-update) runs, then sync, then reloads the matrix so the new badges and states show. `U` does the same for the selected skill only (`skills update -g -y <skill>`). A busy line takes over while either runs, the outcome notice reports fleet's own post-run state — the store scan and lockfile, never the skills CLI's prose — and a failed run shows the CLI's captured output raw.

The hero banner appears on launch only. `fleet --quiet` (or `-q`) keeps the matrix without it. With piped output fleet never enters the TUI: it prints the same listing as `fleet skill ls` instead, so `fleet | grep tdd` does what you mean.

## fleet skill ls

```sh
fleet skill ls [--json] [--quiet]
```

Lists every skill fleet discovers — the canonical store (`~/.agents/skills`) plus custom skills from the tracked set (explicit repos in list order, auto-tracked fleet-home checkouts alphabetically, and the unversioned fleet-home fallback `~/.config/fleet/skills`) — with a column per installed harness. A name present in more than one source appears once with precedence explicit-list order, then fleet-home checkouts alphabetically, then the fallback, then the canonical store; the other copies are doctor drift, not a silent overwrite. Custom skills come first, then installed skills grouped by source repo.

```sh
$ fleet skill ls
NAME        OPENCODE  PI  CODEX  CLAUDE  CURSOR  BOB  UPDATE  SOURCE       DESCRIPTION
git-helper  on        on  on     -       on      on   —       custom       Hand-written commit-message helper.
tdd         on        on  on     -       on      on   ↑       example/tdd  Test-driven development discipline.
```

- The enablement columns sit right after the name — they are the table's point. The description goes last, truncated to whatever room the terminal has left, so no column ever wraps mid-word; piped output keeps the plain 60-column cap.
- `SOURCE` is `custom` (the skill lives in a tracked collection or the fleet-home fallback, or has no lockfile entry) or the source repo the lockfile records.
- `UPDATE` is the outdated badge: `↑` update available, `✓` current, `?` unknown, `—` never checked (custom skills). Non-GitHub sources and failed checks are `?` — fleet checks GitHub directly, one API call per source repo, cached for an hour under the fleet config dir, and reports unknown rather than guessing.
- A harness column shows `on`, `off`, or `-`. `-` means the harness cannot discover the skill at all; for Claude Code that is the normal state until a link exists (see [Claude Code](harnesses.md#claude-code)).
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

Disables a skill: records the toggle in the state file, then sync projects it into each harness's native config. The skill's files stay in the canonical store, so `skills update` keeps updating it.

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
- Cursor and Bob have no per-skill disable mechanism. Toggling for them prints the no-op line and records nothing.
- A harness where something else overrode the write (a foreign config entry that keeps the skill enabled) is left out of the outcome list; its flag line prints instead — `sync: codex/tdd: a skills.config entry fleet doesn't manage overrides fleet's disable — left alone`.
- Sync runs as part of the command and reports real repairs it made beyond the toggle: redundant-link removals and drift fixes print as `sync:` lines. Ambient findings about other skills (untracked config disables, foreign rules) stay out of the report; `fleet skill sync` and `fleet skill doctor` are where they are listed.
- The name must exist in the canonical store. Disabling a typo would silently record state, so it fails loudly:

  ```sh
  $ fleet skill off typo-skill
  Error: skill "typo-skill" not found in ~/.agents/skills
  ```

- The command is idempotent: disabling an already-disabled skill changes nothing, and the outcome says so — `skill: opencode: "tdd" is already disabled` (one line per harness, `already` dim, verb green).

## fleet skill on

```sh
fleet skill on <name> [--harness <harness>]...
```

Re-enables a skill by removing fleet's disable entries. Same targeting rules as `off`.

```sh
$ fleet skill on tdd --harness codex
skill: codex: enabled "tdd"
```

- The outcome is one line per harness (`skill: codex: enabled "tdd"`), and reads `skill: codex: "tdd" is already enabled` (`already` dim, verb green) when nothing had to change. A harness where a foreign rule still disables the skill stays out of the list; its flag line explains (`sync: pi/tdd: still excluded by a !glob entry — left alone`).

`on` is deliberately more lenient than `off`: it also cleans up entries for skills that were uninstalled while disabled, so stale state disappears instead of accumulating.

## fleet skill adopt

```sh
fleet skill adopt <name> [--into <skills-dir>]
```

Moves a custom skill from the canonical store (`~/.agents/skills`) into the adopt destination, where it stays versioned. The destination resolves as: `--into <skills-dir>` for this run, else the configured adopt target (`fleet config set adopt-target <skills-dir>`), else a numbered choice over the tracked collections plus the always-offered fleet-home fallback (`~/.config/fleet/skills`). With no tracked collections the fallback wins with no prompt. Then fleet wires the destination in and links the skill everywhere it is needed:

```sh
$ fleet skill adopt git-helper
adopt: adopted "git-helper"
  from ~/.agents/skills/git-helper
    → ~/.config/fleet/repos/my-customs/skills/git-helper     # or ~/.config/fleet/skills/git-helper with no target set
adopt: opencode: wired "~/.config/fleet/repos/my-customs/skills" as a skill source (skills)
adopt: pi: wired "~/.config/fleet/repos/my-customs/skills" as a skill source (skills)
adopt: codex: linked "git-helper" → ~/.config/fleet/repos/my-customs/skills/git-helper
adopt: claude: linked "git-helper" → ~/.config/fleet/repos/my-customs/skills/git-helper
adopt: cursor: linked "git-helper" → ~/.config/fleet/repos/my-customs/skills/git-helper
adopt: bob: linked "git-helper" → ~/.config/fleet/repos/my-customs/skills/git-helper
```

- The first lines are the outcome: the skill now lives in the destination. On a terminal the `adopt:` prefix is dimmed, the verb is green and harness names are cyan; it is the lines to read. The headline breaks into three lines so both the original store path and the destination are visible. Managed links that were already correct stay quiet — only the wiring and the links that actually changed print, with `adopt: codex: repointed "git-helper" (was ~/.agents/skills/git-helper) → ~/.config/fleet/repos/my-customs/skills/git-helper` for a skills CLI link taken over, or a `— left alone` note when a real directory is in the way.
- Sync runs as part of the command, but ambient findings about other skills stay out of the report; `fleet skill sync` and `fleet skill doctor` are where they are listed.

- `--into` takes a collection dir (not a repo root): it is created on demand, wins with no prompt, and is never saved. A choice picked from the prompt offers a yes/no follow-up (default No) to save it as the adopt target, so persisting is deliberate. Without a terminal, an ambiguous adopt fails listing the numbered candidates and the `--into` hint instead of blocking on stdin — automation never hangs.
- The prompt always includes the fleet-home fallback alongside the tracked collections, so even a single tracked repo is an explicit choice against the default. The configured target may point somewhere unscanned — adopt still proceeds, and doctor surfaces an `unscanned adopt target` warning so the footgun is visible.
- OpenCode and Pi get the destination's path in their own config (in the dialect the file already speaks). Codex, Claude Code, Cursor, and Bob get a symlink named after the skill, pointing at the destination. Managed links never point into the canonical store, so OpenCode and Pi never see an adopted skill twice.
- Adoption records nothing in the state file: custom is defined by living in a tracked collection or the fallback. Disables recorded before adoption keep applying, because config rules target the skill's name wherever it lives.
- A name already present in the destination or in any other scanned source is a double-presence error — resolve by hand before adopting.
- Adopting an already-adopted skill moves nothing but re-ensures the wiring and links, so a partially failed run heals on the next `adopt`. The outcome line reads `adopt: "git-helper" is already adopted` followed by `  → ~/.config/fleet/repos/my-customs/skills/git-helper`.
- To undo, move the directory back into the store by hand; see [undo](undo.md#undo-an-adoption).

## fleet skill pull

```sh
fleet skill pull [<git-url> [path]] [--force]
```

Clones a customs repo and registers it, or fast-forwards every tracked repo. Fresh-machine setup is one command with no config edit:

```sh
$ fleet skill pull git@github.com:me/my-customs.git
pull: cloned "~/.config/fleet/repos/my-customs"
$ fleet skill pull
pull: current "~/.config/fleet/repos/my-customs"
```

- With no path, the checkout lands in the auto-tracked fleet-home slot derived from the URL (final path or colon segment, trailing slashes and `.git` stripped) with no config write. An explicit path inside fleet home stays auto-tracked with no config write; an explicit path outside fleet home is appended once to the explicit repo list (no duplicates, no reordering). Re-pulling an existing path never reorders the list.
- An existing checkout with the same remote fast-forwards only (`git pull --ff-only`): dirty or diverged trees fail with their state surfaced — fleet never stashes, merges, rebases, or resets. A checkout pointing at a different remote fails unless `--force` is given, so a checkout is never silently repointed. A missing git binary fails cleanly.
- A fresh clone prints `pull: cloned "<path>"`. Bare pull fast-forwards every tracked repo and prints one line per repo: `pull: updated "<path>"`, `pull: current "<path>"`, `pull: skipped "<path>" — …`, or `pull: failed "<path>" — …` — so one uncloned path never hides behind an aggregate. Non-git explicit entries are skipped with a warning instead of failing the run; with nothing tracked at all, bare pull prints `pull: no tracked repos`.
- After clone or pull the collection dir (`<repo>/skills`) is ensured (created with a warning on the empty-repo first run), the home is wired into the config-path harnesses and linked for the link-based harnesses, and sync runs so pulled customs are discoverable immediately.
- Fleet shells out to the system git with your normal auth (SSH agent, credential helper) and a full clone, so private repos just work with no fleet-side credential flags.

## fleet skill drop

```sh
fleet skill drop <path-or-name> [--force]
```

Removes a versioned customs home from the tracked set — the inverse of `pull`, not of `adopt`. The single arg is a repo-root path or a fleet-home slot name, resolved against the tracked repos; there is no bare mode and no prompt, since this verb deletes:

```sh
$ fleet skill drop my-customs
drop: removed "~/.config/fleet/repos/my-customs"
drop: opencode: unwired "~/.config/fleet/repos/my-customs/skills" as a skill source (skills)
drop: pi: unwired "~/.config/fleet/repos/my-customs/skills" as a skill source (skills)
drop: codex: unlinked "my-notes" → ~/.config/fleet/repos/my-customs/skills/my-notes
```

- An explicit repo (outside fleet home) is unlisted from the `skillsRepos` list in `config.json` — order of the rest preserved — with the disk untouched. A fleet-home checkout (`~/.config/fleet/repos/<name>`) is deleted from disk; presence is tracked, so deletion is the untrack, and the config file is never rewritten for it.
- A dirty working tree fails with its state surfaced (`git status --porcelain` output) — fleet never stashes — unless `--force` (`-f`) is given. A missing git binary skips the dirty check with a `warning: git not found in PATH: skipped dirty check` on stderr instead of failing.
- An adopt target pointing inside the dropped repo always fails with a re-point hint (`fleet config set adopt-target <skills-dir>`) — no auto-clear, even with `--force`.
- An unknown target fails listing the tracked repos, so a typo never drops the wrong home. A repo tracked only via the `FLEET_REPO` env override fails with an unset hint instead — there is no config entry to remove and no checkout fleet may delete.
- Dropping an explicit repo whose directory was already hand-deleted still succeeds: the entry is unlisted and there is nothing on disk left to check.
- After the drop the collection dir (`<repo>/skills`) is unwired from the config-path harnesses (OpenCode, Pi) and its managed links are unlinked (Codex, Claude Code, Cursor, Bob — only symlinks resolving under the dropped collection), then sync runs. After any sync report, the headline (`drop: removed "<path>"`) prints, then one line per unwired harness and per unlinked managed link; on a terminal the `drop:` prefix is dim, the verb green, harness names cyan.

## fleet skill doctor

```sh
fleet skill doctor
```

The read-only report of what's wrong. It inspects every installed harness, the canonical store, and every tracked custom home, and reports:

- **redundant links** — per-agent symlinks into the canonical store in harnesses that scan it natively; sync removes them on the next command
- **broken symlinks** — targets missing or looping; `fleet skill doctor -i` offers to remove them
- **unknown entries** — anything else in a skills dir (excluding managed custom-skill links into a tracked collection or the fallback); reported, never touched
- **manual edits fleet can't manage** — pattern or blanket rules that disable a skill
- **state drift** — state and config disagreeing in ways sync will resolve
- **double presence** — a skill name that exists in more than one scanned source (canonical store, explicit repos, fleet-home checkouts, fallback), so OpenCode and Pi would see it twice and one copy's rules may shadow the other; remove one of the copies by hand
- **unscanned adopt target** — the configured adopt target points outside the scanned homes, so adopted skills would not appear in `ls`; point it at a tracked collection or the fallback
- **non-git explicit repos** — an explicit list entry with no `.git`, so bare pull skips it; clone the repo there or remove the path from the list by hand
- **stale lockfile entries** — the skills CLI's lockfile still carries the install entry of a skill that was adopted into a custom home, so the skills CLI keeps trying to update a skill that moved; remove the entry by hand — fleet reads the lockfile and never writes it
- **missing directories** and **unreadable configs**

```sh
$ fleet skill doctor
◦ unknown entries (1) · reported, never touched
  codex  ".system" — a real directory, not a symlink — left alone
⚠ redundant links (1) · sync removes them
  opencode  link "tdd" → ~/.agents/skills/tdd — opencode scans the canonical store natively — this link double-covers the skill
⚠ manual edit conflicts (2) · left as is
  HARNESS  SKILL         DISAGREEMENT
  pi       git-helper    config off · state on
  pi       pdf-tools     config on · state off
  k keep my change · r restore — run `fleet skill doctor -i` to pick per skill

1 redundant link, 2 manual edits to resolve, run `fleet skill doctor -i` to resolve
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

## fleet skill sync

```sh
fleet skill sync
```

The explicit form of the sync that runs on every fleet command: converge harness configs with the state file now, as a scriptable step. The scenario is a hand-run `skills update` — it re-creates the per-agent symlinks and may resurrect enablement; sync repairs both and prints one line per fix:

```sh
$ fleet skill sync
sync: opencode: removed redundant link "tdd" — opencode scans the canonical store natively — this link double-covers the skill
sync: opencode: disabled "tdd" (was on)
```

- The report goes to stdout, one line per fix, in sync's own order: link removals first, then enablement changes and flags.
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

The census line (`1 skill in …`) is dim context; the headline is the green outcome — `update: no skills updated` when nothing changed, otherwise `update: updated 2 skills: tdd, foo` (`update:` dim, verb green). The `update: verified disabled:` line re-reads each harness's config after sync and confirms the recorded disables still hold (harness list cyan). If one didn't survive the update, the line reads `update: verified: "<skill>" for <harness> did not stay disabled` instead (`verified:` red). Like `skill: opencode: enabled` the verb carries the weight so a no-op is not mistaken for silence after `sync:` noise.

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

Manages fleet's machine-local config at `~/.config/fleet/config.json` (`FLEET_HOME`-aware). The file holds the tracked customs repos (the explicit repo-root list, managed by `fleet skill pull`) and the adopt-target collection dir. One settable key: `adopt-target` (alias `adoptTarget`), an absolute path to the collection dir where `fleet skill adopt` lands; unset means the fleet-home fallback. The tracked repo list is shown by `list` and managed by `skill pull`, never by `set`.

```sh
$ fleet skill pull git@github.com:me/my-customs.git
$ fleet config set adopt-target ~/.config/fleet/repos/my-customs/skills
$ fleet config get adopt-target
/home/you/.config/fleet/repos/my-customs/skills
$ fleet config list
skills-repos = /home/you/Developer/team-customs
adopt-target = /home/you/.config/fleet/repos/my-customs/skills
$ fleet config list --json
{
  "adoptTarget": "/home/you/.config/fleet/repos/my-customs/skills",
  "skillsRepos": [
    "/home/you/Developer/team-customs"
  ]
}
$ fleet config unset adopt-target
```

- `get <key>` prints the value, or empty when unset. Exit 0 even when unset; unknown keys fail with `unknown config key "foo" (want adopt-target)`. The retired single-pointer keys (`skills-repo`, `skillsRepo`) fail with a hint toward `skill pull` and `adopt-target`.
- `set <key> <value>` validates the path is absolute (`~/` expands against the user's home); the directory is created on demand by adopt, so `set` does not require it to exist. Writes atomically (`temp + rename`), preserving unknown fields.
- `unset <key>` clears the target and writes atomically (unknown fields stay).
- `list` prints one `skills-repos = <path>` line per explicit repo in precedence order plus `adopt-target = <path>` when set (nothing when neither is set); `list --json` emits `{"skillsRepos": [...], "adoptTarget": "…"}` with absent keys omitted, two-space indent.

The file `~/.config/fleet/config.json` sits beside `state.json` and `tree-cache.json` and is `FLEET_HOME`-aware; a missing file means nothing is set, not an error. Custom skills without any tracked repo live in `~/.config/fleet/skills/` — the unversioned fleet-home fallback — and `fleet skill ls` / `adopt` target that home until a repo is pulled or a target is set. The old single-pointer key (`skillsRepo`), if still present from a pre-public checkout, loads as a preserved unknown and is never interpreted — the multi-repo ADR at the repo root (`docs/adr/0002-multi-repo-customs-and-adopt-target.md`) records the manual migration steps.

## fleet harness ls

```sh
fleet harness ls [--json]
```

Lists the six harnesses fleet supports, marking each installed — its config directory exists, the same probe every command uses — or not, with the directory itself and whether fleet can write a per-skill off switch into its config. Cursor and Bob have no off switch; fleet says so instead of pretending.

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
- `OFF SWITCH` is `yes` when fleet can write a per-skill disable into that harness's config, `no` where no such lever exists.
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
- `canDisable` is false exactly for cursor and bob.

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
2. Remove redundant links: symlinks that provably resolve into the canonical store in harnesses that scan it natively (`nativeScanHarnesses`: opencode, pi, codex, Cursor, Bob — never claude code). Links into any tracked collection or the fleet-home fallback are never redundant and are never removed; broken links, real dirs/files, and foreign links stay.
3. For each installed harness with a write side, write the harness's own off-entry for each state-recorded disable. Nothing else — an absent entry means on, and sync never writes "on" markers.
4. Flag (never touch) entries it doesn't recognize: patterns, blankets, foreign shapes.
5. Never edit the state file.

`fleet skill doctor` previews all of it without changing anything.

## Error reference

| Situation                                | Message                                                                                                                    | Exit |
| ---------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- | ---- |
| Unknown `--harness` value                | `unknown harness "emacs" (want one of: opencode, pi, codex, claude, cursor, bob)`                                          | 1    |
| `--harness` names an uninstalled harness | `opencode is not installed on this machine`                                                                                | 1    |
| `off` for a skill not in the store       | `skill "typo-skill" not found in ~/.agents/skills`                                                                         | 1    |
| `adopt` for a missing skill              | `skill "git-helper" not found in ~/.agents/skills`                                                                         | 1    |
| `adopt` double presence                  | `skill "x" exists in … — resolve by hand before adopting` (names every copy)                                               | 1    |
| `adopt` ambiguous, no terminal           | `adopt: multiple destinations available — re-run with --into <skills-dir> or from a terminal:` + numbered candidates       | 1    |
| `pull` onto a different remote           | `<path> points at a different remote "…" (want "…"): use --force to pull anyway`                                           | 1    |
| `pull` dirty tree                        | `<path> has uncommitted changes — resolve by hand (fleet never stashes):` + `git status` output                            | 1    |
| `pull` diverged tree                     | `pull <path>: … (resolve by hand; fleet never merges or rebases)`                                                          | 1    |
| `pull` with missing git                  | `git not found in PATH: install git to use skill pull`                                                                     | 1    |
| `drop` unknown target                    | `unknown target "…": tracked repos:` + one `- <path>` line per repo (or `unknown target "…": no skills repos are tracked`) | 1    |
| `drop` dirty tree                        | `<path> has uncommitted changes — resolve by hand (fleet never stashes):` + `git status` output (`--force` overrides)      | 1    |
| `drop` with missing git                  | `warning: git not found in PATH: skipped dirty check` on stderr, exit 0                                                    | 0    |
| `drop` adopt target inside               | `cannot drop "…" — adopt target "…" is inside it — re-point first with: fleet config set adopt-target <skills-dir>`        | 1    |
| Unknown config key                       | `unknown config key "foo" (want adopt-target)`                                                                             | 1    |
| Retired single-pointer key               | `unknown config key "skills-repo" (the single repo pointer is retired; …)`                                                 | 1    |
| `config set` with non-absolute path      | `path must be absolute: "relative/path"`                                                                                   | 1    |
| Wrapped `skills` failure                 | CLI's raw output, then `Error: skills update -g -y: …`                                                                     | 1    |

There is no "run inside the repo" requirement and no walk-up to `.git`: customs reach fleet through `skill pull` (clone into the tracked set) and leave it through the adopt destination (`--into` for one run, `adopt-target` for the default, prompt otherwise). When nothing is tracked, `adopt` lands in `~/.config/fleet/skills/` (created on demand) and `ls` shows canonical plus fleet-home customs alone.
