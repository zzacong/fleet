# 03: Custom skills — adopt, repo `skills/`, discovery wiring

**What to build:** Your hand-written skills move into this repo's `skills/` directory and stay discoverable everywhere. `fleet skill adopt <name>` migrates a custom skill from the canonical store into the repo; fleet wires the repo path into opencode and pi's native extra-path config and maintains symlinks for codex, claude code, Cursor, and Bob. `ls` marks them custom.

**Blocked by:** 02.

**Status:** ready-for-agent

- [ ] `fleet skill adopt <name>` moves `~/.agents/skills/<name>` into the repo's `skills/` directory and records it (custom = lives in the repo; no state-file ceremony needed beyond migration bookkeeping).
- [ ] Repo `skills/` path wired into opencode config (`skills.paths`) and pi global settings (`skills` array) via the existing adapter write machinery — idempotent, only added if missing.
- [ ] Managed symlinks: one per custom skill into `~/.codex/skills`, `~/.claude/skills`, `~/.cursor/skills`, `~/.bob/skills`, so customs are visible in all six harnesses.
- [ ] Custom skills are NOT symlinked into `~/.agents/skills` (would double-visibility for opencode/pi and cause collisions).
- [ ] `fleet skill ls` shows adopted skills as custom with no source repo.
- [ ] Adoption is reversible by hand (move the dir back) without breaking fleet; doctor flags the inconsistency rather than erroring.
- [ ] Integration test: fake home + fake repo root; adopt, then verify every harness's config/link state; verify the moved skill still parses as a skill.
