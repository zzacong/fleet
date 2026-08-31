# Fleet

Fleet manages agent skills across AI coding agents (harnesses): enable or disable a skill per harness without uninstalling it, while the `skills` CLI stays the install/update backend.

## Language

**Harness**:
An AI coding agent that discovers and loads skills. Fleet targets: opencode, pi, codex, claude code, IBM Bob, Cursor. A harness is detected by its config directory on this machine; the UI only shows harnesses that are installed.
_Avoid_: agent, client, platform

**Skill**:
A directory with a `SKILL.md` file describing an agent capability. The unit that gets enabled or disabled.
_Avoid_: plugin, extension

**Canonical store**:
`~/.agents/skills` — where the `skills` CLI installs and updates skills. codex, opencode, pi and Bob scan it directly. Fleet never moves or renames anything in it.
_Avoid_: skills dir, install dir

**Installed skill**:
A skill with provenance (source repo, hash) in the `skills` CLI lockfile (`~/.agents/.skill-lock.json`). Grouped by source repo.
_Avoid_: managed skill

**Custom skill**:
A skill written by the user; no lockfile entry. Lives in the fleet repo's `skills/` directory and is discovered directly by harnesses.
_Avoid_: local skill, hand-written skill

**Outdated badge**:
The tri-state update marker per skill: outdated (the source repo's current tree hash differs from the lockfile's `skillFolderHash`), current, or unknown. Fleet checks GitHub directly — one call per source repo, cached under fleet's config dir — and reports unknown for custom skills and non-GitHub sources rather than guessing. The skills CLI is not involved (`check` there is an alias of `update`).
_Avoid_: stale marker, version check

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
Move an existing custom skill from the canonical store into the fleet repo's `skills/` directory and record it. A one-time migration verb, also used when promoting a forked installed skill to custom.
_Avoid_: import, register, take over

**Redundant link**:
A per-agent directory symlink to a skill already reachable through the canonical store (e.g. `~/.config/opencode/skills/<name>`). Sync removes these.
_Avoid_: ghost link
