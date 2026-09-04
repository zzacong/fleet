# ADR 0002: Multi-repo customs with `skill pull` and an explicit adopt target

Date: 2026-09-04
Status: Accepted

## Context

ADR 0001 gave fleet one versioned home for custom skills: the single
`skillsRepo` pointer (`FLEET_REPO` env > `config.json`, no walk-up) plus the
unversioned fleet-home fallback. That model cannot represent a second
collection (a personal repo plus a team repo, or a checkout outside fleet
home), and fresh-machine setup is a manual clone plus a pointer command.
Adopt's destination is implied by whichever single repo happens to be set,
which breaks down once several collections are tracked.

## Decision

1.  **Tracked set, not a single pointer.** The versioned homes are an ordered
    set: the explicit non-fleet-home repo-root list from
    `~/.config/fleet/config.json` (list order is precedence order), then every
    fleet-home checkout slot present on disk (immediate children of
    `~/.config/fleet/repos/`, alphabetical by directory name), tracked by
    convention with no config write. The single-path `FLEET_REPO` env override
    still prepends in code (highest precedence, included in bare pull) for
    sandboxes, but it is not user-documented. The unversioned fleet-home
    fallback (`~/.config/fleet/skills/`) is not part of the set; display and
    adopt layers add it where they need it.

2.  **`skill pull` clones and fast-forwards.** `fleet skill pull <git-url>
[path]` clones a customs repo; bare `fleet skill pull` fast-forwards
    every tracked repo with a per-repo report (`updated`, `current`,
    `skipped`, `failed`). With no path, the checkout lands in the
    auto-tracked fleet-home slot derived from the URL (final path or colon
    segment, trailing slashes and `.git` stripped) with no config write. An
    explicit path inside fleet home stays convention-tracked; an explicit path
    outside fleet home is appended once to the explicit list (no duplicates,
    no reordering). Re-pulling an existing path never reorders the list — and
    never registers it either; registration happens only on fresh clone.

3.  **Fast-forward only, never destructive.** An existing checkout with the
    same remote pulls with `git pull --ff-only`. Dirty or diverged trees fail
    with their state surfaced; a different remote fails unless `--force` is
    given. Fleet never stashes, merges, rebases, or resets. Missing git is a
    clean dependency error. Auth and prompts inherit the user's stdio; fleet
    adds no credential flags. Non-git explicit entries are skipped with a
    warning on bare pull instead of failing the run. After clone or pull the
    collection dir (`<repo>/skills`) is ensured (created with a warning on the
    empty-repo first run), the home is wired/linked, and sync runs.

4.  **Scan precedence across the set.** Display order is explicit-list order,
    then fleet-home checkouts alphabetically, then the unversioned fallback,
    then the canonical store. A name present in more than one source appears
    once (winner shown); every collision is doctor drift. Anything resolved
    through a custom home carries no lockfile provenance and reports the
    outdated badge as unknown by definition (`—` in the table, `null` in
    JSON). Doctor additionally warns on unscanned adopt targets and non-git
    explicit entries.

5.  **Adopt takes an explicit destination.** Resolution order: `--into
<skills-dir>` for the run, else the configured adopt target, else the
    prompt/default rules. The flag takes a collection dir (not a repo root),
    created on demand, and never persists. The configured target is likewise a
    collection dir, absent meaning the fleet-home fallback. Prompt candidates
    always include the fallback alongside the tracked collections; zero
    tracked means the fallback alone with no prompt. Prompts appear only on a
    terminal — piped ambiguous runs fail listing the candidates and the flag
    hint. The save-back question defaults to No. An unscanned target still
    proceeds, with a doctor warning.

6.  **Config schema.** `config.json` gains `skillsRepos` (array of absolute
    non-fleet-home repo roots, order significant, empty omitted) and
    `adoptTarget` (absolute collection dir, absent meaning fallback). Both
    honor home expansion on set, absolute-path validation, atomic write,
    canonical formatting (`skillsRepos`, `adoptTarget`, then sorted unknowns),
    and unknown-field preservation. Key aliases follow the kebab/camel pair
    convention (`skills-repos`/`skillsRepos`, `adopt-target`/`adoptTarget`).
    There is deliberately no env override for the adopt target and no
    multi-value env syntax for the list. The tracked list is managed by
    `skill pull`, never by `config set`.

7.  **Retire the single pointer with no code migration.** The old `skillsRepo`
    file key, its accessors, and its `skills-repo` CLI surface are removed.
    The file key still loads as a preserved-verbatim unknown — never
    interpreted, never validated, never migrated — so old-key-only configs
    behave as unset (fallback display). Pre-public checkouts migrate by hand
    (below). Help text, examples, completions, and error hints tell the
    multi-repo story; the retired keys fail with a hint toward `skill pull`
    and `adopt-target`.

8.  **One new seam.** Git runs behind an injected `Runner` mirroring the
    existing skills-CLI runner pattern (real runner shells out with inherited
    stdio; tests inject a stub recording operations). Everything else extends
    existing seams: fleet-config (load/save/validate), paths-resolution (home
    injection plus tracked-set derivation), snapshot-union, adopt-flow,
    wire/link, and the command tree.

## Manual migration for pre-public checkouts

No code migrates the old key. If `~/.config/fleet/config.json` still carries
`"skillsRepo": "/old/path"`:

1.  Note the old path, then pick a lane:
    - Keep the checkout where it is: hand-add its root to the explicit list
      (`"skillsRepos": ["/old/path"]`). The next `fleet skill pull`
      fast-forwards it. (Re-running pull onto the existing path fast-forwards
      but does not register it — registration happens only on fresh clone.)
    - Or move it under the auto-tracked parent (`mv /old/path
~/.config/fleet/repos/<name>`) and delete the lane above: no config
      write needed, ever.
2.  Restore the adopt default, if the old repo's `skills/` was the implied
    home: `fleet config set adopt-target <repo>/skills`. Without a target,
    adopt prompts (tracked collections plus fallback) or lands in the
    fallback when nothing is tracked.
3.  Delete the stale `"skillsRepo"` key by hand. Left in place it is preserved
    verbatim and ignored — harmless, but confusing next to `skillsRepos`.

## Consequences

- Fresh-machine setup is `fleet skill pull <git-url>`; second-machine sync is
  bare `fleet skill pull`. Out-of-band checkouts are remembered after one
  pull; fleet-home checkouts need no config at all.
- Adopt is explicit everywhere: one-off (`--into`), habitual (`adoptTarget`),
  or a deliberate prompt choice with opt-in persistence. Automation never
  blocks on stdin.
- `internal/paths` loses `Repo`/`WithRepo`/`RepoSkills`; `TrackedRepos` is the
  one derivation. `snapshot`, `doctor`, `customs`, and the CLI read the set;
  the old key survives only as a preserved unknown plus a retired-key error
  hint.
- User docs (README, command reference, skills catalog/collection contract,
  state/config explanations) and contributor docs (`CONTEXT.md` tracked-set
  vocabulary, architecture, testing) describe this model; the single-pointer
  terms are retired everywhere except this ADR's history.

## Alternatives considered

- Automatic migration of `skillsRepo` into `skillsRepos` in code. Rejected:
  the pointer's intent is ambiguous (was it the adopt home? the only
  collection? stale?), and every checkout is pre-public, so a documented
  three-step hand migration beats a guess that rewrites user config.
- Multi-value env syntax for the repo list / env override for the adopt
  target. Rejected: env cannot express order-plus-convention cleanly, and the
  single-path override stays available in code for sandboxes.
- Shallow/partial clones, fleet-side credential flags, a Go git
  implementation. Rejected: the system git with inherited stdio already
  handles private repos; fleet adds no auth surface.
