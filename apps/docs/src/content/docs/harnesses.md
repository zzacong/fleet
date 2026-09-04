---
title: Per-Harness Reference
description: Files touched, written shapes, limitations.
---

# Per-harness reference

Fleet targets six harnesses. Each section states exactly which files fleet touches, what it writes, what it never touches, and where the harness's own limits show up. Vocabulary follows [CONTEXT.md](../CONTEXT.md): the canonical store is `~/.agents/skills`, customs live in tracked collections (explicit repos plus auto-tracked `~/.config/fleet/repos/` checkouts) and the fleet-home fallback `~/.config/fleet/skills`, the state file is fleet's source of truth, and sync projects that state into each harness's config.

## Overview

| Harness     | Detected by          | Config fleet writes                 | Disable mechanism                                    | Custom skills reach it via         |
| ----------- | -------------------- | ----------------------------------- | ---------------------------------------------------- | ---------------------------------- |
| opencode    | `~/.config/opencode` | `~/.config/opencode/opencode.jsonc` | deny rule (`permission.skill` V1 / `permissions` V2) | skills path in the same config     |
| pi          | `~/.pi`              | `~/.pi/agent/settings.json`         | force-exclude entry in the `skills` array            | plain path entry in the same array |
| codex       | `~/.codex`           | `~/.codex/config.toml`              | `[[skills.config]]` with `enabled = false`           | symlink into `~/.codex/skills`     |
| claude code | `~/.claude`          | `~/.claude/settings.json`           | `skillOverrides: {"<name>": "off"}`                  | symlink into `~/.claude/skills`    |
| Cursor      | `~/.cursor`          | none                                | none (no config lever exists)                        | symlink into `~/.cursor/skills`    |
| IBM Bob     | `~/.bob`             | none                                | none (undocumented, unverified)                      | symlink into `~/.bob/skills`       |

A harness counts as installed when its config directory exists — the same probe the `skills` CLI uses. Commands only act on installed harnesses. `fleet harness ls` shows all six with their installed state, config directory, and whether fleet can write a per-skill off switch.

What fleet never touches, for every harness:

- the canonical store: skills are never moved, renamed, or edited, and disabling never touches their files
- the skills CLI lockfile (`~/.agents/.skill-lock.json`): read-only, always
- skill frontmatter: no `disable-model-invocation` or similar cross-harness levers, ever
- config content fleet doesn't own: comments, unknown keys, formatting, and rules fleet didn't write are preserved or flagged, never silently changed

Per-harness skills directories (`~/.config/opencode/skills`, `~/.pi/agent/skills`, `~/.codex/skills`, `~/.claude/skills`, `~/.cursor/skills`, `~/.bob/skills`) get two kinds of traffic: managed symlinks for custom skills (always pointing inside the adopt destination `ls` scans) where the harness discovers through links, and removal of redundant links where it doesn't. Sync never removes tracked-collection or fallback links. Details below.

## opencode

**File:** `~/.config/opencode/opencode.jsonc` (JSONC — comments allowed).

One file, two config dialects:

- **V1**: `permission.skill` — a string shorthand or a pattern → effect map — plus `skills: { "paths": [...], "urls": [...] }`
- **V2 beta**: `permissions` — an array of `{action, resource, effect}` rules — plus `skills: ["<path>", ...]`

Semantics are identical: the last matching rule wins, and `deny` hides the skill from the catalog and rejects the tool call (`ask` and `allow` leave it on).

**What a disable writes.** Fleet detects the dialect already present in the file and writes that one:

```jsonc
// V1: exact-name deny appended last, so "last matching rule wins" resolves to it
"permission": { "skill": { "git-helper": "ask", "tdd": "deny" } }
```

```jsonc
// V2: one rule appended to the array
"permissions": [{ "action": "skill", "resource": "tdd", "effect": "deny" }]
```

A string shorthand is converted to a map that keeps its meaning; a missing `permission`/`permissions` subtree is created fresh.

**Dialect detection.** A `permissions` array or any `skills` key marks V2; a `permission` object marks V1; otherwise the file is treated as V1. The asymmetry is deliberate: V2 auto-migrates V1 keys (verified live, mixed files included), while V1 silently drops V2-only keys — so V1 is the safe default for files without V2 markers. A file whose only marker is the V1-shaped `skills: {paths, urls}` object is correctly not read as V1, because V2's decoder silently skips that shape.

**Enable** removes only fleet's own entries: exact-name keys in V1, and in V2 only rules that exactly match fleet's three-key shape. Pattern rules and blanket denies that still disable the skill are flagged and left alone.

**Custom skills** are wired as an extra skill source in the matching dialect — the adopt destination joins `skills.paths` (V1) or the flat `skills` array (V2), not per skill.

**Never touched:** comments, unknown keys, formatting, and deny rules fleet didn't write (they surface as flags in sync's output). The `~/.config/opencode/skills` directory is never written to — links there into the canonical store are redundant (opencode scans the store natively) and sync removes them, but tracked-collection or fallback links are never removed.

**Limitation:** opencode has no live reload. Toggles take effect on the next session; the TUI says so after every apply. The V2 beta moves fast — config discovery and skill-ID resolution are still in flux upstream, so the adapter is re-probed on each beta bump. Skill directory names equal frontmatter names so one rule targets the skill under both ID schemes.

## pi

**File:** `~/.pi/agent/settings.json` (strict JSON — no comments).

**What a disable writes.** A force-exclude entry appended to the `skills` array, in the exact form `pi config` itself produces — a path relative to `~/.agents`:

```json
{
  "model": "pi-main",
  "skills": ["-skills/tdd/SKILL.md"]
}
```

**Enable** removes fleet's exact entries. The bare `!<glob>` exclusion form is recognized when reading (it does disable skills) but fleet never writes it, and a glob that still excludes a skill is flagged, not touched. Plain path entries in the array only add discovery sources; they never disable anything.

**Custom skills** are wired as a plain path entry in the same `skills` array — the adopt destination joins it once, not per skill.

**Never touched:** unknown keys and formatting (the read-modify-write parses strictly and preserves both), glob exclusions, and the `~/.pi/agent/skills` directory's own contents beyond redundant-link removal (tracked-collection / fallback links never removed).

**Limitations:** the global file only — pi's project settings cannot reach user-scope skills. The exclusion mechanism is code-verified against pi (earendil-works/pi v0.84.4), not execution-tested; pi releases near-daily, and sync fixes any drift the next time it runs.

## codex

**File:** `~/.codex/config.toml` (TOML — comments and unknown keys allowed).

**What a disable writes.** A `[[skills.config]]` table appended to the file:

```toml
[[skills.config]]
name = "tdd"
enabled = false
```

An existing fleet-shape block is flipped in place (`enabled = true` becomes `false`, trailing comment intact). Codex lets later entries override earlier ones, which is why fleet's entry goes last.

**Enable** removes fleet's blocks (simple `name`-selected tables). Blocks fleet doesn't recognize — `path` selectors, extra keys, multi-line values — are flagged and left exactly as they are. Codex ignores unknown keys at runtime, so nothing fleet leaves behind changes behavior.

**Custom skills** get a symlink in `~/.codex/skills` pointing inside the adopt destination. codex scans the canonical store natively, so links there into the store are redundant and sync removes them, but tracked-collection / fallback links are never removed.

**Never touched:** every line outside fleet's own `[[skills.config]]` blocks passes through byte for byte — TOML has no comment-preserving encoder, so fleet edits at the line level instead of re-encoding the file.

## claude code

**File:** `~/.claude/settings.json` (strict JSON — no comments).

**What a disable writes.** An entry in `skillOverrides`:

```json
{
  "skillOverrides": { "tdd": "off" }
}
```

The object is created when missing and dropped when the last entry leaves. **Enable** removes the `"off"` entry. Other values (claude's own, like `"user-invocable-only"`) are not disables and stay untouched.

**Discovery is link-based.** Claude is not a native canonical-store reader: a skill reaches it only through a symlink (or directory) named after the skill in `~/.claude/skills`. Without one the skill's state is `absent` — the `ls` column shows `-`, and disabling is a no-op because there is nothing for claude to load. Doctor reports a recorded disable whose link is missing as state drift.

The skills CLI's own store links in `~/.claude/skills` are load-bearing (they are claude's only path to installed skills), so they are never treated as redundant — only a link whose target is missing is reported, as a broken symlink.

**Custom skills** get a managed symlink pointing inside the adopt destination — the same link mechanism, wherever the destination is.

## Cursor

**Config written:** none. Cursor reads `~/.agents/skills` natively (documented) and has no config-level per-skill disable. The only documented lever, the `disable-model-invocation` frontmatter flag, would cross-talk with pi and claude, so per-skill disable for Cursor is out of scope. Every skill reads as `on`, toggles for Cursor are explicit no-ops, and the TUI marks the column `cursor!`.

**Custom skills** get a symlink in `~/.cursor/skills` pointing inside the adopt destination (symlinks supported since IDE 2.5 / CLI 2026.02.27).

**Never touched:** any Cursor config file, ever. Redundant store links in `~/.cursor/skills` are removed by sync, since Cursor scans the canonical store natively.

## IBM Bob

**Config written:** none. Bob reads `~/.agents/skills` natively — verified empirically on this machine; the docs only mention `~/.bob/skills` (and have an open bug about global skills, #288). No per-skill disable mechanism is documented or verified, so Bob behaves like Cursor: every skill reads as `on`, toggles are explicit no-ops, the column is marked `bob!`. Doctor surfaces the discovery picture if Bob's global-skill behavior ever changes.

**Custom skills** get a symlink in `~/.bob/skills` pointing inside the adopt destination — customs live outside what Bob scans, so the link is Bob's only path to them.

**Redundant links here are a judgment call:** the `skills` CLI auto-creates store links in `~/.bob/skills`, which double-cover skills Bob already sees through the canonical store. Sync removes those (a user decision, locked in the spec) while managed custom-home links for custom skills always stay — they point outside the store, so they are never classified as redundant.

## States across harnesses

`fleet skill ls` and `--json` report one of three states per skill and harness:

- `on` — the harness discovers the skill and nothing disables it
- `off` — the harness discovers the skill but its config disables it
- `absent` — the harness cannot discover the skill at all (today, only claude code, when no link exists); the table shows `-`

Cursor and Bob never report `off`: with no disable mechanism, `on` is the only state they can express.
