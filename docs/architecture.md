# Architecture

Fleet is a Go CLI + TUI with one idea at its core: a versioned state file is the single source of truth for per-harness skill enablement, and six adapters project that state into each harness's own native config. Skills never move; the canonical store stays where the `skills` CLI puts it. A second file, `~/.config/fleet/config.json` (`FLEET_HOME`-aware), holds the machine-local pointer to the versioned skills repo and never touches enablement.

```
             state file (~/.config/fleet/state.json)   config file (~/.config/fleet/config.json)
                  ▲ save                    │ read           │ read (FLEET_REPO env > config > "")
                  │                         ▼                ▼
toggle.Apply ─────┴────► sync.Run ──► harness.Adapter.Project ──► harness configs
(CLI on/off, TUI)        │
                         ├─► redundant-link removal (canonical-store links only; fleet-home/skills-repo links never removed)
                         └─► flags for entries it doesn't own

read side:  snapshot.Build ──► scan (canonical + fleet-home + skillsRepo when set, precedence skillsRepo > fleet-home > canonical)
                          ├──► scan lockfile (provenance)
                          ├──► harness.Adapter.Read (per installed harness)
                          └──► outdated.Check (GitHub, cached)
                       ▲
     fleet skill ls ───┤
     bare fleet (TUI) ─┘
```

## Package map

| Package                        | Owns                                                                                                                                                                                                                                                                                                                         |
| ------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cmd/fleet`                    | `main`: resolve paths from the environment, hand them to the CLI. Nothing else.                                                                                                                                                                                                                                              |
| `internal/paths`               | Every filesystem location, derived from one injected home root plus the resolved skills repo. `FLEET_HOME` overrides home; `FLEET_REPO` env > `~/.config/fleet/config.json: skillsRepo` > `""` with no walk-up. Exposes `FleetConfigFile()`, `FleetHomeSkills()`, `RepoSkills()`. No other package calls `os.UserHomeDir()`. |
| `internal/config`              | The machine-local config file `~/.config/fleet/config.json` (beside `state.json`): load/save, `skillsRepo` key, `EffectiveRepo` precedence `env > file > ""`, atomic write, unknown-field preservation, validation for `fleet config` verbs.                                                                                 |
| `internal/scan`                | Read-only listing of the canonical store, fleet-home, and skills-repo stores plus the skills CLI lockfile (provenance, sanitized dir names, frontmatter parsing).                                                                                                                                                            |
| `internal/state`               | The state file: versioned schema, unknown-field preservation, atomic save.                                                                                                                                                                                                                                                   |
| `internal/harness`             | The seam. The `Adapter` interface, six adapters with their read and write sides, link management and classification, skill-source wiring.                                                                                                                                                                                    |
| `internal/jsonc`               | Comment-, key-order-, and formatting-preserving JSONC editing, used by the JSON-config writers.                                                                                                                                                                                                                              |
| `internal/sync`                | State → adapters, plus redundant-link removal (canonical-store links only; fleet-home/skills-repo links never removed). Never edits the state file.                                                                                                                                                                          |
| `internal/toggle`              | The one write path: record toggles in the state file, strip "on" markers directly, then sync. Shared by the CLI verbs and the TUI's staged apply.                                                                                                                                                                            |
| `internal/doctor`              | Read-only report of drift, links, and manual edits; keep-or-restore resolution for conflicts; double presence across canonical, fleet-home, and repo homes.                                                                                                                                                                  |
| `internal/customs`             | `adopt`: move a custom skill into the resolved home (`RepoSkills()` when set else `FleetHomeSkills()`), wire that home, manage the links.                                                                                                                                                                                    |
| `internal/outdated`            | The update badge: lockfile hash vs GitHub tree hash, one call per repo, conditional requests, TTL cache. Customs and non-GitHub sources are always unknown.                                                                                                                                                                  |
| `internal/snapshot`            | The read-side picture (rows × harness columns) that `ls` and the TUI both render. Unions canonical + fleet-home + repo when set; precedence `skillsRepo > fleet-home > canonical`.                                                                                                                                           |
| `internal/skillscli`           | The wrapped `skills` process call: explicit flags, stdin piped closed, output captured for failure display, never parsed.                                                                                                                                                                                                    |
| `internal/cli`, `internal/tui` | Faces. Cobra verbs (`skill ls/on/off/adopt/update/sync/doctor`, `harness ls`, `config get/set/unset/list`) and the Bubble Tea matrix over the same core; neither keeps its own copy of any business logic.                                                                                                                   |
| `internal/buildinfo`           | The version stamp (`make build` and goreleaser set it via `-ldflags -X`).                                                                                                                                                                                                                                                    |

## The adapter interface contract

`internal/harness.Adapter` is fleet's one seam. Every harness goes through it; nothing above the seam knows a config format.

```go
type Adapter interface {
    Harness() Harness                    // opencode, pi, codex, claude, cursor, bob
    Installed() bool                     // config-dir probe
    Read(names []string) (ReadResult, error)
    CanProject() bool                    // false for Cursor and Bob
    Project(writes []SkillWrite) (WriteReport, error)
}
```

**Read side.** `Read` never writes. For every requested name it reports one of three states in `ReadResult.States`:

- `on` — the harness discovers the skill and nothing disables it
- `off` — the harness discovers it but its config disables it
- `absent` — the harness cannot discover it at all (claude code without a link)

It also reports `Linked` (names with a link in the harness's skills dir), `Disables` (every skill the config disables through an exact-name entry fleet could own, including names not in the store — doctor compares this against the state file), and, for opencode, the detected `Dialect` and configured `SkillSources`. A missing config file means everything `on` (or `absent` for claude), not an error.

**Write side.** `Project` is a read-modify-write that takes desired `SkillWrite{Name, State}` values and returns what changed (`Changed`: state flips, `From` → `To`) and what it deliberately left alone (`Flags`). The preservation rules:

- Only fleet's own exact-shape entries are ever removed or flipped. Patterns, blanket rules, glob exclusions, codex blocks with extra keys: flagged, never touched.
- Comments, unknown keys, key order, and formatting survive every write. The file is written only when content actually changed.
- `Project` may only be called when `CanProject()` is true; Cursor and Bob return an error from it by construction.
- Skill names are validated up front (empty names are a programming error, rejected loudly).

**Preservation per config format:**

| Format      | Harnesses       | How preservation works                                                                                                                                                                                                                           |
| ----------- | --------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| JSONC       | opencode        | `internal/jsonc` parses to a tree that keeps comments, key order, and raw literals; writes are tree edits (`Set`/`Append`/`Delete`); render is byte-compared with the source first.                                                              |
| Strict JSON | pi, claude code | Parsed strictly first (comments are an error, matching the harness), then edited through the same tree editor; the writer never emits comments.                                                                                                  |
| TOML        | codex           | No comment-preserving TOML encoder exists, so fleet edits at the line level: blocks it recognizes (header plus simple `name`/`path`/`enabled` lines, comments and blanks) are flipped or removed; every other line passes through byte for byte. |

Two optional interfaces extend the seam for custom skills, both targeting the resolved home (`RepoSkills()` when a repo is set, otherwise `FleetHomeSkills()`):

- `SkillLinker.LinkSkill(name, target)` — keep `<harness skills dir>/<name>` a symlink to the resolved home: created when missing, repointed when it targets something else, never touching a real directory or file. Implemented by codex, claude code, Cursor, and Bob.
- `SourceWiring.WireSkillSource(dir)` — add a directory as an extra skill-discovery source in the config shape the file already speaks. Implemented by opencode and pi.

## The write sequence for toggles

`toggle.Apply` is the only code that changes enablement, and the CLI's `on`/`off` and the TUI's staged apply both call it:

1. Load the state file, apply the toggles, save atomically.
2. "On" writes strip fleet's markers directly, before ambient sync runs — the state entry is already gone by then, so sync would otherwise flag the still-present markers as foreign.
3. "Off" writes need no direct step; sync projects them.
4. Sync runs anyway and fixes any other drift it finds.

## Sync

`sync.Run` reads the state file fresh, then: remove redundant links (symlinks provably resolving into the canonical store in harnesses that scan it natively — never claude code, never non-symlinks, never links into the fleet-home or skills-repo custom homes), then project one off-entry per recorded disable into each installed writable harness. Unrecognized entries are flagged, not touched. The state file is never edited. Every command runs this ambiently, and `fleet skill sync` exposes the same run as an explicit verb. The full decision list lives in [undo and escape hatches](undo.md#how-sync-decides-what-to-touch).

## Doctor

`doctor.Analyze` runs the same link classification and config reads as sync, but writes nothing. Findings are grouped (redundant links, broken symlinks, unknown entries, manual edits fleet can't manage, state drift, double presence across canonical / fleet-home / repo, stale lock entries for adopted skills, missing directories, unreadable configs). Manual-edit _conflicts_ — where config and state disagree and both are expressible — are reported with their keep/restore options by default; `fleet skill doctor -i` walks each one as a prompt: `keep` records the edit in the state file, `restore` re-projects the recorded intent. Everything else is reported for sync to fix or the user to handle.

## Adding a new harness

The guide assumes the harness has some per-skill disable mechanism; if it doesn't, Cursor and Bob show the pattern (read side reports `on`, `CanProject()` false, toggles are explicit no-ops).

1. **Pin down the mechanism first.** Which file, which syntax, what a disable looks like, what the harness does natively (does it scan the canonical store?). Write it down in the spec before code — every current adapter's quirks trace back to verified behavior, not guesswork.
2. **Paths.** Add accessors to `internal/paths/paths.go`: the config dir (the `Installed()` probe), the config file, and the skills dir if the harness has one.
3. **Adapter.** Create `internal/harness/<name>.go` implementing `Adapter`:
   - `Installed()` probes the config directory.
   - `Read` fills `States` for _every_ requested name (missing config = all `on`), plus `Disables` for exact-name entries fleet could own. Patterns and blankets are not `Disables` — they surface as flags.
   - `CanProject()` returns whether there is a real config lever.
   - Put the write side in `<name>_write.go`: parse, edit, `writeIfConfigChanged` (or the TOML line-level equivalent), return `Changed`/`Flags`. Only fleet's exact shapes are removable; everything else is a flag.
4. **Register.** Add the adapter to `All(p)`. The slice order is the column order in `ls`, the TUI, and every report, so place it deliberately.
5. **Custom skills.** If customs reach the harness through a skills dir, implement `SkillLinker` and add the dir to `skillDirPaths`; decide `NativeScan` in `nativeScanHarnesses` (does the harness scan the canonical store on its own? If yes, sync removes store links there). If customs arrive through a config path instead, implement `SourceWiring`. Neither, and customs simply don't reach it — say so in the docs.
6. **Tests.** Build the adapter a fake home (`paths.New(filepath.Join(t.TempDir(), "home"))`), write fixtures, run `Read`/`Project`, and assert the whole file's bytes plus the report. Add the adapter to the cross-harness test files (`disables_test.go`, `links_test.go`, `wiring_test.go`, `harness_test.go`) so shared behavior stays shared. See the [testing guide](testing.md#anatomy-of-an-adapter-test).
7. **Docs.** Add a row to the [per-harness reference](harnesses.md) overview and a section with the same shape as the others: file, written shape, never touched, limitations. `--harness` validation, `ls` columns, and the TUI pick the harness up automatically; `CONTEXT.md` changes only if you introduced new vocabulary.

Invariants that hold across the seam: adapters never call `os.UserHomeDir()` (paths come injected), `Read` never writes, `Project` is only called when `CanProject()`, files are written only when content changed, and unknown config content is flagged or preserved — never silently dropped.
