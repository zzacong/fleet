---
name: watcher
description: Watch every file fleet touches — snapshot and diff fleet state,
  canonical store, and harness configs. Use when asked to watch what
  fleet changed.
---

# Watcher

You are a watcher. The user is testing fleet — watch the files fleet
touches and report diffs on every `go`.

## Tool

`scripts/watcher/watch.ts` (`scripts/watcher/watch.ts:2-19`) — single
TypeScript file, Node stdlib only, no dependencies.

```
node scripts/watcher/watch.ts --initial --label=baseline  # record baseline
node scripts/watcher/watch.ts --label=go-1  # snapshot + diff vs previous
```

Runs directly with Node 22+ — no install step, no Makefile targets (dev
tool only, not CI).

State lives in `scripts/watcher/.state/` (gitignored at `.gitignore:17`):
`snap-*.json` snapshots + `contents/<sha1>` deduped file contents for line
diffs. Do not commit state.

## Watch targets (14, from `internal/paths/paths.go`)

Targets mirror fleet's `Paths` — every location fleet derives from an injected
home root plus the repo root. Only the config file + skills dir per harness are
watched (full harness dirs are not). Harness targets remain derived from
`internal/paths` (Go code at the repo root, watcher
stays at `scripts/watcher/watch.ts`).

| Label             | Path                                | Kind                                                                                                                                                   |
| ----------------- | ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `fleet-config`    | `~/.config/fleet`                   | dir — covers `state.json`, `config.json`, `tree-cache.json` and `skills/` (fleet-home unversioned fallback) via dir walk                               |
| `agents-store`    | `~/.agents`                         | dir (canonical store)                                                                                                                                  |
| `opencode-config` | `~/.config/opencode/opencode.jsonc` | file                                                                                                                                                   |
| `opencode-skills` | `~/.config/opencode/skills`         | dir                                                                                                                                                    |
| `pi-settings`     | `~/.pi/agent/settings.json`         | file                                                                                                                                                   |
| `pi-skills`       | `~/.pi/agent/skills`                | dir                                                                                                                                                    |
| `codex-config`    | `~/.codex/config.toml`              | file                                                                                                                                                   |
| `codex-skills`    | `~/.codex/skills`                   | dir                                                                                                                                                    |
| `claude-skills`   | `~/.claude/skills`                  | dir                                                                                                                                                    |
| `claude-config`   | `~/.claude/settings.json`           | file                                                                                                                                                   |
| `cursor-skills`   | `~/.cursor/skills`                  | dir                                                                                                                                                    |
| `bob-skills`      | `~/.bob/skills`                     | dir                                                                                                                                                    |
| `bob-settings`    | `~/.bob/settings.json`              | file                                                                                                                                                   |
| `repo-skills`     | `<repo>/skills`                     | dir — monorepo `skills/` publishable collection at the repo root (versioned collection; what skills.sh publishes; checkout's `skills/` for dogfooding) |

## Protocol

1. **Initial snapshot:** on first run or when the user says to re-baseline, run
   with `--initial`. Report the target table (`ok` / `MISSING` + entry counts)
   and note gaps fleet will likely create (e.g. `state.json`, missing skills
   dirs).
2. **Every `go`:** retake a snapshot and report diffs vs the previous snapshot.
   Do not skip the diff even when empty — report "No changes detected."
3. Keep the initial baseline in mind for context, but diff against the
   _previous_ snapshot by default. Mention the initial→current delta when the
   user asks.

## Diff markers

| Marker | Meaning                                                                                                 |
| ------ | ------------------------------------------------------------------------------------------------------- |
| `+`    | Added path (or `+ <dir created>` for an empty dir becoming present)                                     |
| `-`    | Removed path (or `- <dir removed>`)                                                                     |
| `~`    | Modified — SHA-1 differs; followed by `size/hash8 -> size/hash8` and a unified line diff when available |
| `·`    | Touched — stat differs but SHA-1 identical (fleet sync rewrote with identical bytes)                    |

Line diffs are capped at 120 lines per file. Minified blobs (any line

> 500 chars, e.g. `tree-cache.json`) are summarized as
> `(minified content — see hash change above)` — rely on the hash change,
> not the dump. Files > 4 MB are tracked by size + mtime only.

## Reporting

- Group diffs by target label (`## fleet-config (…)`).
- For `~` files, show the unified diff (`-`/`+` lines) already provided by
  `contentDiff()` — do not re-implement diffing.
- Call out fleet-relevant signals: `state.json` creates/updates, harness deny
  entries (`-skills/<name>/SKILL.md` in pi, `{"action":"skill"}` in opencode,
  `[[skills.config]]` in codex), symlink creation, and redundant-link removal.
- Note when a diff looks like a fleet sync sweep vs an external harness
  self-write (e.g. pi flipping `modelThinkingLevels`).
- End every reply with a one-sentence summary of the diff, then on a
  new line: `Say "go" for the next diff.`

## Reproducibility

State persists in `scripts/watcher/.state/` across sessions on the same
checkout (not across a fresh `git clone`). No prior chat history needed
— this file + `scripts/watcher/watch.ts` is sufficient.
