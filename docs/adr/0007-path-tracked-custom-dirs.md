# ADR 0007: Track custom skill directories by path, not git

Date: 2026-10-04
Status: Accepted

> **Supersedes:** ADR 0002 §1 (the tracked set of explicit repo roots plus
> fleet-home checkouts), §2 (`skill pull` clones and fast-forwards), §3
> (fast-forward-only, never destructive), §4 (scan precedence across the
> set), §6 (the `skillsRepos` config schema), §7 (the retired single
> pointer's hint toward `skill pull`), and §8 (the git runner seam). ADR
> 0002 §5 (the explicit adopt destination) still stands. Also supersedes
> ADR 0006 §4 (the collection is consumable with `fleet skill pull <url>`).
> The split of the collection into `zzacong/agent-skills` in ADR 0006
> stands.

## Context

ADR 0002 made `fleet skill pull` the only on-ramp for custom skills: clone a repo into an auto-tracked slot under `~/.config/fleet/repos/`, or an explicit path, then fast-forward every tracked repo on a bare pull. That model assumed fleet should fetch and update the collections it tracks.

The reality is different. A user authors skills in their own repo, opens it to edit, and runs git themselves. They do not want fleet cloning anything, and they do not want their collection living inside fleet's config directory. The git machinery also made `pull` and `drop` the confusing pair: one verb that cloned and fast-forwarded, one that deleted a fleet-home checkout from disk, both wrapped in a vocabulary that does not match "I have a directory of skills and I want fleet to use it." The skills CLI already owns install and update for published skills, so fleet's git seam was extra surface for a job the user was already doing.

## Decision

1. **Two path verbs replace pull and drop.** `fleet skill add-dir <path>` registers an existing directory of skills. `fleet skill remove-dir <path>` unregisters it. Fleet never clones, pulls, or deletes a collection. `~` expands to the home directory and a relative path resolves against the working directory. Both verbs address a directory by path only, never by a derived slot name or a basename.

2. **A tracked entry is a collection directory.** Its immediate children are skill directories, each holding a `SKILL.md`. The repo-root-plus-`skills/` derivation is gone; the tracked path is the collection itself.

3. **`add-dir` validates loudly.** The path must exist, be a directory, hold at least one skill, and scan. It refuses the canonical store, the fleet-home fallback, any path inside fleet home, an already tracked path, a path nested inside or containing a tracked path, and a skill name that collides with another tracked directory or the fallback. A collision against the canonical store is allowed, because custom outranks canonical and fleet cannot rename an installed skill; doctor reports it.

4. **The config key is `skillsDirs`.** An ordered array of absolute collection directories, order significant, empty means none. `skillsRepos` is retired to a preserved unknown field, never read, with no automatic migration in code, matching how the earlier `skillsRepo` key was retired. `adoptTarget` is unchanged.

5. **First tracked directory wins.** Precedence is `skillsDirs` order, then the unversioned fallback, then the canonical store. The managed harness link is fixed to point at the same winner the listing shows; previously the fan-out let the lowest-precedence home win a name collision, which disagreed with display. **Transitional precedence while the old model still lands.** Until `skillsRepos` is retired, both models resolve together, deterministically: every `skillsDirs` entry in list order (scanned directly), then every legacy repo-derived collection (the explicit repo-root list in order, then the fleet-home checkout slots alphabetically, each joined with `skills/`), then the unversioned fleet-home fallback, then the canonical store. An explicit collection dir therefore outranks every legacy source.

6. **`remove-dir` never deletes.** It unlists the path, removes the managed links resolving under it, and syncs. A tracked path missing from disk still unlists cleanly; an untracked path is an error listing the tracked directories.

7. **The git seam and the fleet-home convention are deleted.** The pull package, the drop flow, the `FLEET_REPO` env override, the fleet-home checkout parent, and the inside-fleet-home classification all go. The tracked-set module becomes the directory-list manager: list, add, remove, with every rule in one place. The skill index scans tracked directories directly. Doctor gains findings for a tracked directory missing or empty, and for duplicate or nested entries from a hand-edited config.

## Consequences

- Fresh-machine setup is `fleet skill add-dir <path>` after the user checks the repo out with plain git. Fleet has no git dependency, so a missing git binary can no longer break any command.
- Moving a collection is a `remove-dir` plus an `add-dir`. Fleet never places skills where the user did not choose.
- The `~/.config/fleet/repos/` convention and the sandbox-only `FLEET_REPO` override disappear from the model; sandbox tests set the config list instead.
- A name collision among custom homes is caught at add time rather than surfacing later as drift. A collision that appears later still resolves by precedence and is reported by doctor.
- Old configs keep their `skillsRepos` key verbatim and behave as if no custom directories are tracked. The migration is one documented pair of commands.
- The watcher gains a target per tracked directory, since the collection no longer lives somewhere the watcher already watched.

## Alternatives considered

- **Keep `fleet skill pull` and add `add-dir` alongside.** Rejected: `pull` implies fleet fetching and updating, which is the exact job being removed, and keeping both leaves two on-ramps and two mental models.
- **Reuse `skillsRepos` and redefine its entries as collection directories.** Rejected: the name would lie and old entries meant repo roots, so the change would be silent and ambiguous. A new key with the old one preserved as an unknown matches the established retirement pattern.
- **Fail hard on any collision, including the canonical store.** Rejected: fleet cannot rename an installed skill, and custom already deliberately outranks canonical, so a hard failure would block a valid shadow. A doctor warning keeps it visible.
- **Fail only at add time and let presentation pick a winner.** Rejected: a collision can appear later when a file is dropped into either directory, so the precedence rule and the link winner must be correct regardless of when the collision appears.
- **Keep the `FLEET_REPO` env override for sandboxes.** Rejected: with the tracked unit changed, the override had no coherent meaning, and the tests can set the config list directly.
