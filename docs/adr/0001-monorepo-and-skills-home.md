# ADR 0001: Monorepo with a designated skills repo and fleet-home fallback

Date: 2026-09-02
Status: Accepted

> **Superseded in part (2026-09-06):** the nested `apps/cli` + `go.work` layout
> decided below is superseded by ADR 0003 (release strategy), implemented in
> ticket 01. The Go module now lives at the repo root, `apps/docs` moved to
> `www/`, `apps/desktop/` was deleted, and `apps/` is gone. This ADR remains
> the record of the original monorepo decision.

## Context

Fleet managed custom skills by tracking them in the fleet repo's `skills/` directory. `Paths.Repo` was `FLEET_REPO` env or `DiscoverRepo($PWD)` walk to `.git`, and `RepoSkills()` was `<repo>/skills`. The walk-up was surprising — running `fleet` in an unrelated checkout could pick up the wrong repo — and a distributed binary has no checkout at all, so `Repo == ""` and adopt/scan broke. `~/.config/fleet/skills` did not exist.

The goal is narrower than "distribute fleet": keep custom skills versioned in git, publishable to skills.sh as a collection (`skills/<name>/SKILL.md`), while fleet itself versions separately. Conflating fleet-the-CLI and skills-the-content in one history and one path made distribution, publishing, and a future GUI awkward.

We also need a persistent machine-local pointer so `fleet` works from any `$PWD` without implicit discovery.

## Decision

1.  **Monorepo, one git history.** The repo at `github.com/zzacong/fleet` becomes a monorepo. Brand is `fleet`.

    - `skills/` at the repo root is the versioned **skills collection** — what skills.sh points at. Each `skills/<name>/SKILL.md` is a custom skill. Fleet code alongside is ignored by the publisher.
    - Go CLI moves to `apps/cli/` (binary stays `fleet`, `go.work` at the root `use ./apps/cli`, `go.mod` moves with it, `goreleaser` main becomes `./apps/cli/cmd/fleet`). `apps/desktop` is reserved for a future GUI. No `fleet-cli` rename.
    - User-facing docs (`cli.md`, `harnesses.md`, `state-file.md`, `undo.md`) migrate from `docs/` to `apps/docs` as a **Starlight** site. Root `docs/` keeps contributor docs (`architecture.md`, `testing.md`, `adr/`, `agents/`). One site today; `apps/www` can be added later for pure marketing.
    - Project skill `.agents/skills/watcher` stays per-checkout, out of fleet scope.

2.  **Two homes for custom skills, one display.**

    - Designated **skills repo** — the versioned home. Resolved per command as `FLEET_REPO` env > `~/.config/fleet/config.json: skillsRepo` > `""` (no implicit walk-up to `.git`; the pointer must be explicit). When set and valid, `<skillsRepo>/skills` is the versioned collection; when unset, no repo is scanned.
    - **Fleet home** `~/.config/fleet/skills/` — unversioned fallback, always available, no git. `FLEET_HOME` overrides the home root.
    - `snapshot.Build` unions the sources that exist: `~/.agents/skills` (canonical) + `~/.config/fleet/skills` (fleet-home) + `<skillsRepo>/skills` when a repo is set. Display uses skills-repo precedence (`skillsRepo > fleet-home > canonical`); every name collision across the scanned sources is doctor drift ("double presence").
    - **Filesystem is the registry.** A custom skill is any dir with `SKILL.md` in either custom home, no lockfile entry. No registry in `state.json`.

3.  **Machine-local pointer.** New file `~/.config/fleet/config.json` (next to `state.json`), atomic write, `FLEET_HOME`-aware. Shape `{"skillsRepo": "/abs/path"}` — one key today, unknown fields preserved for forward compat. CLI `fleet config get|set|unset|list` (and `fleet config set skills-repo <path>`). Env overrides file; there is no `DiscoverRepo` walk-up. After a fresh clone the user runs `fleet config set skills-repo ~/Developer/fleet` once and every later invocation works from any `$PWD`.

4.  **Adopt retargeted.** `fleet skill adopt <name>` moves `~/.agents/skills/<name>` into the custom home that will be scanned: designated repo's `skills/` when one is set, otherwise `~/.config/fleet/skills/`. Wiring (`opencode`, `pi` `SourceWiring`) and links (`codex`, `claude`, `cursor`, `bob` `SkillLinker`) target that home. Adding `fleet skill new <name>` scaffold (dir + `SKILL.md` frontmatter + wiring/links) is a follow-up, same target.

5.  **`fleet config` is one field today but lays ground.** No second store, no XDG split (`~/.local/share`), no `state.json` piggyback.

## Consequences

- No implicit repo discovery. `fleet` never walks up to `.git`; the only ways to point it at a versioned collection are `FLEET_REPO` env or `fleet config set skills-repo`. Fresh clone requires one explicit `fleet config set skills-repo ~/Developer/fleet`; `DiscoverRepo` is removed (or kept only for `fleet config` helper messaging, never as an implicit fallback).
- `skills/` at the root can be published to skills.sh without moving fleet code; fleet releases (`fleet-v*`) and collection releases can tag separately later, or split via `git subtree split` if histories must diverge.
- `docs/` split: contributor docs stay at the root, user docs live in `apps/docs` (Starlight). CI must build `apps/cli` via `go.work` and `apps/docs` via `pnpm -F docs`.
- `internal/paths` gains `FleetConfigFile()` and `FleetHomeSkills()`, and `FromEnv` reads `config.json` instead of calling `DiscoverRepo`. `internal/customs`, `internal/snapshot`, `internal/harness` wiring/links follow the new target. Tests must cover `env > config > ""` precedence and the no-walk-up rule.
- Not done in this ADR: actual `git mv` of Go code/docs, `go.work`/`pnpm-workspace.yaml`, Starlight scaffold, `fleet config` and `fleet skill new` verbs — those are tracer tickets off this ADR. This ADR is the spec they implement.

## Alternatives considered

- Keep `DiscoverRepo` walk-up as implicit fallback. Rejected: surprising — running `fleet` in an unrelated git checkout would silently pick up that checkout's `skills/` and wire/link the wrong collection.
