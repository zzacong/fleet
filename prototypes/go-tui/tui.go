package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ---- styles, declared once as package vars ----

var (
	styEnabled  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styDisabled = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styStaged   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styDim      = lipgloss.NewStyle().Faint(true)
	styBold     = lipgloss.NewStyle().Bold(true)
	stySelected = lipgloss.NewStyle().Reverse(true)
	styGood     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styHelpBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
)

// ---- model ----

type phase int

const (
	phaseIdle phase = iota
	phaseApplying
	phaseRefreshing
)

type model struct {
	rows        []skill
	filter      textinput.Model
	filterFocus bool
	cursor      int
	offset      int // first visible row index (scroll window)
	staged      map[string]bool
	phase       phase
	spinner     spinner.Model
	helpOpen    bool
	notice      string
	preview     string
	seq         int // guards against late apply timers after esc-cancel
	width       int
	height      int
	bench       bool
	start       time.Time
}

type applyDoneMsg struct{ seq, count int }
type refreshedMsg struct{}
type benchMsg struct{}

func newModel(rows []skill, bench bool) model {
	ti := textinput.New()
	ti.Placeholder = "filter"
	ti.SetWidth(30)

	sp := spinner.New()
	sp.Spinner = spinner.Line
	sp.Style = lipgloss.NewStyle()

	m := model{
		rows:    rows,
		filter:  ti,
		staged:  map[string]bool{},
		spinner: sp,
		width:   80,
		height:  24,
		bench:   bench,
		start:   time.Now(),
	}
	return m
}

func (m model) Init() tea.Cmd {
	if m.bench {
		return func() tea.Msg { return benchMsg{} }
	}
	return nil
}

// ---- messages / helpers ----

func (m model) filtered() []skill {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == "" {
		return m.rows
	}
	out := make([]skill, 0, len(m.rows))
	for _, s := range m.rows {
		if strings.Contains(strings.ToLower(s.Name), q) ||
			strings.Contains(strings.ToLower(s.Description), q) {
			out = append(out, s)
		}
	}
	return out
}

func (m model) visibleRows() int {
	return max(1, m.height-4) // status, filter, notice, footer
}

func (m *model) move(d int) {
	n := len(m.filtered())
	if n == 0 {
		return
	}
	m.cursor += d
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > n-1 {
		m.cursor = n - 1
	}
	m.scrollToCursor()
}

func (m *model) scrollToCursor() {
	v := m.visibleRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+v {
		m.offset = m.cursor - v + 1
	}
}

// syncSelection keeps selection on the same skill after the filter changed.
func (m *model) syncSelection(prevName string) {
	fs := m.filtered()
	for i, s := range fs {
		if s.Name == prevName {
			m.cursor = i
			m.scrollToCursor()
			return
		}
	}
	m.cursor = min(m.cursor, max(0, len(fs)-1))
	m.scrollToCursor()
}

func (m *model) applyStaged(count int) {
	for name := range m.staged {
		for i := range m.rows {
			if m.rows[i].Name == name {
				m.rows[i].Enabled = !m.rows[i].Enabled
			}
		}
	}
	m.staged = map[string]bool{}
	m.notice = fmt.Sprintf("applied %d change(s)", count)
}

// ---- update ----

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scrollToCursor()
		return m, nil

	case spinner.TickMsg:
		if m.phase == phaseApplying || m.phase == phaseRefreshing {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case applyDoneMsg:
		if msg.seq == m.seq && m.phase == phaseApplying {
			m.applyStaged(msg.count)
			m.phase = phaseIdle
			m.preview = ""
		}
		return m, nil

	case refreshedMsg:
		m.rows = expandRows(len(m.rows)) // reload at the same stress size
		m.phase = phaseIdle
		m.notice = "fixture reloaded"
		return m, nil

	case benchMsg:
		fmt.Fprintf(os.Stderr, "startup→first frame: %dms\n", time.Since(m.start).Milliseconds())
		return m, tea.Quit

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.notice = ""
	if m.helpOpen {
		m.helpOpen = false
		return m, nil
	}

	key := msg.String()

	if m.filterFocus {
		switch key {
		case "esc":
			prev := m.selectedName()
			m.filter.Reset()
			m.filterFocus = false
			m.syncSelection(prev)
			return m, nil
		case "enter":
			m.filterFocus = false
			return m, nil
		case "q":
			// quit only while focused and empty; otherwise it's text
			if m.filter.Value() == "" {
				return m, tea.Quit
			}
		}
		prev := m.selectedName()
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.syncSelection(prev)
		return m, cmd
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "/":
		m.filterFocus = true
		return m, m.filter.Focus()
	case " ", "space":
		if name := m.selectedName(); name != "" {
			if m.staged[name] {
				delete(m.staged, name)
			} else {
				m.staged[name] = true
			}
		}
	case "enter":
		if len(m.staged) == 0 {
			m.notice = "nothing staged"
			return m, nil
		}
		m.seq++
		m.phase = phaseApplying
		count := len(m.staged)
		m.preview = fmt.Sprintf("applying %d change(s): %s", count, m.previewList())
		seq := m.seq
		return m, tea.Batch(
			m.spinner.Tick,
			tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
				return applyDoneMsg{seq: seq, count: count}
			}),
		)
	case "r":
		m.seq++
		m.phase = phaseRefreshing
		m.notice = ""
		return m, tea.Batch(
			m.spinner.Tick,
			tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg {
				return refreshedMsg{}
			}),
		)
	case "?":
		m.helpOpen = true
	case "esc":
		if m.phase == phaseApplying {
			m.seq++ // cancel: the pending applyDoneMsg will be ignored
			m.phase = phaseIdle
			m.preview = ""
			m.notice = "apply cancelled"
			return m, nil
		}
		if m.filter.Value() != "" {
			prev := m.selectedName()
			m.filter.Reset()
			m.syncSelection(prev)
		}
	}
	return m, nil
}

func (m model) selectedName() string {
	fs := m.filtered()
	if m.cursor >= 0 && m.cursor < len(fs) {
		return fs[m.cursor].Name
	}
	return ""
}

func (m model) previewList() string {
	names := make([]string, 0, len(m.staged))
	byName := map[string]skill{}
	for _, s := range m.rows {
		byName[s.Name] = s
	}
	for name := range m.staged {
		if s, ok := byName[name]; ok && s.Enabled {
			names = append(names, "-"+name)
		} else {
			names = append(names, "+"+name)
		}
	}
	return strings.Join(names, " ")
}

// ---- view ----

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

// renderRow draws one body row at the given width. Also used by the CLI
// `list` table. With stagedMark "" the marker column renders as a space.
func renderRow(s skill, width, nameW int, selected, staged bool) string {
	if width <= 0 {
		width = 80
	}
	if nameW <= 0 {
		nameW = 24
	}
	glyph, glyphSty := "○", styDisabled
	if s.Enabled {
		glyph, glyphSty = "●", styEnabled
	}
	mark := " "
	if staged {
		mark = "*"
	}
	name := truncate(s.Name, nameW)
	name = name + strings.Repeat(" ", max(0, nameW-len([]rune(name))))

	rootsW := 24
	rootsCol := truncate(strings.Join(s.Roots, ", "), rootsW)
	descW := width - 4 - nameW - 1 - len([]rune(rootsCol))
	if descW < 8 {
		rootsCol = truncate(strings.Join(s.Roots, ", "), max(4, width-nameW-20))
		descW = width - 4 - nameW - 1 - len([]rune(rootsCol))
	}
	descCol := truncate(s.Description, descW)
	pad := strings.Repeat(" ", max(0, width-4-nameW-1-len([]rune(rootsCol))-len([]rune(descCol))))

	line := glyphSty.Render(glyph) + styStaged.Render(mark) + " " +
		name + " " + descCol + pad + " " + styDim.Render(rootsCol)
	if selected {
		line = stySelected.Width(width).Render(line)
	}
	return line
}

func (m model) statusLine() string {
	enabled, disabled := 0, 0
	for _, s := range m.rows {
		if s.Enabled {
			enabled++
		} else {
			disabled++
		}
	}
	line := styBold.Render("skillctl") + styDim.Render(" · ") +
		fmt.Sprintf("%d skills · %d enabled · %d disabled", len(m.rows), enabled, disabled)
	if n := len(m.staged); n > 0 {
		line += styDim.Render(" · ") + styStaged.Render(fmt.Sprintf("%d staged", n))
	}
	return line
}

func (m model) filterLine() string {
	prefix := styDim.Render("/")
	if !m.filterFocus && m.filter.Value() == "" {
		return prefix + " " + styDim.Render(m.filter.Placeholder)
	}
	return prefix + " " + m.filter.View()
}

func (m model) noticeLine() string {
	switch m.phase {
	case phaseApplying:
		return m.spinner.View() + " " + m.preview
	case phaseRefreshing:
		return m.spinner.View() + " refreshing fixture…"
	}
	if m.notice != "" {
		return styGood.Render(m.notice)
	}
	return ""
}

func (m model) footer() string {
	return styDim.Render("space toggle   enter apply staged   / filter   r refresh   ? help   q quit")
}

func (m model) helpOverlay() string {
	rows := []string{
		"keybindings",
		"",
		"j / k, ↑ / ↓   move selection",
		"/             focus filter",
		"esc           clear filter / cancel apply",
		"space         stage/unstage toggle",
		"enter         apply staged changes",
		"r             refresh fixture",
		"?             toggle this help",
		"q             quit (ignored while filter non-empty)",
	}
	box := styHelpBox.Render(strings.Join(rows, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m model) View() tea.View {
	if m.helpOpen {
		v := tea.NewView(m.helpOverlay())
		v.AltScreen = true
		return v
	}

	var b strings.Builder
	b.WriteString(m.statusLine() + "\n")
	b.WriteString(m.filterLine() + "\n")

	fs := m.filtered()
	nameW := 8
	for _, s := range fs {
		if n := len([]rune(s.Name)); n > nameW && n <= 30 {
			nameW = n
		}
	}
	nameW = min(nameW, 30)

	v := m.visibleRows()
	for i := m.offset; i < m.offset+v && i < len(fs); i++ {
		s := fs[i]
		b.WriteString(renderRow(s, m.width, nameW, i == m.cursor, m.staged[s.Name]))
		b.WriteString("\n")
	}
	// keep the notice/footer lines pinned to the bottom
	for i := len(fs) - m.offset; i < v; i++ {
		b.WriteString("\n")
	}
	if n := m.noticeLine(); n != "" {
		b.WriteString(truncate(n, m.width) + "\n")
	} else {
		b.WriteString("\n")
	}
	b.WriteString(m.footer())

	out := tea.NewView(b.String())
	out.AltScreen = true
	return out
}

// runTui builds the model and runs the program. rows is already expanded.
func runTui(rows []skill, bench bool) error {
	p := tea.NewProgram(newModel(rows, bench))
	_, err := p.Run()
	return err
}
