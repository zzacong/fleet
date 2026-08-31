# 07: TUI

**What to build:** Bare `fleet` opens the interface: a skill × harness matrix with staged toggles, filter, hero banner, and the keybindings from the prototype UX spec. All writes go through the same state + sync machinery as the CLI verbs — the TUI is a face on the core, not a second brain.

**Blocked by:** 02, 04, 06.

**Status:** ready-for-agent

- [ ] Bubble Tea v2 + Lipgloss v2 matrix view: one row per skill (name, custom/installed, source repo, description, update badge), one column per installed harness; only detected harnesses shown.
- [ ] Staged toggling: space stages, enter applies (through state file + sync), esc cancels; staged rows marked per the prototype spec (`●`/`○` glyphs, yellow `*` staged mark, inverted selected row).
- [ ] `/` filter, j/k/↑/↓ navigation, `r` refresh, `?` help overlay, `q` quit.
- [ ] Hero ASCII banner on TUI launch only; suppressed when piped or `--quiet`.
- [ ] Rows grouped custom-first, then installed by source repo; update-available badge from ticket 06.
- [ ] UI notes that some harnesses pick up changes next session (opencode has no live reload).
- [ ] Cursor shows its disable limitation inline (no per-skill off switch) rather than a silent no-op.
- [ ] Status bar with counts (total, custom, installed, outdated, staged).
- [ ] All TUI state transitions covered by tests through the core (stage → apply → sync), not by UI snapshotting.
