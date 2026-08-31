# 04: Redundant link cleanup + doctor

**What to build:** The skills CLI's symlink spam stops accumulating, and fleet gets its read-only checkup command. Sync removes per-agent links that double-cover canonical-store skills; `doctor` reports everything wrong — drift, manual-edit conflicts, broken links, unknown entries — and asks keep-vs-restore on manual edits. Nothing is ever silently overwritten or deleted.

**Blocked by:** 02.

**Status:** ready-for-agent

- [ ] Sync removes redundant per-agent symlinks for harnesses that natively scan the canonical store: `~/.config/opencode/skills/*`, `~/.codex/skills/*`, `~/.cursor/skills/*`, `~/.pi/agent/skills/*`.
- [ ] `~/.bob/skills` links to canonical-store skills are **removed** (user decision: Bob reads `~/.agents/skills` natively, so the skills CLI's auto-symlinks there are redundant double-coverage). Managed custom-skill symlinks pointing at the repo are never touched — they are Bob's only path to customs.
- [ ] `fleet skill doctor` reports: redundant links (with what/why), broken symlinks, skills in agent dirs unknown to fleet, manual config edits that disagree with the state file (offering keep-my-change vs restore), missing harness dirs.
- [ ] Manual-edit resolution is a prompt, not vocabulary: "keep my change" adopts it into the state file; "restore" syncs it back. Unknown entries are reported, never deleted.
- [ ] Link cleanup is idempotent and safe when a link's target is missing.
- [ ] Integration tests: recreate a redundant link by hand → sync removes it; hand-edit a config → doctor flags it; both paths green in a fake home.
