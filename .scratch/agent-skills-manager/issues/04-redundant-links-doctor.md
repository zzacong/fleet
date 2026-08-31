# 04: Redundant link cleanup + doctor

**What to build:** The skills CLI's symlink spam stops accumulating, and fleet gets its read-only checkup command. Sync removes per-agent links that double-cover canonical-store skills; `doctor` reports everything wrong — drift, manual-edit conflicts, broken links, unknown entries — and asks keep-vs-restore on manual edits. Nothing is ever silently overwritten or deleted.

**Blocked by:** 02.

**Status:** resolved

- [x] Sync removes redundant per-agent symlinks for harnesses that natively scan the canonical store: `~/.config/opencode/skills/*`, `~/.codex/skills/*`, `~/.cursor/skills/*`, `~/.pi/agent/skills/*`.
- [x] `~/.bob/skills` links to canonical-store skills are **removed** (user decision: Bob reads `~/.agents/skills` natively, so the skills CLI's auto-symlinks there are redundant double-coverage). Managed custom-skill symlinks pointing at the repo are never touched — they are Bob's only path to customs.
- [x] `fleet skill doctor` reports: redundant links (with what/why), broken symlinks, skills in agent dirs unknown to fleet, manual config edits that disagree with the state file (offering keep-my-change vs restore), missing harness dirs.
- [x] Manual-edit resolution is a prompt, not vocabulary: "keep my change" adopts it into the state file; "restore" syncs it back. Unknown entries are reported, never deleted.
- [x] Link cleanup is idempotent and safe when a link's target is missing.
- [x] Integration tests: recreate a redundant link by hand → sync removes it; hand-edit a config → doctor flags it; both paths green in a fake home.

## Comments

Implementation notes (agent, 2026-09-01):

- Classification lives in `internal/harness/links.go` (`SkillDirs`, `ScanSkillDir`): safe by construction — only symlinks that provably resolve into the canonical store (lexically or through evaluated paths) in a native scanner are redundant. Symlinks pointing anywhere else (repo custom links from ticket 03, user links, the store dir itself), real dirs, and files are foreign/untracked and never touched. Broken links are doctor's business; sync removes only the redundant class, including links whose target is missing.
- Doctor (`internal/doctor`, `fleet skill doctor`) runs **no ambient sync** — that is the only way it can report what sync would change before sync changes it (story 27). Manual-edit disagreements come from a new read-side seam: `ReadResult.Disables` (exact-name disable entries per harness config, including names not in the store). Both disagreement directions are prompted: keep records the config's disable in state, or adopts a hand-deleted fleet marker as enabled; restore re-projects through `Project`. Patterns/blankets are findings, never prompts (state can't record them, fleet can't remove them).
- "Missing harness dirs" covers the canonical store and the skills dir of harnesses that discover only through links (claude today). Native scanners don't need their per-agent skills dir — its absence is the goal, not a problem.
- Claude's enable path now removes a stale `off` override for a skill it can no longer discover, so a restore always converges the config instead of silently doing nothing.
