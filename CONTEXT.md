# Fleet

Fleet manages agent skills across AI coding agents (harnesses): enable or disable a skill per harness without uninstalling it, while the `skills` CLI stays the install/update backend.

## Language

**Harness**:
An AI coding agent that discovers and loads skills. Fleet targets: OpenCode, Pi, Codex, Claude Code, IBM Bob, Cursor. A harness is detected by its config directory on this machine; the UI only shows harnesses that are installed.
_Avoid_: agent, client, platform

**Skill**:
A directory with a `SKILL.md` file describing an agent capability. The unit that gets enabled or disabled.
_Avoid_: plugin, extension

**Canonical store**:
`~/.agents/skills` — where the `skills` CLI installs and updates skills. Codex, OpenCode, Pi and Bob scan it directly. Fleet never moves or renames anything in it.
_Avoid_: skills dir, install dir

**Installed skill**:
A skill with provenance (source repo, hash) in the `skills` CLI lockfile (`~/.agents/.skill-lock.json`). Grouped by source repo.
_Avoid_: managed skill

**Custom skill**:
A skill written by the user; no lockfile entry. Lives in one of the tracked collections or the unversioned fleet-home fallback, and is discovered via wiring/links — never through the canonical store.
_Avoid_: local skill, hand-written skill

**Tracked set**:
The ordered versioned homes for custom skills: the explicit non-fleet-home repo-root list from `~/.config/fleet/config.json` (list order is precedence order), then every fleet-home checkout slot present on disk (immediate children of `~/.config/fleet/repos/`, alphabetical), tracked by convention with no config write. The single-path `FLEET_REPO` env override still prepends in code (highest precedence, included in bare pull) for sandboxes, but it is not user-documented.
_Avoid_: skills repo (singular), designated repo

**Explicit repo list**:
The `skillsRepos` key in `~/.config/fleet/config.json`: absolute non-fleet-home repo roots, order significant. Managed by `fleet skill pull` (an outside-home clone appends once; re-pulls never reorder), never hand-edited in the normal flow. Empty means none.
_Avoid_: repo pointer, skillsRepo (singular)

**Fleet-home checkout**:
One auto-tracked customs repo under `~/.config/fleet/repos/<name>/`. A pull with no path derives `<name>` from the URL and clones there with no config write; an explicit path inside fleet home likewise stays convention-tracked. Each checkout contributes its `skills/` collection subdir when present.
_Avoid_: fleet repo, customs repo

**Skills collection**:
The `skills/` directory at the root of a customs repo (`<repo>/skills/`). Each immediate child with a `SKILL.md` is a custom skill. Every tracked repo contributes its collection subdir when present; in the monorepo that is this checkout, `skills/` at its root is the publishable collection skills.sh points at.
_Avoid_: repo skills, custom skills dir

**Skills repo (retired term)**:
The old model: exactly one versioned home via the single `skillsRepo` pointer (`FLEET_REPO` env > config file, no walk-up). Superseded by the tracked set; the file key still loads as a preserved unknown but is never interpreted, never validated, and never migrated in code. Do not use in new docs or help text.
_Avoid_: fleet repo, designated repo, customs repo

**Fleet home**:
`~/.config/fleet/` — fleet's own config dir. Holds `state.json` (enablement), `config.json` (machine-local: the explicit repo list plus the adopt target), `skills/` (unversioned custom-skill fallback), and `repos/` (auto-tracked customs checkouts). `FLEET_HOME` overrides the home root.
_Avoid_: config dir, fleet dir

**Fleet config**:
`~/.config/fleet/config.json` — machine-local settings fleet reads on every command. Two keys: `skillsRepos` (array of absolute non-fleet-home repo roots, order significant, managed by `skill pull`) and `adoptTarget` (absolute adopt-target collection dir, absent means the fleet-home fallback). Both honor home expansion on set, absolute-path validation, atomic write, and unknown-field preservation. Missing file means nothing is set; only fleet-home customs are scanned.
_Avoid_: settings, prefs

**Adopt target**:
The default adopt destination: an explicit collection dir (`<repo>/skills/` of a tracked checkout, or anywhere unscanned with a doctor warning). Set with `fleet config set adopt-target <skills-dir>`; absent means the fleet-home fallback. A run-scoped `--into <skills-dir>` overrides it for one adopt with no prompt and no config write. With neither, adopt prompts over the tracked collections plus the always-offered fallback (no prompt when nothing is tracked), in a terminal only; piped runs fail with the candidate list and the flag hint.
_Avoid_: adopt home, default repo

**Project skill**:
A skill in the current checkout's `.agents/skills/` (e.g. `.agents/skills/watcher`). Per-project, committed with the project, discovered via the working directory. Fleet does not manage it; `fleet skill ls` does not list it.
_Avoid_: local skill, repo skill

**Outdated badge**:
The tri-state update marker per skill: outdated (the source repo's current tree hash differs from the lockfile's `skillFolderHash`), current, or unknown. Fleet checks GitHub directly — one call per source repo, cached under fleet's config dir — and reports unknown for customs (anything resolved through a custom home renders `—`, never checked by definition) and non-GitHub sources rather than guessing. The skills CLI is not involved (`check` there is an alias of `update`).
_Avoid_: stale marker, version check

**Fleet update notice**:
The notice that the running binary is older than the latest GitHub Release. Manual re-install, never auto-upgrade; skipped for `dev` builds.
_Avoid_: update, upgrade prompt, outdated badge

**Enable / Disable**:
A per-skill, per-harness state. Disabling writes that harness's own "off" setting (a config entry); the skill's files always stay in the canonical store.
_Avoid_: uninstall, hide, mute

**State file**:
Fleet's record of intended per-harness enablement. The single source of truth. Harness config files are outputs derived from it.
_Avoid_: config, lockfile

**Sync**:
Making each harness's config match the state file. Runs on every fleet command and after every wrapped `skills` CLI call. Fixing a mismatched config is part of sync — there is no separate repair step.
_Avoid_: reconcile, repair, apply

**Doctor**:
The read-only report of what's wrong: manual edits that disagree with the state file, redundant links, missing harness dirs. Doctor reports; Sync fixes.
_Avoid_: reconcile, audit

**Adopt**:
Move an existing custom skill from the canonical store into the adopt destination — `--into <skills-dir>` for the run, else the configured adopt target, else a terminal prompt over the tracked collections plus the always-offered fleet-home fallback (the fallback alone with no prompt when nothing is tracked) — then wire/link it. A one-time migration verb, also used when promoting a forked installed skill to custom.
_Avoid_: import, register, take over

**Drop**:
Remove a versioned customs home from the tracked set with `fleet skill drop <path-or-name>` — an explicit repo is unlisted from the explicit repo list with the disk untouched, a fleet-home checkout is deleted from disk. The inverse of pull, not of adopt: a one-time removal verb, never a migration.
_Avoid_: uninstall, unpull

**Redundant link**:
A per-agent directory symlink to a skill already reachable through the canonical store (e.g. `~/.config/opencode/skills/<name>`). Sync removes these.
_Avoid_: ghost link

**Docs site**:
The static Starlight site built from `www/` (output `www/dist/`), deployed on Vercel.
_Avoid_: www, website, docs
