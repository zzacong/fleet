// The view: banner (unless quiet), status bar, filter line, the skill ×
// harness matrix with group headers, a notice line, and the key hints.
// Rendering only — every piece of data comes from the model, which gets it
// from the core.
package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/snapshot"
)

// layout is the matrix's column math, computed once per frame so rows and
// the column header stay aligned.
type layout struct {
	width int
	nameW int
	descW int
	cellW []int // per-harness column widths
}

const (
	minCellW = 3  // state columns never narrow below one padded glyph
	descCap  = 60 // the description yields long before the screen edge
	cellGap  = "  "
)

// harnessAbbrev maps harness names to the short labels the matrix header
// renders. Names are state keys everywhere else — this only shrinks the
// column header, so ten-plus columns still fit a normal terminal. A
// harness without an entry renders its full name.
var harnessAbbrev = map[string]string{
	"opencode": "oc",
	"codex":    "cx",
	"claude":   "cl",
	"cursor":   "cu",
}

// harnessLabel is the header label for one harness.
func harnessLabel(h string) string {
	if a, ok := harnessAbbrev[h]; ok {
		return a
	}
	return h
}

// harnessLegend renders the abbreviation key for the help overlay, sorted
// so the line is stable between frames.
func harnessLegend() string {
	names := make([]string, 0, len(harnessAbbrev))
	for name := range harnessAbbrev {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, harnessAbbrev[name]+" = "+name)
	}
	return strings.Join(parts, " · ")
}

func (m model) layout() layout {
	l := layout{width: max(m.width, 40)}

	l.nameW = 8
	for _, r := range m.filtered() {
		if n := runeLen(r.Name); n > l.nameW && n <= 30 {
			l.nameW = n
		}
	}

	for _, h := range m.harnesses {
		label := harnessLabel(h)
		if !m.writable[h] {
			label += "!" // flagged: no per-skill off switch, see the help
		}
		l.cellW = append(l.cellW, max(runeLen(label), minCellW))
	}

	// glyph + mark + space, name + space, badge + two spaces, cells joined
	// by two-space gaps. The description gets whatever is left — capped so
	// the cells stay near the names on wide terminals — and is dropped
	// entirely on narrow terminals: the cells matter more.
	used := 3 + l.nameW + 1 + 3
	for _, w := range l.cellW {
		used += w + runeLen(cellGap)
	}
	l.descW = max(0, l.width-used-1)
	l.descW = min(l.descW, descCap)
	if l.descW < 8 {
		l.descW = 0
	}
	return l
}

// visibleRows is the body's row budget: everything else is fixed chrome.
func (m model) visibleRows() int {
	fixed := 5 // status, filter, column header, notice, footer
	if !m.quiet {
		fixed += len(bannerLines())
	}
	return max(1, m.height-fixed)
}

func (m model) View() tea.View {
	if m.helpOpen {
		v := tea.NewView(m.helpOverlay())
		v.AltScreen = true
		return v
	}

	var b strings.Builder
	if !m.quiet {
		for _, line := range bannerLines() {
			b.WriteString(line + "\n")
		}
	}
	b.WriteString(m.statusLine() + "\n")
	b.WriteString(m.filterLine() + "\n")
	b.WriteString(m.columnHeaderLine() + "\n")

	// The body: filtered rows in snapshot order (custom first, then by
	// source repo), a dim header line whenever the group changes. Headers
	// spend the same line budget the scroll math accounts for (endFrom),
	// so the cursor row always renders.
	fs := m.filtered()
	budget := m.visibleRows()
	lines := 0
	prev := ""
	for i := m.offset; i < m.endFrom(m.offset); i++ {
		g := groupOf(fs[i])
		if g != prev && lines+2 <= budget {
			b.WriteString(groupHeader(g) + "\n")
			lines++
			prev = g
		}
		b.WriteString(m.renderRow(fs[i], i == m.row) + "\n")
		lines++
	}
	for ; lines < budget; lines++ {
		b.WriteString("\n")
	}

	b.WriteString(m.noticeLine() + "\n")
	b.WriteString(m.footer())

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// endFrom returns the first row index at or after start that does not fit
// in the body's line budget, counting group headers. The view and the
// scroll math share it, so a cursor the scroll math considers visible is
// one the view actually renders.
func (m model) endFrom(start int) int {
	fs := m.filtered()
	budget := m.visibleRows()
	lines, prev := 0, ""
	for i := start; i < len(fs); i++ {
		g := groupOf(fs[i])
		need := 1
		if g != prev && lines+2 <= budget {
			need = 2 // the header line and the row
		}
		if lines+need > budget {
			return i
		}
		lines += need
		if need == 2 {
			prev = g
		}
	}
	return len(fs)
}

// groupOf names the section a row renders under.
func groupOf(r snapshot.SkillRow) string {
	switch {
	case r.Custom:
		return "custom"
	case r.Source == "":
		return "installed"
	default:
		return r.Source
	}
}

func groupHeader(group string) string {
	return styDim.Render("· " + group)
}

// statusLine: total, installed, custom, outdated — plus the staged count
// when something is staged, per the prototype spec.
func (m model) statusLine() string {
	installed, custom, outdated := 0, 0, 0
	for _, r := range m.rows {
		if r.Custom {
			custom++
		} else {
			installed++
		}
		if r.Outdated != nil && *r.Outdated {
			outdated++
		}
	}
	line := styBold.Render("fleet") + styDim.Render(" · ") +
		fmt.Sprintf("%d skills · %d installed · %d custom · %d outdated", len(m.rows), installed, custom, outdated)
	if n := len(m.staged); n > 0 {
		line += styDim.Render(" · ") + styStaged.Render(fmt.Sprintf("%d staged", n))
	}
	return line
}

func (m model) filterLine() string {
	prefix := styDim.Render("/")
	if !m.filterFocus && m.filter.Value() == "" {
		return prefix + " " + styDim.Render("filter")
	}
	return prefix + " " + m.filter.View()
}

// columnHeaderLine labels the harness columns. The ! marks harnesses
// without a per-skill off switch (Cursor, Bob); long names render
// abbreviated, keyed in the help overlay.
func (m model) columnHeaderLine() string {
	l := m.layout()
	prefix := 3 + l.nameW + 1 + 3 // glyph, mark, space; name, space; badge, two spaces
	if l.descW > 0 {
		prefix += l.descW + 1
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", prefix))
	for i, h := range m.harnesses {
		if i > 0 {
			b.WriteString(cellGap)
		}
		label := harnessLabel(h)
		sty := styDim
		if !m.writable[h] {
			label += "!"
			sty = styNowrite
		}
		if i == m.col {
			sty = sty.Bold(true).Faint(false) // the column ←/→ moves to
		}
		b.WriteString(sty.Render(pad(label, l.cellW[i])))
	}
	return b.String()
}

// renderRow draws one matrix row. The leading glyph is the selected
// harness cell's current state — the thing space flips; a yellow *
// after it marks the row as holding staged changes. Staged cells show
// their target state in yellow.
func (m model) renderRow(r snapshot.SkillRow, selected bool) string {
	l := m.layout()

	glyph, glyphSty := "·", styDim
	if len(m.harnesses) > 0 {
		switch m.cellState(r.Name, m.harnesses[m.col]) {
		case harness.StateOn:
			glyph, glyphSty = "●", styOn
		case harness.StateOff:
			glyph, glyphSty = "○", styOff
		default:
			glyph, glyphSty = "-", styAbsent
		}
	}

	mark := " "
	if m.rowStaged(r.Name) {
		mark = styStaged.Render("*")
	}

	line := glyphSty.Render(glyph) + mark + " " +
		pad(truncate(r.Name, l.nameW), l.nameW) + " "

	if l.descW > 0 {
		line += pad(truncate(r.Description, l.descW), l.descW) + " "
	}

	switch {
	case r.Outdated == nil:
		line += styDim.Render("?")
	case *r.Outdated:
		line += styStaged.Render("↑")
	default:
		line += styGood.Render("✓")
	}
	line += "  " + m.renderCells(r, l)

	if selected {
		line = stySelected.Width(l.width).Render(line)
	}
	return line
}

// renderCells draws one row's harness cells, left to right in column
// order: the current state, or the staged target in yellow. The selected
// column — the one ←/→ moves and space flips — renders reversed so the
// cursor is always visible. Cells in a column without a write side render
// faint — fleet cannot change them.
func (m model) renderCells(r snapshot.SkillRow, l layout) string {
	var b strings.Builder
	for i, h := range m.harnesses {
		if i > 0 {
			b.WriteString(cellGap)
		}
		var label string
		var sty lipgloss.Style
		switch {
		case m.hasStaged(r.Name, h):
			glyph := "○"
			if m.staged[cellKey{skill: r.Name, harness: h}] {
				glyph = "●"
			}
			label, sty = glyph, styStaged
		case !m.writable[h]:
			label, sty = "●", styNowrite
		default:
			switch m.cellState(r.Name, h) {
			case harness.StateOn:
				label, sty = "●", styOn
			case harness.StateOff:
				label, sty = "○", styOff
			default:
				label, sty = "-", styAbsent
			}
		}
		if i == m.col {
			sty = sty.Reverse(true)
		}
		b.WriteString(sty.Render(center(label, l.cellW[i])))
	}
	return b.String()
}

// rowStaged reports whether the row holds any staged change.
func (m model) rowStaged(name string) bool {
	for k := range m.staged {
		if k.skill == name {
			return true
		}
	}
	return false
}

// hasStaged reports whether one cell is staged.
func (m model) hasStaged(skill, harnessName string) bool {
	_, ok := m.staged[cellKey{skill: skill, harness: harnessName}]
	return ok
}

func (m model) noticeLine() string {
	if m.phase == phaseUpdating {
		return styDim.Render("updating all skills…")
	}
	if m.phase == phaseRefreshing {
		return styDim.Render("refreshing…")
	}
	if m.notice.text == "" {
		return ""
	}
	text := truncate(m.notice.text, m.width)
	switch m.notice.kind {
	case noticeErr:
		return styErr.Render(text)
	case noticeGood:
		return styGood.Render(text)
	default:
		return styStaged.Render(text)
	}
}

func (m model) footer() string {
	text := "space toggle   ←/→ harness   enter apply staged   u update all   / filter   r refresh   ? help   q quit"
	return styDim.Render(truncate(text, m.width))
}

func (m model) helpOverlay() string {
	rows := []string{
		"keybindings",
		"",
		"j / k, ↑ / ↓   move selection",
		"h / l, ← / →   move harness column",
		"space          stage/unstage the selected cell",
		"enter          apply staged changes (state file, then sync)",
		"u              update all installed skills (skills update, then sync)",
		"esc            discard staged, clear filter, or close this help",
		"/              focus filter",
		"r              refresh",
		"?              toggle this help",
		"q              quit (ignored while the filter holds text)",
		"",
		"columns",
		"",
		harnessLegend(),
		"",
		"notes",
		"",
		"cursor and bob have no per-skill off switch — they read the",
		"canonical store natively, so toggles for them are no-ops.",
		"some harnesses pick up config changes on their next session",
		"(opencode has no live reload).",
	}
	box := styHelpBox.Render(strings.Join(rows, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// ---- text helpers ----

func runeLen(s string) int { return len([]rune(s)) }

// pad right-pads s with spaces to w runes.
func pad(s string, w int) string {
	if n := runeLen(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// center pads s with spaces on both sides so a glyph sits mid-column.
func center(s string, w int) string {
	n := runeLen(s)
	if n >= w {
		return s
	}
	left := (w - n) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", w-n-left)
}

// truncate shortens s to at most w runes, marking the cut with an ellipsis.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}
