# 07: TUI

**What to build:** Bare `fleet` opens the interface: a skill × harness matrix with staged toggles, filter, hero banner, and the keybindings from the prototype UX spec. All writes go through the same state + sync machinery as the CLI verbs — the TUI is a face on the core, not a second brain.

**Blocked by:** 02, 04, 06.

**Status:** resolved

- [x] Bubble Tea v2 + Lipgloss v2 matrix view: one row per skill (name, custom/installed, source repo, description, update badge), one column per installed harness; only detected harnesses shown.
- [x] Staged toggling: space stages, enter applies (through state file + sync), esc cancels; staged rows marked per the prototype spec (`●`/`○` glyphs, yellow `*` staged mark, inverted selected row).
- [x] `/` filter, j/k/↑/↓ navigation, `r` refresh, `?` help overlay, `q` quit.
- [x] Hero ASCII banner on TUI launch only; suppressed when piped or `--quiet`.
- [x] Rows grouped custom-first, then installed by source repo; update-available badge from ticket 06.
- [x] UI notes that some harnesses pick up changes next session (opencode has no live reload).
- [x] Cursor shows its disable limitation inline (no per-skill off switch) rather than a silent no-op.
- [x] Status bar with counts (total, custom, installed, outdated, staged).
- [x] All TUI state transitions covered by tests through the core (stage → apply → sync), not by UI snapshotting.

## Comments

### Implementation notes (agent, 2026-09-01)

- **One brain, extracted not duplicated.** The read-side report builder moved from `internal/cli/ls.go` into `internal/snapshot` — the ls verb and the TUI render the same snapshot. The write sequence (record in the state file, project "on" markers directly so ambient sync doesn't flag them, then sync) moved into `internal/toggle`; `fleet skill on/off` and the TUI's enter key both call `toggle.Apply`. Neither face keeps its own copy of the logic.
- **Per-cell staging.** The prototype staged a row-wide toggle; the real system is a matrix, so space stages the selected cell (skill × harness) — `←/→`/`h/l` move the column cursor, documented in the help overlay. The leading row glyph shows the selected cell's state (the thing space flips), the yellow `*` marks a staged row, and staged cells render their target state in yellow.
- **Origin/source as group headers.** Rather than adding a source column (the matrix already carries name, description, badge, and up to six harness cells), custom/installed and the source repo render as dim group-header lines between row groups; they follow the filtered view.
- **Blocked toggles say so.** Space on a Cursor/Bob cell explains there is no per-skill disable mechanism (same wording as the CLI verbs' no-op message); space on a cell a harness cannot discover (claude without its link) says the skill isn't discoverable. Neither stages anything.
- **Piped bare `fleet` falls back to the `skill ls` listing** — a matrix nobody can steer is not a face, and this keeps the banner a TUI-launch thing only. `--quiet` keeps the TUI but drops the banner.
- **No spinner.** The prototype's 300 ms apply spinner simulated work; the real apply is synchronous and the reload that follows shows a `refreshing…` line. Applies confirm immediately with the next-session note.
- **v1-idiom guard.** `TestNoV1CharmImports` fails the suite if any `github.com/charmbracelet/*` (v1) import path appears; only `charm.land/*/v2` is allowed.
