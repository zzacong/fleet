# Spec: Monorepo with designated skills repo and fleet-home fallback

Status: resolved

## Problem Statement

Fleet today ties custom skills to the fleet repo's `skills/` directory and finds that repo by walking up from `$PWD` to the nearest `.git` or via `FLEET_REPO`. This has three problems. First, running `fleet` outside the checkout — or after distributing a binary with no checkout — leaves `Repo == ""`, so custom skills are invisible and `adopt` fails with "no fleet repo found". Second, the walk-up is surprising: invoking `fleet` inside an unrelated git checkout silently picks up that checkout's `skills/` and would wire/link the wrong collection. Third, fleet-the-CLI and skills-the-content share one history and one release, but the user's goal is a versioned skills collection publishable to skills.sh on its own cadence, with fleet versioning separately, plus a future GUI and a marketing/docs site. Today there is no machine-local pointer that works from any working directory and no unversioned fallback for ad-hoc customs.

## Solution

Turn `github.com/zzacong/fleet` into a monorepo with an explicit, non-magical home for custom skills.

- `skills/` at the repo root is the versioned **skills collection** — what skills.sh publishes. Fleet code alongside it is ignored.
- Go CLI lives at `apps/cli` (binary stays `fleet`, `go.work` at the root, `go.mod` moves with it). `apps/desktop` is reserved for a future GUI. No rename to `fleet-cli`.
- User-facing docs (`cli.md`, `harnesses.md`, `state-file.md`, `undo.md`) move from root `docs/` to `apps/docs` as a **Starlight** site; root `docs/` keeps contributor docs (`architecture.md`, `testing.md`, `adr/`, `agents/`). One published site today; `apps/www` can be added later.
- Custom skills are discovered from two explicit homes, united in one display: the **skills repo** (versioned, when set) and **fleet home** `~/.config/fleet/skills` (unversioned fallback). The designated skills repo is resolved only via `FLEET_REPO` env or `~/.config/fleet/config.json: skillsRepo` — no implicit walk-up. When unset, only fleet-home is scanned. Canonical store `~/.agents/skills` is the third source. Display uses skills-repo precedence (`skillsRepo > fleet-home > canonical`); every cross-source name collision is doctor drift.
- Add persistent machine-local pointer `~/.config/fleet/config.json` and CLI `fleet config get|set|unset|list` (one key today: `skillsRepo`). After a fresh clone the user runs `fleet config set skills-repo ~/Developer/fleet` once.
- Retarget `adopt` (and future `fleet skill new`) to move/scaffold into the home that will be scanned: designated repo's `skills/` when one is set, otherwise `~/.config/fleet/skills`, with wiring/links targeting that home.
- Project skills in `.agents/skills` stay per-project and out of fleet scope.

## User Stories

1. As a skill author, I want my custom skills in `skills/<name>/SKILL.md` at the repo root, so that the collection is publishable to skills.sh without fleet code interfering.
2. As a skill author, I want `fleet skill ls` to show customs from my designated skills repo plus fleet-home plus canonical store in one table, so that I see the full picture without switching directories.
3. As a skill author, I want the same skill name in two homes to appear once with skills-repo precedence (`skillsRepo > fleet-home > canonical`), so that my versioned checkout edits shadow my home while developing.
4. As a skill author, I want doctor to flag any name present in more than one of the three sources as double presence, so that I can resolve collisions by hand.
5. As a fleet user on a fresh clone, I want to run `fleet config set skills-repo ~/Developer/fleet` once and then run `fleet` from any working directory, so that I don't need to be inside the checkout.
6. As a CI operator, I want `FLEET_REPO` env to override `fleet config`, so that sandboxes and tests can point at a temp skills repo without writing a file.
7. As a fleet user with no versioned repo set, I want `fleet skill ls` and `adopt` to work against `~/.config/fleet/skills` alone, so that ad-hoc customs need no git.
8. As a fleet user who later designates a skills repo, I want existing fleet-home customs to remain visible alongside the repo's collection, so that I don't lose work.
9. As a fleet user, I want `fleet config get skills-repo` to show the current pointer and `fleet config list` to show all fleet config, so that I can audit my machine.
10. As a fleet user, I want `fleet config unset skills-repo` to clear the pointer, so that I can fall back to fleet-home only.
11. As a fleet user, I want fleet to never walk up to `.git` implicitly, so that running in an unrelated checkout doesn't pick up the wrong skills.
12. As a fleet user, I want `fleet skill adopt <name>` to move a skill from the canonical store into my designated repo's `skills/` when one is set, otherwise into `~/.config/fleet/skills`, so that adoption versions in the right place.
13. As a fleet user, I want `adopt` to wire the target home into opencode and pi and link it for codex, claude code, Cursor and Bob in one run, so that every installed harness discovers it.
14. As a fleet user, I want adopting an already-adopted skill to re-ensure wiring/links without moving, so that a partially failed run heals on the next adopt.
15. As a skill author, I want a future `fleet skill new <name>` to scaffold `SKILL.md` frontmatter in the same home `adopt` uses, so that creation and adoption share the target.
16. As a skill author, I want hand-created directories in either custom home (with `SKILL.md`) to appear in `fleet skill ls` with no registry step, so that the filesystem is the registry.
17. As a skill author, I want `rm -rf` of a custom skill dir to be its uninstall, so that no state file edit is needed.
18. As a fleet user, I want enable/disable in the state file to apply to custom skills regardless of which home they live in, so that harness rules target the skill name, not its path.
19. As a fleet user, I want `fleet skill off <name> --harness <harness>` and `on` to work for customs exactly as for installed skills, so that toggling is uniform.
20. As a fleet user, I want sync to project recorded disables into each harness's native config on every command, so that state stays the source of truth.
21. As a fleet user, I want sync to remove redundant links only for harnesses that scan the canonical store natively and never touch fleet-home or skills-repo links, so that custom discovery is never deleted.
22. As a fleet user, I want doctor to report redundant links, broken symlinks, untracked entries, manual edits, state drift, double presence, stale lock entries, missing dirs and unreadable configs, so that I can see what sync would do.
23. As a monorepo contributor, I want `apps/cli` to build via `go.work` at the root and `apps/docs` to build via `pnpm -F docs`, so that Go and Astro don't fight.
24. As a monorepo contributor, I want `go install github.com/zzacong/fleet/apps/cli/cmd/fleet@latest` semantics preserved via the workspace, so that distribution still works.
25. As a docs reader, I want `apps/docs` Starlight to render user-facing docs plus a skills catalog that reads `skills/*/SKILL.md` frontmatter at build, so that marketing and reference share one site.
26. As a contributor, I want root `docs/` to keep `architecture.md`, `testing.md`, `adr/` and `agents/` as contributor docs distinct from the published site, so that internal and external docs don't conflate.
27. As a fleet maintainer, I want `apps/desktop` reserved with a placeholder, so that a future GUI has a locked name and path.
28. As a skills.sh publisher, I want `skills/` at the root to be taggable separately from fleet releases, so that collection and CLI version independently.
29. As a future maintainer, I want `FLEET_HOME` to still override `~/.config/fleet` for sandboxes, so that tests never touch the real home.
30. As a fleet user, I want `outdated` badges to remain tri-state and report unknown for customs and non-GitHub sources, so that version checks don't guess.
31. As a project user, I want `.agents/skills/watcher` and any future project-local `.agents/skills/<name>` to stay out of `fleet skill ls`, so that per-project skills stay per-project.
32. As a watcher user, I want `scripts/watcher/watch.py` and `.agents/skills/watcher/SKILL.md` to cover fleet home and the designated skills repo (not a hard-coded repo-skills path), so that snapshots and diffs stay accurate after the monorepo move.

## Implementation Decisions

- **Monorepo layout.** Repo root holds `skills/` (publishable collection). Go CLI moves to `apps/cli` with its `go.mod` and `cmd/fleet`; root `go.work` contains `use ./apps/cli`. `apps/desktop` is a placeholder directory for a future GUI. Go binary stays `fleet`; no `fleet-cli` rename. `goreleaser` main becomes `./apps/cli/cmd/fleet`.
- **Docs split.** Root `docs/` retains contributor docs (`architecture.md`, `testing.md`, `adr/`, `agents/`). User-facing `cli.md`, `harnesses.md`, `state-file.md`, `undo.md` migrate to `apps/docs/src/content/docs` as Starlight content. `apps/docs` is an Astro + Starlight app with `astro.config.mjs` and a Starlight sidebar. `apps/www` is not created now; it can be added later.
- **Two custom homes, one scanner.** The scanner unions `~/.agents/skills` (canonical), `~/.config/fleet/skills` (fleet home), and `<skillsRepo>/skills` when a skills repo is set. Fleet home is `FleetConfigDir()/skills` and is always scanned if present. Skills repo path is `FleetConfig: skillsRepo` or `FLEET_REPO` env — absolute, validated, must be a git repo root (contains `.git`) at use time; missing/invalid is an error at the command that needs it, not a silent skip. `DiscoverRepo` walk-up is removed as an implicit fallback (may remain only as a helper for `fleet config` suggestion text, never called from the read path).
- **Precedence.** `FLEET_REPO` env overrides `fleet config` file which overrides unset. No walk-up. Tests must assert `env > config > ""`.
- **Fleet config.** New file `~/.config/fleet/config.json` beside `state.json` and `tree-cache.json`, `FLEET_HOME`-aware. JSON, atomic write via temp+rename, unknown fields preserved for forward compat, canonical formatting. One key today: `skillsRepo` (string, absolute path). `fleet config get <key>`, `set <key> <value>`, `unset <key>`, `list` (`--json` for machine). `set` validates the path exists and contains `.git` or warns; `get` on missing key is empty, not an error.
- **Skills collection contract.** `skills/` at the collection root; each immediate child dir containing `SKILL.md` is a skill. Frontmatter `name` falls back to dir name. Skills.sh publishing reads this tree; fleet code elsewhere is ignored.
- **Retargeted adopt.** `adopt` resolves the custom target as designated repo's `skills/` when a skills repo is set, otherwise fleet-home `skills/`. It scans canonical and both custom homes for collision, moves the dir from canonical into the target, ensures the target parent exists, then wires the target via `SourceWiring` for opencode and pi and links via `SkillLinker` for codex, claude code, Cursor and Bob. No state file write. Doctor's double-presence rule covers the new homes.
- **Wiring/linking target.** `WireSkillSource` and `LinkCustomSkill` take the resolved custom home path, not a hard-coded repo path. Native-scan flag still decides sync's redundant-link removal (custom links are never redundant).
- **Project skill boundary.** `.agents/skills` at `$PWD` is not scanned by fleet; no `ProjectSkills()` path. Documented as out of scope.
- **Package management.** `pnpm-workspace.yaml` with `packages: ["apps/*"]`, root `package.json` private, `apps/docs/package.json` for Astro. `go.work` for Go workspace. CI builds `apps/cli` via `go test ./...` in `apps/cli` context and `apps/docs` via `pnpm -F docs build`.
- **Module path.** Keep `module github.com/zzacong/fleet` (at `apps/cli/go.mod`) so imports remain stable; workspace indirection avoids `module github.com/zzacong/fleet/apps/cli` rename. Revisit if a second Go module is added.
- **Schema changes.** `state.json` unchanged (version 1, disable-only, unknown-field preserving). `config.json` is independent, versioned implicitly by unknown-field preservation; no `version` field initially.
- **No XDG split.** Fleet home stays `~/.config/fleet` for both state and custom fallback; no `~/.local/share` second root.
- **Watcher.** `scripts/watcher/watch.py` `WATCH_TARGETS` and `.agents/skills/watcher/SKILL.md` watch table are updated for the new homes and the monorepo move: `fleet-config` continues to cover `~/.config/fleet` (now includes `config.json`, `skills/` fleet-home, `state.json`, `tree-cache.json`), `repo-skills` is retargeted to the monorepo `skills/` at the repo root (and after the move is resolved relative to the watcher location depth — `apps/cli` move changes the relative path), and harness targets stay derived from `internal/paths`. The watcher never reads `fleet config` implicitly — its repo-skills target is the checkout's `skills/` for dogfooding; fleet-home customs are already covered by the `fleet-config` dir walk.
- **Proposed seam for fleet config.** New package owning the config file (load/save/validate) behind an interface the CLI and `paths` consume; the config read is a new highest seam alongside `paths.FromEnv`, `snapshot.Build` and `customs.Adopt` so tests can inject a fake home and a temp config.

## Testing Decisions

- **What makes a good test.** Test external behavior (what a command prints, what files are written, what `snapshot.Build` returns for a given home), not internal parsing details. Fake homes via injected `FLEET_HOME` in `t.TempDir` are the harness for every test; no test touches the real home. Preserve-comments/formatting cases assert whole-file bytes plus the write report.
- **Watcher is tested via its own script.** `scripts/watcher/watch.py` is verified by running `--initial` and `--label` against a fake home and a temp repo checkout and asserting the watch table and diff markers; `SKILL.md` table is kept in sync.
- **Which modules will be tested.** The new fleet config package (load/save precedence `env > config > ""`, missing file is empty, malformed JSON is error, unknown fields preserved), `paths` resolution (absolute-path validation, `FleetConfigFile`, `FleetHomeSkills`), `snapshot.Build` union (canonical + fleet-home + skillsRepo, skills-repo precedence `skillsRepo > fleet-home > canonical`, doctor double-presence), `customs.Adopt` retargeting (move into the correct home, wire/link the correct target, collision and missing-skill errors), harness wiring/links with the new target, and the `fleet config` CLI verbs.
- **Prior art.** Adapter fixture tests with fake homes (`internal/harness/*_test.go`), scan tests for `ScanStore`, state round-trip tests with unknown-field preservation, snapshot tests that build a fake home and assert rows/harnesses, and `customs/adopt_test.go` move+wire+link cases. New tests follow the same pattern: build a fake home, write fixtures, run through the seam, assert file bytes and reports.

## Out of Scope

- `fleet skill new` scaffold (follows this spec, same target home).
- Starlight site build/content beyond scaffolding and migration of the four user docs.
- Actual `skills/` collection content (custom skills themselves).
- GUI at `apps/desktop` beyond a placeholder README.
- Separate git histories or `git subtree split` automation for skills vs CLI releases.
- Publishing to skills.sh automation or `skills` CLI integration beyond the existing canonical store contract.
- Managing project-local `.agents/skills` via fleet.
- XDG data-home split (`~/.local/share`) or `state.json` piggyback for fleet config.
- Reintroducing `DiscoverRepo` walk-up as an implicit fallback.

## Further Notes

- Fresh clone requires one explicit `fleet config set skills-repo ~/Developer/fleet` (or `FLEET_REPO` env) — no magic. Document this in `README` and `docs/cli.md` and in the `adopt` error hint when no skills repo is set.
- `skills/` at the monorepo root is what `skills add` publishing and skills.sh will point at; fleet releases should tag as `fleet-v*` so collection tags (`skills-v*` or plain) don't collide if split later.
- ADR 0001 is the source of truth for this spec; deviations need an ADR update.

