# Spec: Path-tracked custom skill directories

Status: resolved

## Problem Statement

I keep my own skills in a git repo at a path I choose, and I edit them in place. Fleet can only bring custom skills onto a machine one way: `fleet skill pull` clones and fast-forwards a repo into a slot under `~/.config/fleet/repos/` that fleet picks. I don't want fleet cloning anything, and I don't want my skills living inside fleet's config directory where I'll open and modify them. I already run git myself, so fleet's git machinery adds nothing.

What I want is to point fleet at a directory of skills that already exists anywhere on my machine, have fleet track it, and have its skills reach every harness. That does not exist today. Instead there is a `pull`/`drop` vocabulary built around git that I have to re-learn every time I come back to it.

## Solution

Replace the git-based `pull` and `drop` with two path verbs.

`fleet skill add-dir <path>` registers an existing directory of skills and wires its skills into every installed harness. `fleet skill remove-dir <path>` unregisters it and removes the managed links, leaving the directory untouched. Fleet never clones, pulls, or deletes anything.

The tracked set becomes a plain ordered list of collection directories in config. The `~/.config/fleet/repos/` convention and the `FLEET_REPO` env override go away. A `~` argument expands to the home directory, so `fleet skill add-dir ~/Developer/projects/agent-skills/skills` works.

## User Stories

1. As a fleet user with my own skills repo, I want `fleet skill add-dir <path>` to register it, so that fleet tracks a directory I already own and edit.
2. As a fleet user, I want `add-dir` to expand `~` to the home directory, so that `fleet skill add-dir ~/Developer/projects/agent-skills/skills` works.
3. As a fleet user, I want a relative path to resolve against the working directory, so that I can register a directory without typing an absolute path.
4. As a fleet user, I want `add-dir` to fail loudly when the path does not exist, so that a typo is caught instead of silently tracking nothing.
5. As a fleet user, I want `add-dir` to fail loudly when the path is not a directory, so that I cannot register a file.
6. As a fleet user, I want `add-dir` to fail when the directory holds no skills, so that I do not track an empty or wrong directory.
7. As a fleet user, I want a skill to be an immediate child directory holding a `SKILL.md`, so that the tracked shape is predictable, `skill-a/SKILL.md`, `skill-b/SKILL.md`.
8. As a fleet user, I want `add-dir` to refuse the canonical store, so that installed skills are never registered as custom.
9. As a fleet user, I want `add-dir` to refuse the fleet-home fallback, so that the always-on fallback is never listed twice.
10. As a fleet user, I want `add-dir` to refuse any path inside fleet home, so that fleet's own config directory is never used as a collection.
11. As a fleet user, I want `add-dir` to be a no-op when the cleaned path is already tracked, so that re-running it is safe.
12. As a fleet user, I want `add-dir` to refuse a path nested inside or containing an already tracked path, so that precedence is never ambiguous.
13. As a fleet user, I want `add-dir` to refuse when a skill name in the incoming directory collides with another tracked directory or the fallback, listing the colliding names and their existing homes, so that skill identity stays unambiguous.
14. As a fleet user, I want a collision with the canonical store to be allowed with a doctor warning rather than a refusal, so that a custom skill can deliberately shadow an installed one.
15. As a fleet user, I want a newly added directory appended to the list, so that the existing precedence order is never reordered.
16. As a fleet user, I want `add-dir` to link every skill into every installed harness and then sync, so that the skills are discoverable immediately.
17. As a fleet user, I want `add-dir` to print one headline plus one linked line per harness per skill, so that I can see exactly what was wired in.
18. As a fleet user, I want `remove-dir <path>` to unregister a tracked directory, so that fleet stops managing it.
19. As a fleet user, I want `remove-dir` to remove the managed links for that directory's skills and then sync, so that nothing dangles in a harness.
20. As a fleet user, I want `remove-dir` to never delete my directory, so that my files are always safe.
21. As a fleet user, I want `remove-dir` on an untracked path to fail and list the tracked directories, so that a typo does not silently succeed.
22. As a fleet user, I want `remove-dir` on a tracked path that is missing from disk to still unlist it, so that a hand-deleted directory cannot wedge the config.
23. As a fleet user, I want `remove-dir` to accept `~` and relative paths exactly like `add-dir`, so that the two verbs resolve the same way.
24. As a fleet user, I want the tracked directory list in config to be the single source of precedence, so that the model is explainable in one sentence.
25. As a fleet user, I want the first tracked directory to win for both display and the harness link, so that `fleet skill ls` and the actual link agree.
26. As a fleet user, I want a skill name present in two homes to be reported by doctor, so that I can find and resolve the collision.
27. As a fleet user, I want fleet to have no git dependency at all, so that a missing git binary can no longer break a fleet command.
28. As a fleet user, I want bare `pull` and `drop` gone, so that there is no dead command to confuse me.
29. As a fleet user with an old `skillsRepos` config, I want that key ignored but preserved, so that nothing breaks and no silent migration rewrites my config.
30. As a fleet user, I want a one-line migration, add my new collection and remove the stale entry, so that I can move to the new model without guesswork.
31. As a fleet user, I want doctor to flag a tracked directory missing from disk, so that an incomplete skill index is visible.
32. As a fleet user, I want doctor to flag a tracked directory with zero skills, so that an empty registration is visible.
33. As a fleet user, I want doctor to flag duplicate or nested tracked directories from a hand-edited config, so that a manual edit cannot hide a mistake.
34. As a fleet user, I want `adoptTarget` to keep working as a collection directory, unchanged by `add-dir` and `remove-dir`, so that adopt is unaffected.
35. As a maintainer dogfooding fleet, I want the watcher to watch every tracked directory, so that edits inside my collection show up in a diff.
36. As a contributor, I want the contributor docs, architecture notes, testing notes, and glossary to describe the path-tracked model, so that future changes build on the true model.
37. As a docs reader, I want the user docs to document `add-dir`, `remove-dir`, the `skillsDirs` key, and the precedence rule, so that reference matches behavior.

## Implementation Decisions

- **CLI surface.** Two verbs under the skill command: `add-dir <path>` and `remove-dir <path>`, each taking one required path. The git-based `pull` and `drop` verbs are removed. No bare mode: there is nothing to update without git.
- **Path resolution.** `~` expands to the home directory and a relative path absolutizes against the working directory. The result is cleaned and stored as the identity. There is no basename or slot-name matching: both verbs address a directory by path only.
- **Tracked unit.** A collection directory whose immediate children are skill directories, each holding a `SKILL.md`. This is the one registration shape; there is no repo root plus implied `skills/` subdirectory anymore.
- **Add validation.** `add-dir` requires the path to exist, be a directory, and hold at least one skill, and requires the directory to be readable for scanning. It refuses the canonical store, the fleet-home fallback, any path inside fleet home, an already tracked path, a path nested inside or containing a tracked path, and a skill name that collides with another tracked directory or the fallback. It reports the specific reason for every refusal.
- **Collision policy.** A collision against the canonical store is allowed, because custom outranks canonical and fleet cannot rename an installed skill; doctor reports it. A collision against another custom home is refused at add time. The precedence rule (first tracked directory wins) governs anything that appears on disk later, when a file is dropped into either directory.
- **Config schema.** The machine-local config gains `skillsDirs`, an ordered array of absolute collection directories, order significant, empty means none. `skillsRepos` is retired and treated as a preserved unknown field, never read, with no automatic migration in code, matching how the earlier `skillsRepo` key was retired. `adoptTarget` is unchanged. Home expansion on set, absolute-path validation, atomic write, canonical formatting, and unknown-field preservation all carry over.
- **Precedence and the link winner.** Precedence is the `skillsDirs` order, then the unversioned fallback, then the canonical store. The first home holding a name wins. The current link fan-out that makes the lowest-precedence home win a name collision is fixed so the managed harness link points at the same winner the listing shows.
- **Removal semantics.** `remove-dir` unlists the directory from `skillsDirs`, removes the managed links that resolve under it, and syncs. It never deletes the directory or its contents. A tracked path missing from disk still unlists cleanly. An untracked path is an error that lists the tracked directories.
- **Deleted surface.** The git clone/fast-forward seam and the pull package are deleted. The drop package collapses into removal of a list entry. The `FLEET_REPO` env override, the fleet-home checkout parent, and the inside-fleet-home path classification are removed. The scan path stops deriving a `skills/` subdirectory from a tracked entry and scans the tracked directory itself.
- **Tracked-set module.** The existing tracked-set module owns the directory list as its domain seam: list, add, and remove, with all validation rules. It is the one writer of `skillsDirs` and the one reader of the order. The skill index and both verbs read through it instead of re-deriving anything.
- **Doctor.** New findings: a tracked directory missing from disk or not a directory, a tracked directory with zero skills, duplicate or nested tracked directories, and the existing unscanned adopt-target warning. The non-git warning is removed. Cross-home double presence stays.
- **Watcher.** The watcher gains one target per tracked directory, read from config, so edits inside a collection are diffed. The watcher skill document's target table and count are updated to match.
- **Docs.** One new ADR records the path-tracked decision and supersedes the git-based sections of the earlier multi-repo ADR and the extract-collection ADR's runtime contract. The user command reference, the state and config explanation, the undo guide, the README, and the contributor architecture and testing docs follow. The glossary retires the git-based terms and adds the two verbs and the new config key.
- **Seams.** No new seam. The tracked-set module is the primary test surface; config keeps its round-trip seam extended to the new key; the command layer keeps its end-to-end run seam with the existing fake-home fixtures; the skill index, snapshot, and doctor keep their existing seams. Two obsolete seams, the git runner and the drop flow, are deleted.

## Testing Decisions

- **What makes a good test.** Assert external behavior only: command output and exit code, the files and symlinks written, the config bytes after a run, the config key round-trip, and doctor findings for a built fake home. Never assert internal parsing details, message styling, or call counts. Every test builds its homes in a temporary directory; no test touches the real home, the real config, git, or the network.
- **Which modules will be tested.**
  - The tracked-set module: every add validation rule (missing path, not a directory, zero skills, unreadable, canonical store, fallback, inside fleet home, duplicate, nested both directions, custom-home name collision, canonical-store collision allowed), the append and no-reorder rule, removal (untracked error listing, missing-on-disk still unlists), and precedence order.
  - Config: `skillsDirs` round-trip, order preserved, empty means none, the retired key preserved as an unknown, malformed values error, unknown fields preserved, home expansion and absolute-path validation.
  - Command layer end to end: `add-dir` and `remove-dir` stdout and stderr, the links created and removed per harness, the sync effect, the headline and per-harness line shape, exit codes for each refusal, and that no git binary is consulted.
  - Skill index and snapshot: precedence with no `skills/` join, the first home winning a name collision, and the managed link pointing at the same winner.
  - Doctor: each new finding, and the removed non-git warning.
  - Paths: the helper deletions covered by updating the existing tests.
- **Prior art.** The existing pull, drop, snapshot, skill-index, doctor, and command-tree test suites, which already build fake homes and assert stdout, stderr, symlinks, and config bytes. New tests follow the same shape. The deleted git-runner stub tests are removed; the end-to-end command tests replace the git-driven ones.
- **Watcher.** No automated seam. It is a dev tool, verified by running it once against a temporary home and confirming a tracked directory appears as a target and diffs on edit.

## Out of Scope

- Any git usage in fleet, including clone, fast-forward, remote checks, dirty-tree checks, and a missing-git error path.
- Automatic migration of the retired `skillsRepos` key, and any rewriting of an existing config file beyond the keys the two verbs manage.
- Reordering a tracked directory, a move or priority flag, and prepending.
- Resolving a target by basename or an auto-derived slot name.
- Changing collision semantics to key on the directory name rather than the frontmatter name.
- Changes to the adopt flow beyond the wording the new model requires, and any change to `adoptTarget` behavior.
- The canonical store, the skills CLI install and update backend, per-project checkout skills, the TUI, and second-store or XDG splits.
- Content of any skills collection, and publishing to skills.sh.

## Further Notes

- This supersedes the git-based parts of the multi-repo customs decision and the runtime contract of the extract-collection decision. It does not reopen the split of the collection into its own repo, which stands.
- The motivating case is the extracted `agent-skills` repo, kept where the user edits it, registered with a single `add-dir` once the change ships.
- The `FLEET_REPO` env override was kept working in code only for sandboxes; those tests move to the config list, and the override is deleted rather than re-documented.
