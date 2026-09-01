# Command reference

Fleet has one binary and one command family. Bare `fleet` opens the interactive matrix; everything scripted lives under `fleet skill`. Sync runs inside every command, and `fleet skill sync` is that machinery as an explicit verb (see [Sync](#sync)).

Examples were run against a sandbox home (`FLEET_HOME=$(mktemp -d)`) with two skills: `tdd`, installed from a source repo, and `git-helper`, a custom skill. Paths are shortened to `~` for readability.

## fleet

```sh
fleet
```

Opens the skill × harness matrix: one row per skill, one column per installed harness. Toggles are staged with `space` and applied together with `enter` — the same state-file-then-sync write path the `on`/`off` verbs run.

Keys:

| Key               | Action                                                      |
| ----------------- | ----------------------------------------------------------- |
| `j` / `k`, arrows | move the skill cursor                                       |
| `h` / `l`, arrows | move the harness column                                     |
| `space`           | stage or unstage the selected cell                          |
| `enter`           | apply staged changes                                        |
| `u`               | update all installed skills (wrapped skills CLI, then sync) |
| `esc`             | discard staged changes, clear the filter, or close help     |
| `/`               | filter by name or description                               |
| `r`               | refresh                                                     |
| `?`               | help overlay                                                |
| `q`, `ctrl+c`     | quit (`q` is text while the filter holds input)             |

Cells read `●` on, `○` off, `-` absent. Columns for Cursor and Bob render faint with a `!` in the header: they have no per-skill off switch, so toggles there are no-ops.

`u` is the one-key update-all: it runs the same wrapped `skills update -g -y` the [update verb](#fleet-skill-update) runs, then sync, then reloads the matrix so the new badges and states show. A busy line takes over while it runs, the outcome notice reports fleet's own post-run state — the store scan and lockfile, never the skills CLI's prose — and a failed run shows the CLI's captured output raw.

The hero banner appears on launch only. `fleet --quiet` (or `-q`) keeps the matrix without it. With piped output fleet never enters the TUI: it prints the same listing as `fleet skill ls` instead, so `fleet | grep tdd` does what you mean.

## fleet skill ls

```sh
fleet skill ls [--json] [--quiet]
```

Lists every skill in the canonical store, plus the fleet repo's custom skills, with a column per installed harness. Custom skills come first, then installed skills grouped by source repo.

```sh
$ fleet skill ls
NAME        ORIGIN     SOURCE       UPDATE  DESCRIPTION                          OPENCODE  PI  CODEX  CLAUDE  CURSOR  BOB
git-helper  custom     -            ?       Hand-written commit-message helper.  on        on  on     -       on      on
tdd         installed  example/tdd  ?       Test-driven development discipline.  on        on  on     -       on      on
```

- `ORIGIN` is `custom` or `installed`. Custom means the skill lives in the fleet repo's `skills/` directory (or has no lockfile entry); installed means the skills CLI lockfile records provenance.
- `UPDATE` is the outdated badge: `↑` update available, `✓` current, `?` unknown. Custom skills and non-GitHub sources are always `?` — fleet checks GitHub directly, one API call per source repo, cached for an hour under the fleet config dir, and reports unknown rather than guessing.
- A harness column shows `on`, `off`, or `-`. `-` means the harness cannot discover the skill at all; for claude code that is the normal state until a link exists (see [claude code](harnesses.md#claude-code)).
- On a terminal a one-line summary prints above the table (`fleet · 2 skills · 1 installed · 1 custom · …`). Piped output skips it, and `--quiet` skips it everywhere.

Sync runs first. Its reports go to **stderr**, so stdout stays machine-readable:

```sh
$ fleet skill ls > /dev/null
sync: opencode: removed redundant link "tdd" — opencode scans the canonical store natively — this link double-covers the skill
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
sync: opencode: disabled "tdd" (was on)
sync: pi: disabled "tdd" (was on)
sync: codex: disabled "tdd" (was on)
cursor: no per-skill disable mechanism — disable "tdd" is a no-op
bob: no per-skill disable mechanism — disable "tdd" is a no-op
```

- Without `--harness`, every installed harness is targeted. `--harness` is repeatable and takes a harness ID: `opencode`, `pi`, `codex`, `claude`, `cursor`, `bob`.
- Cursor and Bob have no per-skill disable mechanism. Toggling for them prints the no-op line and records nothing.
- The name must exist in the canonical store. Disabling a typo would silently record state, so it fails loudly:

  ```sh
  $ fleet skill off typo-skill
  Error: skill "typo-skill" not found in ~/.agents/skills
  ```

- The command is idempotent: disabling an already-disabled skill changes nothing and prints nothing.

## fleet skill on

```sh
fleet skill on <name> [--harness <harness>]...
```

Re-enables a skill by removing fleet's disable entries. Same targeting rules as `off`.

```sh
$ fleet skill on tdd --harness codex
sync: codex: enabled "tdd" (was off)
```

`on` is deliberately more lenient than `off`: it also cleans up entries for skills that were uninstalled while disabled, so stale state disappears instead of accumulating.

## fleet skill adopt

```sh
fleet skill adopt <name>
```

Moves a custom skill from the canonical store (`~/.agents/skills`) into the fleet repo's `skills/` directory, where it stays versioned. Then it wires the repo in and links the skill everywhere it is needed:

```sh
$ fleet skill adopt git-helper
moved ~/.agents/skills/git-helper → ~/Developer/fleet/skills/git-helper
opencode: wired "~/Developer/fleet/skills" as a skill source (skills.paths)
pi: wired "~/Developer/fleet/skills" as a skill source (skills)
codex: linked "git-helper" → ~/Developer/fleet/skills/git-helper
claude: linked "git-helper" → ~/Developer/fleet/skills/git-helper
cursor: linked "git-helper" → ~/Developer/fleet/skills/git-helper
bob: linked "git-helper" → ~/Developer/fleet/skills/git-helper
```

- The repo is found by walking up from the working directory to the nearest `.git`; `FLEET_REPO` overrides it. Outside any repo: `Error: no fleet repo found — run inside the repo or set FLEET_REPO`.
- opencode and pi get the repo path in their own config (in the dialect the file already speaks). codex, claude code, Cursor, and Bob get a symlink named after the skill, pointing at the repo. Managed links never point into the canonical store, so opencode and pi never see an adopted skill twice.
- Adoption records nothing in the state file: custom is defined by living in the repo. Disables recorded before adoption keep applying, because config rules target the skill's name wherever it lives.
- Adopting an already-adopted skill moves nothing but re-ensures the wiring and links, so a partially failed run heals on the next `adopt`.
- To undo, move the directory back into the store by hand; see [undo](undo.md#undo-an-adoption).

## fleet skill doctor

```sh
fleet skill doctor
```

The read-only report of what's wrong. It inspects every installed harness, the canonical store, and the repo's `skills/` directory, and reports:

- **redundant links** — per-agent symlinks into the canonical store in harnesses that scan it natively; sync removes them on the next command
- **broken symlinks** — targets missing or looping
- **unknown entries** — anything else in a skills dir; reported, never touched
- **manual edits fleet can't manage** — pattern or blanket rules that disable a skill
- **state drift** — state and config disagreeing in ways sync will resolve
- **double presence** — a skill name that exists in both the canonical store and the repo's `skills/` dir, so opencode and pi would see it twice and one copy's rules may shadow the other; remove one of the copies by hand
- **stale lockfile entries** — the skills CLI's lockfile still carries the install entry of a skill that was adopted into the repo, so the skills CLI keeps trying to update a skill that moved; remove the entry by hand — fleet reads the lockfile and never writes it
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

`keep` adopts your hand edit as the new intent; `restore` re-projects the recorded intent. When one harness has several conflicts, `[a]` and `[x]` apply the choice to all of that harness's conflicts at once — and never beyond it: the next harness's conflicts are prompted separately, so one keypress never adopts edits from a config you haven't been shown. There is no restore-all because that is exactly what `fleet skill sync` does. In interactive mode with piped input, conflicts are reported and left as is (`left as is (no input)`), and the command still exits 0. A clean home reports `no problems found`.

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
fleet skill update
```

Runs `skills update -g -y` — the skills CLI stays the update backend — then syncs, so disabled skills stay disabled and cleaned links stay clean no matter what the wrapped run re-created.

On success it reports from fleet's own post-run state:

```sh
$ fleet skill update
1 skill in ~/.agents/skills (1 installed, 0 custom)
disabled: "tdd" for opencode, pi, claude
```

The `disabled:` line re-reads each harness's config after sync and confirms the recorded disables hold. If one didn't survive the update, the line reads `<skill>: <harness> did not stay disabled` instead.

On failure the skills CLI's captured output is shown raw and the command stops — a half-finished update is yours to resolve before anything else runs:

```sh
$ fleet skill update
skills: network unreachable
Error: skills update -g -y: exit status 1
```

The wrapped call is fully explicit and non-interactive: stdin is piped closed, so an unexpected prompt fails fast instead of hanging. Fleet never parses the CLI's prose, never writes the skills CLI lockfile, and `skills check` (an undocumented alias of `update`) is not used.

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
2. Remove redundant links: symlinks into the canonical store in harnesses that scan it natively. Never claude's; never anything else in those dirs.
3. For each installed harness with a write side, write the harness's own off-entry for each state-recorded disable. Nothing else — an absent entry means on, and sync never writes "on" markers.
4. Flag (never touch) entries it doesn't recognize: patterns, blankets, foreign shapes.
5. Never edit the state file.

`fleet skill doctor` previews all of it without changing anything.

## Error reference

| Situation                                | Message                                                                           | Exit |
| ---------------------------------------- | --------------------------------------------------------------------------------- | ---- |
| Unknown `--harness` value                | `unknown harness "emacs" (want one of: opencode, pi, codex, claude, cursor, bob)` | 1    |
| `--harness` names an uninstalled harness | `opencode is not installed on this machine`                                       | 1    |
| `off` for a skill not in the store       | `skill "typo-skill" not found in ~/.agents/skills`                                | 1    |
| `adopt` outside a repo                   | `no fleet repo found — run inside the repo or set FLEET_REPO`                     | 1    |
| `adopt` for a missing skill              | `skill "git-helper" not found in ~/.agents/skills`                                | 1    |
| Wrapped `skills` failure                 | CLI's raw output, then `Error: skills update -g -y: …`                            | 1    |
