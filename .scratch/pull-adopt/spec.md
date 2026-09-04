# Spec: Multi-repo custom skills with `skill pull` and explicit adopt target

Status: resolved

## Problem Statement

I author my own skills in git (often private) and work from two machines. Today fleet tracks exactly one versioned home for custom skills via a single repo-root pointer, with the fleet-home fallback always on. There is no one-command way to clone my customs repo on a fresh machine, point fleet at it, and later pull the latest from the other machine. I end up running a manual clone plus a pointer command, and with more than one collection (e.g. a personal repo plus a team repo, or a checkout outside fleet home) the single-pointer model cannot represent what I track. Adopting also has no explicit home of its own: its destination is implied by whichever single repo happens to be set, which breaks down once several collections are tracked.

## Solution

Teach fleet to track many versioned custom-skill homes and to fetch them with one verb.

- `skill pull <git-url> [path]` clones a customs repo (defaulting into an auto-tracked fleet-home slot) and registers it when it lives outside fleet home; bare `skill pull` fast-forwards every tracked repo.
- The fleet config holds an explicit list of non-fleet-home repo roots plus an explicit adopt-target collection dir, replacing the single repo pointer (no automatic migration code; pre-public manual step, separately ticketed).
- Fleet-home checkouts are tracked by convention with no config write; explicit out-of-band paths are tracked by list.
- `adopt` gains an explicit destination (`--into` for one run, `adoptTarget` in config for the default) with a prompt when ambiguous, instead of implying the destination from a single pointer.
- Scanning, precedence, doctor drift, wiring/links, and sync extend from one repo to the ordered tracked set. All user docs, README, contributor docs, glossary, and ADR follow.

## User Stories

1. As a skill author with a private customs repo, I want `skill pull <git-url>` with no path to clone into an auto-tracked fleet-home slot, so that fresh-machine setup is one command with no config edit.
2. As a skill author, I want `skill pull <git-url> <path>` with an explicit path inside fleet home to clone without touching config, so that the checkout stays auto-tracked by convention.
3. As a skill author, I want `skill pull <git-url> <path>` with an explicit path outside fleet home to clone and append that repo root to the tracked list, so that my out-of-band checkout (e.g. beside other projects) is remembered.
4. As a skill author on a second machine, I want bare `skill pull` to fast-forward every tracked repo (explicit list plus every fleet-home checkout plus the env-pointed checkout when set), so that I get the latest customs with one command.
5. As a skill author, I want a pull onto an existing checkout with the same remote to fast-forward only, so that my local commits are never auto-merged away.
6. As a skill author, I want a pull that would point an existing path at a different remote to fail unless forced, so that I never silently repoint a checkout.
7. As a skill author with uncommitted changes or a diverged checkout, I want pull to fail with the working-tree state surfaced, so that I resolve it by hand instead of fleet stashing or resetting.
8. As a skill author, I want fleet to shell out to the system git with my normal auth (SSH agent, credential helper) and a full clone, so that private repos just work with no fleet-side credential flags.
9. As a skill author, I want a missing git binary to produce a clean failure, so that I know what to install.
10. As a skill author cloning an empty private repo, I want the collection dir created with a warning, so that first-run setup does not fail on an empty remote.
11. As a skill author, I want every pulled collection wired into the config-path harnesses and linked for the link-based harnesses followed by a sync, so that pulled customs are discoverable immediately.
12. As a fleet user, I want `skill ls` to show the union of the canonical store, the ordered explicit repos, the auto-tracked fleet-home checkouts, and the unversioned fleet-home fallback with deterministic precedence, so that I see one coherent table.
13. As a fleet user, I want the precedence to be explicit-list order first, then auto-tracked fleet-home checkouts alphabetically, then the unversioned fallback, then the canonical store, so that shadowing is predictable.
14. As a fleet user, I want any skill name present in more than one source to appear once (winner shown) with every collision reported as doctor drift, so that I resolve duplicates by hand.
15. As a fleet user, I want custom skills to keep reporting the outdated badge as unknown rather than guessed, so that version checks stay honest for git-managed customs.
16. As a skill author, I want `adopt <name>` with `--into <skills-dir>` to move (or re-ensure) exactly there with no prompt and no config write, so that one-off destinations stay one-off.
17. As a skill author, I want `adopt <name>` with an `adoptTarget` configured to use it with no prompt, so that my habitual home is zero-friction.
18. As a skill author, I want `adopt <name>` with no target configured and no tracked collections to land in the fleet-home fallback with no prompt, so that ad-hoc customs need no setup.
19. As a skill author, I want `adopt <name>` with no target configured and at least one tracked collection to get a numbered prompt that always includes the fleet-home fallback alongside the tracked collections, so that even a single tracked repo is an explicit choice against the default.
20. As a skill author at the adopt prompt, I want a yes/no follow-up (defaulting to No) offering to save my choice as the adopt target, so that persisting is deliberate.
21. As a fleet user in a pipe or script, I want an ambiguous adopt with no `--into` and no configured target to fail listing the candidates and the flag hint instead of blocking on stdin, so that automation never hangs.
22. As a fleet user, I want `config set adopt-target <path>`, `get`, `unset`, and `list` with the same alias, home-expansion, atomic-write, and unknown-field preservation behavior as the existing pointer key, so that machine-local config stays uniform.
23. As a fleet user, I want an adopt target pointing somewhere unscanned to still proceed but surface a doctor warning, so that footguns are visible, not silent.
24. As a CI operator, I want the existing single-path env override to keep working in code (prepended, highest precedence, included in bare pull) while disappearing from user docs, so that sandboxes keep a one-liner without blessing a multi-value env syntax.
25. As a fleet user, I want non-git explicit entries skipped with a warning during bare pull rather than failing the whole run, so that one uncloned path does not block the rest.
26. As a fleet user, I want a per-repo pull report (updated, already current, skipped, failed) rather than a single aggregate line, so that I know what happened per collection.
27. As a docs reader, I want the README quickstart and custom-skills explanation to describe pull, the tracked set, precedence, and adopt-target selection, so that I can set up from docs alone.
28. As a docs reader, I want the user docs site (command reference, skills catalog page, state/config explanations) to document the new verbs, flags, config keys, precedence chain, and prompt behavior, so that reference matches behavior.
29. As a contributor, I want the contributor docs, architecture notes, testing notes, glossary, and ADR to reflect the multi-repo model and the retired single-pointer semantics, so that future changes build on the true model.

## Implementation Decisions

- **Tracked set.** The tracked set is the union of: the explicit non-fleet-home repo-root list in config (list order is precedence order), every fleet-home checkout slot present on disk (alphabetical by directory name), the single-path env override when set (prepended, highest), and the unversioned fleet-home fallback for display/adopt-default purposes. Explicit entries are repo roots; each contributes its collection subdir when present. Fleet-home slots are tracked by convention (presence on disk), never by config write.
- **Default clone slot.** A pull with no explicit path derives a directory name from the URL (final path or colon segment, trailing slashes and suffix stripped) and clones under the fleet-home checkout parent. Deriving the name is a pure string decision covered by unit tests; anything git itself accepts as a URL is passed through untouched.
- **Config-write rule.** Clones landing inside fleet home write no config. Clones landing outside fleet home append their repo root to the explicit list (no duplicates; appending preserves existing order). Re-cloning onto an existing path never reorders the list.
- **Update semantics.** Existing path plus same remote resolves to fast-forward-only pull. Different remote is an error unless forced. Dirty or diverged working trees are errors that surface working-tree state; no stash, merge, rebase, or reset. Missing git is a clean dependency error. Auth and prompts inherit the user's stdio; fleet adds no credential flags.
- **Collection readiness.** After clone or pull, the collection subdir is ensured (created on demand with a warning when absent, covering the empty-private-repo first run), then the existing wire/link tail runs for that home, then the standard sync runs.
- **Scan precedence.** Display order is explicit-list order, then fleet-home checkouts alphabetically, then the unversioned fallback, then the canonical store. Name collisions show the winner only; every collision is doctor drift. Customs (anything resolved through a custom home) carry no lockfile provenance and report the outdated badge as unknown by definition.
- **Adopt destination.** Resolution order: `--into` flag for the run, else configured adopt target, else the prompt/default rules from the user stories. The flag takes a collection dir (not a repo root), created on demand, and never persists. The configured target is likewise a collection dir with the fleet-home fallback as its default when unset. Prompt candidates always include the fallback alongside tracked collections; zero tracked means the fallback alone with no prompt.
- **Prompt contract.** Prompts appear only on a terminal. Piped/non-terminal ambiguous runs fail with the candidate list and the flag hint. The save-back question defaults to No. A supplied flag suppresses both prompts and any config write.
- **Config schema.** The machine-local config file gains a list key holding absolute non-fleet-home repo roots (order significant, empty means none) and a scalar key holding the absolute adopt-target collection dir (absent means fallback). Both honor home expansion on set, absolute-path validation, atomic write, canonical formatting, and unknown-field preservation. Key aliases follow the existing kebab/camel pair convention. There is deliberately no env override for the adopt target and no multi-value env syntax for the list.
- **Retired single pointer.** The old single repo-root key is superseded with no automatic migration in code; existing pre-public checkouts are a manual step covered by a separate ticket. Docs and help text stop presenting the single-pointer model and the env override in user-facing surfaces (kept working in code only).
- **CLI surface.** New pull verb under the skill command with optional URL and optional path plus a force flag; bare invocation means update-all. Adopt gains a run-scoped destination flag. Config gains get/set/unset/list coverage for the adopt-target key consistent with the existing key. Help, examples, shell completions, and error hints updated for the multi-repo model.
- **Docs updates (explicitly in scope per request).** Update the root README, the user docs site pages (command reference, skills catalog/collection contract, state/config explanations), the contributor docs (architecture, testing), the domain glossary (supersede single-repo terms with the tracked-set vocabulary), and add an ADR for the multi-repo + adopt-target decisions. No placeholder-path cleanup beyond what the model change requires.
- **Seams.** Highest seams, fewest possible, one new: extend the existing fleet-config seam (load/save/validate) and the existing paths-resolution seam (home injection plus tracked-set derivation) with no new pattern; consume the existing snapshot-union seam, adopt-flow seam, wire/link seams, and command-tree seam; add exactly one new seam — a git runner behind an injected interface mirroring the existing skills-CLI runner pattern (real runner shells out with inherited stdio; tests inject a stub recording requested operations). Pull logic, bare-pull fan-out, and per-repo reporting all sit behind that runner so no test shells out to real git.

## Testing Decisions

- **What makes a good test.** Assert external behavior only: command output and exit status, files and symlinks written, config bytes after the run, snapshot rows and precedence for a built fake home, per-repo pull reports. Never assert internal parsing details, message styling, or call counts on the real git binary. Every test builds its homes in temp dirs; no test touches the real home, the real git config, or the network.
- **Which modules will be tested.** Config file handling (list round-trip, scalar round-trip, alias acceptance, missing file is empty, malformed JSON errors, unknown fields preserved, home-expansion and absolute-path validation); tracked-set resolution (env prepended, explicit order kept, fleet-home convention scan alphabetical, inside-vs-outside clone write rule); pull flow via the stubbed git runner (fresh clone default and explicit paths, same-remote fast-forward, different-remote error/force, dirty/diverged errors, missing-git error, empty-collection creation warning, non-git skip with warning, per-repo report); snapshot union and precedence across three-plus sources with collision drift; adopt resolution (flag wins, configured target, zero/one/many candidate prompt rules, non-terminal failure hint, save-back default No, unscanned-target warning); command-tree behavior for the new verbs and flags.
- **Prior art.** Fake-home adapter fixture tests, store-scan tests, state round-trip tests with unknown-field preservation, snapshot tests asserting rows and harness columns from a built home, adopt move-plus-wire-plus-link tests, config load/save precedence tests, and CLI verb tests driving the command tree against a fake home asserting stdout/stderr plus file bytes. New tests follow the same shape; the stubbed git runner follows the existing runner-stub pattern used for the skills-CLI backend.

## Out of Scope

- Automatic migration of the old single repo-root key in code (manual pre-public step, separately ticketed).
- A Go git implementation, shallow/partial clone modes, or fleet-side credential/SSH flags.
- Multi-value env syntax for the repo list or any env override for the adopt target.
- Auto-persisting prompt choices without the explicit yes/no step, or changing adopt history semantics beyond destination resolution.
- Skills-collection content itself, publishing-to-catalog automation beyond documenting the contract, per-project checkout skills management, GUI work, second-store or XDG data-home splits, walk-up repo discovery revival, subtree-split automation, or release-tagging schemes.

## Further Notes

- Docs updates across README, the user docs site, contributor docs, glossary, and ADR are part of this work, not a follow-up, per explicit request.
- The old single-pointer env override keeps working in code (undocumented) so sandbox and test one-liners survive; user docs must not present it as the multi-repo story.
- Example paths using a personal `Developer` checkout are pre-existing doc placeholders, not leaks; leave them unless the doc rewrite naturally neutralizes them.
