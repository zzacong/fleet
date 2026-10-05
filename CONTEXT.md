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
A skill written by the user; no lockfile entry. Lives in one of the tracked dirs or the unversioned fleet-home fallback, and reaches every harness through a managed custom link, never through the canonical store.
_Avoid_: local skill, hand-written skill

**Managed custom link**:
The one symlink fleet creates per custom skill per harness, in that harness's own skills directory, pointing at the skill directory in its tracked dir. On a harness that links skills and scans the canonical store natively (OpenCode, Codex, Pi, Cursor, Bob) the link is also the custom skill's enable/disable lever. On Claude Code it is discovery only, because that harness does not scan the canonical store.
_Avoid_: wired path, collection path

**Tracked set**:
The ordered custom homes: the explicit collection-dir list from `~/.config/fleet/config.json` (list order is precedence order), then the unversioned fleet-home fallback. Every entry is an absolute directory the user registered; nothing is tracked by convention, and there is no env override.
_Avoid_: skills repo (singular), designated repo, versioned home

**Explicit dir list**:
The `skillsDirs` key in `~/.config/fleet/config.json`: absolute collection dirs, order significant. Managed by `fleet skill add-dir` (append once; never reorders) and `fleet skill remove-dir` (unlist), never hand-edited in the normal flow. Empty means none.
_Avoid_: repo pointer, skillsRepo (singular), skillsRepos

**Collection dir**:
An absolute directory whose immediate children are skill dirs, each with a `SKILL.md`. The tracked unit, registered with `fleet skill add-dir`. The old repo-root-plus-`skills/` derivation is gone; the tracked path is the collection itself.
_Avoid_: fleet repo, customs repo, repo skills

**Skill index**:
The precedence-ordered union of every skill source: the tracked set in order, then the canonical store. Keyed by skill directory (the identity the lockfile, links, and moves act on), with lookup by frontmatter name for harness-facing matching. Snapshot, doctor, and adopt all read through it instead of scanning homes themselves.
_Avoid_: skill list, source union

**Fleet home**:
`~/.config/fleet/` — fleet's own config dir. Holds `state.json` (enablement), `config.json` (machine-local: the explicit dir list plus the adopt target), and `skills/` (unversioned custom-skill fallback). `FLEET_HOME` overrides the home root.
_Avoid_: config dir, fleet dir

**Fleet config**:
`~/.config/fleet/config.json` — machine-local settings fleet reads on every command. Two keys: `skillsDirs` (array of absolute collection dirs, order significant, managed by `skill add-dir` and `skill remove-dir`) and `adoptTarget` (absolute adopt-target collection dir, absent means the fleet-home fallback). Both honor home expansion on set, absolute-path validation, atomic write, and unknown-field preservation. The retired `skillsRepos` key is preserved as an unknown and never read. Missing file means nothing is set; only fleet-home customs are scanned.
_Avoid_: settings, prefs

**Adopt target**:
The default adopt destination: an explicit collection dir (a tracked dir, or anywhere unscanned with a doctor warning). Set with `fleet config set adopt-target <skills-dir>`; absent means the fleet-home fallback. A run-scoped `--into <skills-dir>` overrides it for one adopt with no prompt and no config write. With neither, adopt prompts over the tracked dirs plus the always-offered fallback (no prompt when nothing is tracked), in a terminal only; piped runs fail with the candidate list and the flag hint.
_Avoid_: adopt home, default repo

**Project skill**:
A skill in the current checkout's `.agents/skills/` (e.g. `.agents/skills/watcher`). Per-project, committed with the project, discovered via the working directory. Fleet does not manage it; `fleet skill ls` does not list it.
_Avoid_: local skill, repo skill

**Outdated badge**:
The tri-state update marker per skill: outdated (the source repo's current tree hash differs from the lockfile's `skillFolderHash`), current, or unknown. Fleet checks GitHub directly — one call per source repo, cached under fleet's config dir — and reports unknown for customs (anything resolved through a tracked dir renders `—`, never checked by definition) and non-GitHub sources rather than guessing. The skills CLI is not involved (`check` there is an alias of `update`).
_Avoid_: stale marker, version check

**Fleet update notice**:
The notice that the running binary is older than the latest GitHub Release. Manual re-install, never auto-upgrade; skipped for `dev` builds.
_Avoid_: update, upgrade prompt, outdated badge

**Enable / Disable**:
A per-skill, per-harness state. The lever depends on the skill's source. On a harness that links skills and scans the canonical store natively (OpenCode, Codex, Pi, Cursor, Bob), a custom skill toggles by its managed custom link: off removes the link, on recreates it, and `ls` shows `absent` while it is missing. A canonical-store skill toggles by that harness's own config off-entry where one exists (OpenCode deny rule, Codex `[[skills.config]]`, Pi force-exclude) and is a no-op on Cursor and Bob. Claude Code always toggles by its `skillOverrides` config, for custom and canonical skills alike. An unversioned skill in the canonical store has no managed link, so it keeps the config lever even though `ls` marks it custom. The skill's files always stay where they are.
_Avoid_: uninstall, hide, mute

**State file**:
Fleet's record of intended per-harness enablement. The single source of truth. Harness config files are outputs derived from it.
_Avoid_: config, lockfile

**Sync**:
Making each harness's config match the state file, plus linking every tracked dir's and the fallback's skills into every installed harness, one managed custom link per skill. On a native-scanning harness it also removes the managed link of a custom the state disables, because there the link is the lever. On the first run after an upgrade it removes the legacy fleet-owned entries a previous release wrote, and it removes redundant store links. Runs on every fleet command and after every wrapped `skills` CLI call. Fixing a mismatched config is part of sync; there is no separate repair step.
_Avoid_: reconcile, repair, apply

**Doctor**:
The read-only report of what's wrong: manual edits that disagree with the state file, redundant links, missing harness dirs, a tracked dir that is missing, holds no skills, or is duplicated or nested against another tracked dir, and a custom skill's managed link disagreeing with the state on any harness that toggles customs by link (OpenCode, Codex, Pi, Cursor, Bob). Doctor reports; Sync fixes.
_Avoid_: reconcile, audit

**Adopt**:
Move an existing custom skill from the canonical store into the adopt destination — `--into <skills-dir>` for the run, else the configured adopt target, else a terminal prompt over the tracked dirs plus the always-offered fleet-home fallback (the fallback alone with no prompt when nothing is tracked) — then link it into every installed harness. A one-time migration verb, also used when promoting a forked installed skill to custom.
_Avoid_: import, register, take over

**Add-dir**:
Register an existing collection dir with `fleet skill add-dir <path>`, appending it to the explicit dir list and linking its skills into every installed harness. Validation is loud: the path must exist, be a directory of skills, and not duplicate, nest inside, or collide by name with a tracked dir. A collision with the canonical store is allowed and reported by doctor, because custom outranks canonical.
_Avoid_: pull, track, import

**Remove-dir**:
Unregister a collection dir with `fleet skill remove-dir <path>`, removing it from the explicit dir list and its managed links from every installed harness. The disk is never touched. The inverse of add-dir, not of adopt.
_Avoid_: pull, drop, unpull, uninstall

**Redundant link**:
A per-agent directory symlink to a skill already reachable through the canonical store (e.g. `~/.config/opencode/skills/<name>`). Sync removes these.
_Avoid_: ghost link

**Docs site**:
The static Starlight site built from `www/` (output `www/dist/`), deployed on Vercel.
_Avoid_: www, website, docs
