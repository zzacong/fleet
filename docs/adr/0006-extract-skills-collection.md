# ADR 0006: Extract the skills collection into its own repo

Date: 2026-10-04
Status: Accepted

> **Supersedes:** ADR 0001 §1 (the repo-root `skills/` collection) and ADR 0003
> §7 ("The skills collection stays in this repo"). The skills-catalog coupling
> in ADR 0004 is dropped with it.

## Context

Fleet has shipped since ADR 0001 with the versioned skills collection at the
fleet repo root (`skills/`), published to skills.sh and rendered as a docs
catalog. ADR 0003 kept it there and named the signals that would justify a
split: skills commits dominating release history, the collection needing
version semantics of its own, or skills.sh requiring a repo-root shape. It
rejected splitting "now" as premature pre-first-release.

Fleet is now past first release (v0.3.2), and the collection and the binary
still share one history, one release pipeline, and one docs build. The coupling
shows in the seams: `release-please` must exclude `skills/` so collection
commits cannot cut a binary release, the docs site reads the collection at
build time, and the Vercel gate watches `skills/` for it. The collection is
content with its own cadence; the binary is a tool.

## Decision

1.  **The collection moves to `zzacong/agent-skills`**, a public repo with the
    shape fleet already expects: a top-level `skills/` directory, each immediate
    child with a `SKILL.md` a skill. The collection README is promoted to the
    repo root.
2.  **History is extracted, not recreated.** `git subtree split -P skills`
    replays the collection's commits into a standalone history; one `chore:`
    commit then nests the skill directories back under `skills/`, and the
    README lands at the repo root from the split.
3.  **Fleet drops every in-repo collection coupling.** The `skills/` directory
    is removed; the docs catalog page and its `SkillsCatalog` component are
    deleted; `vercel.json` no longer watches `skills/`; `release-please` no
    longer excludes it; the watcher's `repo-skills` target is removed; and the
    `skills` commit scope retires from `AGENTS.md`.
4.  **The contract is unchanged.** Fleet still discovers customs from a tracked
    repo's `skills/` collection subdir, so the new repo is consumable with
    `fleet skill pull <url>` and no code change.

## Consequences

- Fleet's repo is binary, docs, and tooling only; the collection versions and
  releases on its own.
- The docs site no longer lists skills. The collection's own repo README is the
  canonical list.
- A `feat`/`fix` commit can no longer be ambiguous between binary and
  collection; the `skills` scope is gone.
- Every machine must repoint its tracked set: `fleet skill pull <agent-skills
url>`, `fleet skill drop <old fleet root>`, and set the adopt target to the
  new repo's `skills/`. Until then, `fleet skill ls` does not show the customs
  and their managed links dangle.
- No Go code changes: fleet never read the repo-root `skills/` at runtime.

## Alternatives considered

- **Git submodule** (`fleet/skills` → `agent-skills`). Rejected: keeps the docs
  catalog and Vercel gate working with no code change, but adds clone
  `--recursive`, Vercel submodule fetch, and CI friction for content that does
  not need to live inside the binary's tree.
- **`git-filter-repo` for a prefix-preserving history.** Rejected: needs a tool
  install; the single nesting commit from `git subtree split` is good enough
  for a small collection.
- **Keep the collection in the fleet repo.** Rejected: the split signals ADR
  0003 named now hold, and the collection should version independently.
