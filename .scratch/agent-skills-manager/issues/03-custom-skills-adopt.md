# 03: Custom skills — adopt, repo `skills/`, discovery wiring

**What to build:** Your hand-written skills move into this repo's `skills/` directory and stay discoverable everywhere. `fleet skill adopt <name>` migrates a custom skill from the canonical store into the repo; fleet wires the repo path into opencode and pi's native extra-path config and maintains symlinks for codex, claude code, Cursor, and Bob. `ls` marks them custom.

**Blocked by:** 02.

**Status:** resolved

- [x] `fleet skill adopt <name>` moves `~/.agents/skills/<name>` into the repo's `skills/` directory and records it (custom = lives in the repo; no state-file ceremony needed beyond migration bookkeeping).
- [x] Repo `skills/` path wired into opencode config (`skills.paths`) and pi global settings (`skills` array) via the existing adapter write machinery — idempotent, only added if missing.
- [x] Managed symlinks: one per custom skill into `~/.codex/skills`, `~/.claude/skills`, `~/.cursor/skills`, `~/.bob/skills`, so customs are visible in all six harnesses.
- [x] Custom skills are NOT symlinked into `~/.agents/skills` (would double-visibility for opencode/pi and cause collisions).
- [x] `fleet skill ls` shows adopted skills as custom with no source repo.
- [x] Adoption is reversible by hand (move the dir back) without breaking fleet; doctor flags the inconsistency rather than erroring.
- [x] Integration test: fake home + fake repo root; adopt, then verify every harness's config/link state; verify the moved skill still parses as a skill.

**Notes for later tickets:**

- Repo root resolution: `FLEET_REPO` env override, else a walk up from the working directory to the nearest `.git` (file or dir — worktrees included); `paths.RepoSkills()` is empty outside a repo and adopt says so.
- Managed links always target the repo (never `~/.agents/skills`), so ticket 04's cleanup can identify them by target alone. Adopt repoints existing per-agent symlinks (e.g. the skills CLI's links to the canonical store) at the repo, and skips real directories with a note.
- opencode wiring follows the `skills` key's existing shape (object → `paths`, array → append); with no `skills` key, a V2-marker file gets the flat array, everything else the V1 object (the permission writes' safe default).
- Doctor (04) should flag: a skill name present in both the store and the repo (ls reports the store copy once), broken managed links after a hand-reversal, and stale lockfile entries for adopted skills (ls deliberately ignores them, and the update check skips them).
