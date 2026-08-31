# Spec: Fleet — per-harness agent skill manager

Status: resolved

## Problem Statement

Skills installed through the `skills` CLI land in `~/.agents/skills` and immediately become active in every harness that scans that directory (opencode, pi, codex, Cursor). There is no way to keep a skill installed but inactive for one harness without uninstalling it everywhere. There is no grouping (custom vs installed, by source repo), no visibility into which harness sees which skill, no update status, and the `skills` CLI creates per-agent symlinks that are redundant for harnesses that scan the canonical store directly (e.g. `~/.config/opencode/skills/*`, `~/.cursor/skills/*`).

## Solution

Fleet, a Go CLI + TUI (`fleet skill <verb>` commands, bare `fleet` opens the TUI). Fleet keeps a state file as the single source of truth for per-harness enablement and projects that state into each harness's own native config mechanism. Skills never move; the canonical store stays exactly where the `skills` CLI puts it, so `skills update` keeps working on disabled skills. Custom (hand-written) skills live in the fleet repo's `skills/` directory and are discovered by harnesses through their own extra-path config. Fleet wraps the `skills` CLI (thin, non-interactive) for add/update, and provides `sync` (repair drift), `doctor` (read-only health report), `adopt` (migrate custom skills into the repo), and an outdated badge computed from lockfile hashes.

## User Stories

1. As a solo developer, I want to disable a skill for one harness without uninstalling it, so that the skill stays installed and updatable but stops loading in that harness.
2. As a solo developer, I want `skills update` to keep updating disabled skills, so that re-enabling gives me the latest version.
3. As a solo developer, I want a per-skill × per-harness matrix in a TUI, so that I can see and change exactly which harness loads which skill.
4. As a solo developer, I want the TUI to only show harnesses actually installed on my machine, so that I don't toggle settings for tools I don't use.
5. As a solo developer, I want a big ASCII hero banner when the TUI launches, so that the tool feels finished.
6. As a solo developer, I want plain verbs (`fleet skill ls`, `on`, `off`, `update`, `doctor`, `sync`, `adopt`) for scripting, so that I can automate without the TUI.
7. As a solo developer, I want `fleet skill ls --json`, so that scripts can consume skill state.
8. As a solo developer, I want my hand-written skills to live in the fleet repo's `skills/` directory, so that my custom work is versioned in one place.
9. As a solo developer, I want custom skills to be discoverable by opencode and pi through their own path configs, so that moving them out of the canonical store doesn't break my workflow.
10. As a solo developer, I want custom skills visible in codex/claude code/Cursor via managed symlinks, so that every harness sees them.
11. As a solo developer, I want installed skills grouped by source repo/author, so that I can see what came from where.
12. As a solo developer, I want custom and installed skills visually separated, so that I can tell my own work from third-party installs.
13. As a solo developer, I want an "update available" badge per skill, so that I know what is stale without running update.
14. As a solo developer, I want one-key "update all" in the TUI, so that updating is one action.
15. As a solo developer, I want redundant per-agent symlinks (e.g. `~/.config/opencode/skills/*`, `~/.cursor/skills/*`) removed automatically, so that the skills CLI's link spam stops accumulating.
16. As a solo developer, I want the cleanup to never remove `~/.bob/skills` links, so that Bob keeps working (Bob has no native canonical-store access; its links are load-bearing).
17. As a solo developer, I want running the `skills` CLI by hand to keep working, so that fleet never becomes a hard dependency.
18. As a solo developer, I want manual edits to harness configs to be surfaced by doctor, so that I can choose to keep my edit or let sync overwrite it — never silently overwritten.
19. As a solo developer, I want sync to run automatically after a wrapped `skills update`, so that disabled skills stay disabled even when update re-creates links or resurrects state.
20. As a solo developer, I want disabling a skill to never move, rename, or edit the skill's files, so that the canonical store stays pristine and `skills update` semantics are unchanged.
21. As a solo developer, I want the state file and all projections to survive the skills CLI being run independently, so that fleet never becomes a hard dependency.
22. As a solo developer, I want `fleet skill ls` to show, per skill: name, custom/installed, source repo, description, per-harness on/off, and update-available, so that one screen answers "what do I have and where is it active".
23. As a solo developer, I want the TUI to stage toggles and apply them together, so that I can flip several skills and commit once (matching the prototype UX).
24. As a solo developer, I want a filter (`/`) in the TUI, so that I can find a skill among dozens.
25. As a solo developer, I want `fleet skill on <name>` / `off <name>` with an optional `--harness` flag (default: all installed harnesses), so that CLI toggles are quick.
26. As a solo developer, I want `fleet skill update` to wrap `skills update -g -y` and then sync, so that disabled skills stay disabled after update.
27. As a solo developer, I want `fleet skill doctor` to explain anything it would change before sync changes it, so that I'm never surprised.
28. As a solo developer, I want the TUI to note that some harnesses pick up config changes on next session (opencode has no live reload), so that I'm not confused when a toggle "doesn't work".
29. As a solo developer, I want the state file to be forward-compatible with the upstream skills CLI enable/disable proposal (PR #641's `enabled` field shape), so that we can converge with upstream later.
30. As a solo developer, I want `fleet --version` and shell completions, so that the tool feels finished.

## Implementation Decisions

- **Stack: Go + Bubble Tea v2 + Bubbles/Lipgloss v2 + Cobra.** Single static binary, distributed via `go install`/goreleaser (Homebrew later). Pin `charm.land/*/v2` imports; a build + test suite guards against v1-idiom regressions from AI-written code.
- **Binary and command shape:** binary `fleet`; `fleet skill <verb>` for the skills domain (ls, on, off, update, doctor, sync, adopt); bare `fleet` opens the TUI. `skillctl` is not used. Hero ASCII banner on TUI launch only; suppressed when piped or `--quiet`.
- **Core architecture: state file + per-harness adapters, no file moves.** The state file (fleet-owned, versioned schema, lives under `~/.config/fleet/`) is the single source of truth for per-harness enablement. Each adapter projects that state into the harness's own native config. Skills are never moved, renamed, or frontmatter-edited by fleet.
- **Verified per-harness mechanisms:**
  - opencode: one adapter, two config dialects in the same shared file (`~/.config/opencode/opencode.jsonc`). V1: `permission.skill` object map + `skills: {paths, urls}`; V2 beta: `permissions` rules array + `skills: [...]` flat array. Semantics are identical (last matching rule wins, deny hides from the catalog and rejects the tool call). Write strategy: detect the dialect already present in the file and write that one — V2 auto-migrates V1 keys (verified live on this machine, mixed files included), while V1 silently drops V2-only keys, so V1 shape is the safe default for files without V2 markers. Guard against the V1-detection edge case (a file containing only `skills: {paths, urls}` is not detected as V1 and V2's decoder silently skips it). Custom skills via the skills paths config in the matching dialect. Re-probe discovery and permissions on every V2 beta bump; keep skill dir names equal to frontmatter names so one rule targets the skill under both ID schemes.
  - pi: force-exclude entries in the `skills` array of `~/.pi/agent/settings.json`, written in the exact form `pi config` itself produces: `-skills/<name>/SKILL.md` (verified against the user's live settings file; bare `!<name>` glob form also works per source, but fleet matches pi's own output format). Global file only — project settings cannot reach user-scope skills. Custom skills via the same `skills` array as a plain path entry. Verified twice against earendil-works/pi (formerly badlogic/pi-mono, now redirects) at v0.84.4 / commit 853a80d; docs at pi.dev/docs/latest.
  - codex: `[[skills.config]]` with `name = "..."` + `enabled = false` in `~/.codex/config.toml` (TOML comments allowed, unknown keys ignored at runtime); customs via symlinks into `~/.codex/skills`.
  - claude code: `skillOverrides: { "<name>": "off" }` in `~/.claude/settings.json` (strict JSON, no comments); customs via symlinks into `~/.claude/skills`. Claude is not a native canonical-store reader, so the symlink is its discovery path.
  - Cursor: reads `~/.agents/skills` natively (documented); customs via `~/.cursor/skills` symlinks (symlinks supported since IDE 2.5 / CLI 2026.02.27). No config-level per-skill disable exists; the only documented lever is `disable-model-invocation` frontmatter, which cross-talks with pi and claude — **Cursor per-skill disable is out of scope for v1**; the UI shows the limitation.
  - IBM Bob: empirically reads `~/.agents/skills` (user-verified, decision locked). Per-skill disable: undocumented → v1 gives Bob **no per-skill disable, same as Cursor**; toggling for Bob is a no-op with a clear message. The skills CLI's auto-symlinks into `~/.bob/skills` are redundant double-coverage and fleet removes them (user decision). Customs still get managed symlinks into `~/.bob/skills` — repo skills are outside the canonical store, so the link is Bob's only path to them.
- **Redundant link cleanup (Q15):** sync removes per-agent symlinks for harnesses that natively scan the canonical store: opencode (`~/.config/opencode/skills`), codex (`~/.codex/skills`), cursor (`~/.cursor/skills`), pi (`~/.pi/agent/skills`), and Bob (`~/.bob/skills` — user decision: Bob reads the canonical store natively, so the skills CLI's links there are redundant). Managed custom-skill symlinks (ticket: adopt) are never treated as redundant — they point at the repo, not the canonical store.
- **Wrapped skills CLI policy (Q7):** fleet shells out to `skills add -g -y -s <names>` / `skills update -g -y`. Always explicit flags, stdin piped closed (unexpected prompts fail fast instead of hanging the TUI), output captured for failure display, then sync runs. Fleet never writes the skills CLI lockfile (read-only) and never deletes entries it doesn't own, so running `skills` by hand keeps working. `skills check` is an undocumented alias of `update` — not used.
- **Outdated badge (Q16):** fleet implements its own check: compare `skillFolderHash` in the lockfile against the current GitHub tree hash per source repo (one cheap API call per repo, conditional requests for rate limits).
- **Adopt:** one-time migration of custom skills into the repo; also usable to promote a forked installed skill.
- **Grouping (Q6):** custom vs installed, installed subgrouped by source repo (from lockfile `source`). No tags in v1.
- **Scope (Q5):** global/personal only. Project-level skill dirs out of scope for v1.
- **Detection:** harness presence detected by config-dir probes (`~/.config/opencode`, `~/.pi`, `~/.codex`, `~/.claude`, `~/.cursor`, `~/.bob`) — same technique the skills CLI uses.
- **Banner (Q8):** hero ASCII banner on TUI launch only; one-line branded header for plain verbs; suppressed when piped or `--quiet`.
- **v1 feature set (Q9):** doctor, sync, outdated badge, `--json` output + shell completions. Export/import manifest and in-TUI browse/install are next, not v1. Usage tracking never.

## Testing Decisions

- **The seam: the harness adapter interface.** One seam — every adapter implements the same interface (read current enablement from a harness config; write enablement for a skill). All tests run through this seam plus the injected home root. No adapter touches the filesystem except through the injected root.
- **What makes a good test here:** assert external behavior (the written config file content, the reported state), never internal call order. Every test builds a fake home in `t.TempDir()`; nothing outside the project directory is touched. `FLEET_HOME` env override exists so the same sandboxing works for manual dry runs.
- **Modules tested:** every adapter (unit + golden files, including unknown-key/comment preservation for JSONC/TOML and strict-JSON writers), the state file (versioned schema, forward compatibility), sync (drift scenarios: link re-created by skills CLI, manual edit, missing marker), doctor (each problem class), skillscli wrapper (explicit-args construction, stdin-closed failure path), scan (lockfile parsing incl. sanitized names).
- **Prior art in repo:** the prototypes' fixture-driven approach (`prototypes/fixture/skills.json`) maps to fixture-driven tests; `prototypes/PLAN.md`'s CLI surface doubles as the acceptance checklist.

## Out of Scope

- Project-level skill management (per-project state, relative symlinks).
- Cursor per-skill disable (no config mechanism; frontmatter cross-talks with pi and claude). Cursor ships with visibility + customs only; doctor explains.
- Tags/categories beyond custom vs installed + source-repo grouping.
- In-TUI browse/install from the skills.sh catalog.
- Export/import manifest for machine bootstrap (next after v1).
- Usage tracking (needs harness hooks; fragile).
- Reimplementing the skills CLI registry client.
- Fleet writing the skills CLI lockfile (read-only forever).

## Further Notes

- **Sources** (full reports under `.scratch/agent-skills-manager/`): skills CLI internals — vercel-labs/skills v1.5.23 source + skills.sh/docs; harness discovery — `2026-08-31-agent-skills-discovery.md`; Bob + Cursor — `2026-08-31-bob-cursor-research.md` (bob.ibm.com IDE/Shell docs, cursor.com docs + staff-confirmed forum threads, IBM/ibm-bob issues); pi — earendil-works/pi @ 853a80d + pi.dev/docs/latest (badlogic/pi-mono redirects there); opencode V1+V2 — opencode.ai/docs, opencode.ai/v2/docs, anomalyco/opencode `dev` branch, plus live probes against the installed V2 beta; TUI stack comparison — `2026-08-31-tui-stack-comparison.md`.
- The upstream skills CLI has an open PR (#641) adding enable/disable with an `enabled` field in their lockfile. Fleet's state schema keeps that shape in mind for potential upstream convergence.
- opencode has no live reload; the TUI should note that toggles take effect next session for some harnesses. The V2 beta moves fast (config discovery and skill-ID resolution mid-refactor on the dev branch) — re-run the opencode probes on each beta bump.
- Bob's docs claim only `~/.bob/skills` discovery and have an open bug about global skills (#288); empirically Bob reads `~/.agents/skills` (user-verified). The Bob adapter therefore relies on empirical behavior; doctor should surface if Bob's global skills ever stop appearing.
- pi's exclusion mechanism is code-verified twice against earendil-works/pi (v0.84.4, commit 853a80d), not execution-tested; pi releases near-daily, and reconcile covers drift.
- The skills CLI's `check` command is an undocumented alias of `update` — fleet does not use it.
- Repo hygiene when implementing: `prototypes/go-tui/skillctl*` is already gitignored; the stale prototype config (`~/.config/skillctl/`) has been deleted.
