// Package tui is fleet's interactive face: a skill × harness matrix over
// the same core the CLI verbs drive. Staged toggles apply through the
// state file and sync (internal/toggle — the exact code `fleet skill
// on/off` runs); `u` runs the wrapped update-all (internal/skillscli then
// sync — the exact code `fleet skill update` runs); the matrix renders a
// snapshot (internal/snapshot — the exact code `fleet skill ls` renders).
// Nothing here reads or writes harness configs directly: it is a face on
// the core, not a second brain.
package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zacong/fleet/internal/harness"
	"github.com/zacong/fleet/internal/paths"
	"github.com/zacong/fleet/internal/snapshot"
	fleetsync "github.com/zacong/fleet/internal/sync"
	"github.com/zacong/fleet/internal/toggle"
)

// ---- styles, declared once as package vars ----

var (
	styOn       = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styOff      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styStaged   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styAbsent   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styNowrite  = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true)
	styDim      = lipgloss.NewStyle().Faint(true)
	styBold     = lipgloss.NewStyle().Bold(true)
	stySelected = lipgloss.NewStyle().Reverse(true)
	styErr      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styGood     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styHelpBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
)

// ---- model ----

type phase int

const (
	phaseIdle phase = iota
	phaseRefreshing
	phaseUpdating
)

// cellKey is one staged change: a skill in one harness.
type cellKey struct{ skill, harness string }

type noticeKind int

const (
	noticeInfo noticeKind = iota
	noticeGood
	noticeErr
)

type notice struct {
	text string
	kind noticeKind
}

type model struct {
	p *paths.Paths

	// The matrix columns: one per installed harness, in harness.All's
	// order — the same order the snapshot's states are keyed by.
	adapters  []harness.Adapter
	harnesses []string
	writable  map[string]bool

	rows   []snapshot.SkillRow
	col    int // harness column cursor
	row    int // skill row cursor, into the filtered rows
	offset int // first visible filtered row (scroll window)

	filter      textinput.Model
	filterFocus bool
	staged      map[cellKey]bool // value is the staged target: on or off

	phase    phase // refreshing while a reload runs
	helpOpen bool
	notice   notice
	quiet    bool // suppress the hero banner

	width, height int
	seq           int // guards against late snapshot messages
}

// newModel builds the model from an already-loaded snapshot. Columns come
// from the adapters, not from the report: the TUI needs each harness's
// write capability, and the order must match the snapshot's state keys.
func newModel(p *paths.Paths, report *snapshot.Report, quiet bool) model {
	ti := textinput.New()
	ti.Placeholder = "filter"
	ti.SetWidth(30)

	m := model{
		p:      p,
		rows:   report.Skills,
		filter: ti,
		staged: map[cellKey]bool{},
		quiet:  quiet,
		width:  80,
		height: 24,
	}
	m.rebuildColumns()
	return m
}

// rebuildColumns derives the matrix columns from the installed adapters.
func (m *model) rebuildColumns() {
	m.adapters = nil
	m.harnesses = nil
	m.writable = map[string]bool{}
	for _, a := range harness.Installed(m.p) {
		m.adapters = append(m.adapters, a)
		m.harnesses = append(m.harnesses, string(a.Harness()))
		m.writable[string(a.Harness())] = a.CanProject()
	}
	if m.col >= len(m.harnesses) {
		m.col = 0
	}
}

// loadSnapshot builds a fresh snapshot through the core: the canonical
// store, the repo's customs, every installed harness's config, and the
// update check. Update-check warnings are dropped here on purpose — the
// badge already shows ? for anything unchecked, and the notice line holds
// one message at a time.
//
// newTreeClient is a var so tests can script the GitHub seam.
var newTreeClient = snapshot.DefaultTreeClient

func loadSnapshot(p *paths.Paths) (*snapshot.Report, error) {
	report, _, err := snapshot.Build(context.Background(), p, newTreeClient(p))
	return report, err
}

// snapshotMsg carries a finished reload. notice is shown on success; empty
// keeps whatever notice is already up (an apply confirmation must survive
// the refresh that follows it).
type snapshotMsg struct {
	seq    int
	report *snapshot.Report
	notice string
	err    error
}

// Run launches the TUI. quiet suppresses the hero banner.
func Run(p *paths.Paths, quiet bool) error {
	m, err := prepare(p, quiet)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m).Run()
	return err
}

// prepare is the launch sequence: sync first — it runs on every fleet
// command — then the snapshot the matrix renders.
func prepare(p *paths.Paths, quiet bool) (model, error) {
	reports, err := fleetsync.Run(p)
	if err != nil {
		return model{}, fmt.Errorf("sync: %w", err)
	}
	report, err := loadSnapshot(p)
	if err != nil {
		return model{}, err
	}
	m := newModel(p, report, quiet)
	if s := syncNotice(reports); s != "" {
		m.notice = notice{text: s, kind: noticeInfo}
	}
	return m, nil
}

// syncNotice compresses sync's reports into one launch line.
func syncNotice(reports []fleetsync.Report) string {
	var removed, changed, flagged int
	for _, r := range reports {
		removed += len(r.Removed)
		changed += len(r.Changed)
		flagged += len(r.Flags)
	}
	if removed+changed+flagged == 0 {
		return ""
	}
	var parts []string
	if removed > 0 {
		parts = append(parts, fmt.Sprintf("%d redundant link%s removed", removed, plural(removed)))
	}
	if changed > 0 {
		parts = append(parts, fmt.Sprintf("%d state change%s applied", changed, plural(changed)))
	}
	if flagged > 0 {
		parts = append(parts, fmt.Sprintf("%d entr%s flagged for review", flagged, pick(flagged, "y", "ies")))
	}
	return "sync: " + strings.Join(parts, ", ")
}

func plural(n int) string { return pick(n, "", "s") }

func pick(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// ---- update ----

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scrollToCursor()
		return m, nil

	case snapshotMsg:
		return m.applySnapshot(msg)

	case updateMsg:
		return m.applyUpdate(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// applySnapshot lands a reload: rows and columns are replaced wholesale —
// the snapshot is the truth — while the selection follows its skill when
// it is still visible.
func (m model) applySnapshot(msg snapshotMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.seq {
		return m, nil // a cancelled or superseded reload
	}
	m.phase = phaseIdle
	if msg.err != nil {
		// The reload failed, but the cells can still be made truthful:
		// re-read the harness configs locally (no update check involved).
		// Only the update badges may go stale until the next refresh.
		text := "refresh failed: " + msg.err.Error()
		if err := m.refreshStates(); err != nil {
			text += " (cells not re-read: " + err.Error() + ")"
		}
		m.notice = notice{text: text, kind: noticeErr}
		return m, nil
	}
	m.setSnapshot(msg.report)
	if msg.notice != "" {
		m.notice = notice{text: msg.notice, kind: noticeGood}
	}
	return m, nil
}

// setSnapshot swaps in a fresh snapshot: rows, then columns (the harness
// set may have changed), then bookkeeping that references both.
func (m *model) setSnapshot(report *snapshot.Report) {
	prev := m.selectedName()
	m.rows = report.Skills
	m.rebuildColumns()
	m.selectByName(prev)
	m.pruneStaged()
	m.scrollToCursor()
}

// refreshStates re-reads per-skill enablement from the harness configs
// through the adapter seam and updates the rows in place. It never scans
// the store or the network: the skill set is whatever the rows already hold.
func (m *model) refreshStates() error {
	if len(m.rows) == 0 {
		return nil
	}
	names := make([]string, len(m.rows))
	for i, r := range m.rows {
		names[i] = r.Name
	}
	for _, a := range m.adapters {
		read, err := a.Read(names)
		if err != nil {
			return fmt.Errorf("read %s config: %w", a.Harness(), err)
		}
		for i := range m.rows {
			s := string(read.States[names[i]])
			if s == "" {
				s = string(harness.StateOn) // adapters report every name; be defensive anyway
			}
			m.rows[i].States[string(a.Harness())] = s
		}
	}
	return nil
}

// pruneStaged drops staged cells whose skill or harness no longer exists.
func (m *model) pruneStaged() {
	skills := map[string]bool{}
	for _, r := range m.rows {
		skills[r.Name] = true
	}
	for k := range m.staged {
		if !skills[k.skill] || !m.writable[k.harness] {
			delete(m.staged, k)
		}
	}
}

// startRefresh begins a snapshot reload; the notice line shows a
// "refreshing…" label while it runs.
func startRefresh(m *model, onDone string) tea.Cmd {
	m.seq++
	m.phase = phaseRefreshing
	seq, p := m.seq, m.p
	return func() tea.Msg {
		report, err := loadSnapshot(p)
		return snapshotMsg{seq: seq, report: report, notice: onDone, err: err}
	}
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" {
		return m, tea.Quit
	}

	if m.helpOpen {
		m.helpOpen = false // any key closes the overlay
		return m, nil
	}

	m.notice = notice{} // notices last until the next keypress

	if m.filterFocus {
		switch key {
		case "esc":
			prev := m.selectedName()
			m.filter.Reset()
			m.filterFocus = false
			m.selectByName(prev)
			return m, nil
		case "enter":
			m.filterFocus = false
			return m, nil
		case "q":
			// q quits only while the filter is empty; otherwise it is text.
			if m.filter.Value() == "" {
				return m, tea.Quit
			}
		}
		prev := m.selectedName()
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.selectByName(prev)
		return m, cmd
	}

	// While the update-all runs, the keys that would race it — another
	// update, a refresh, a staged apply — wait for idle. Navigation, the
	// filter, staging, help, and quit still work.
	if m.phase == phaseUpdating {
		switch key {
		case "u", "r", "enter":
			return m, nil
		}
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "h", "left":
		if m.col > 0 {
			m.col--
		}
	case "l", "right":
		if m.col < len(m.harnesses)-1 {
			m.col++
		}
	case "/":
		m.filterFocus = true
		return m, m.filter.Focus()
	case " ", "space":
		m.stage()
	case "enter":
		return m.applyStaged()
	case "u":
		return m, startUpdate(&m)
	case "r":
		return m, startRefresh(&m, "refreshed")
	case "?":
		m.helpOpen = true
	case "esc":
		switch {
		case len(m.staged) > 0:
			m.staged = map[cellKey]bool{}
			m.notice = notice{text: "staged changes discarded", kind: noticeInfo}
		case m.filter.Value() != "":
			prev := m.selectedName()
			m.filter.Reset()
			m.selectByName(prev)
		}
	}
	return m, nil
}

// selectByName puts the row cursor on the named skill when it is still
// visible, falling back to the nearest clamp when it is not.
func (m *model) selectByName(name string) {
	if name != "" {
		for i, r := range m.filtered() {
			if r.Name == name {
				m.row = i
				m.scrollToCursor()
				return
			}
		}
	}
	m.clampCursor()
	m.scrollToCursor()
}

// move shifts the row cursor by d and scrolls to keep it visible.
func (m *model) move(d int) {
	n := len(m.filtered())
	if n == 0 {
		return
	}
	m.row = min(max(m.row+d, 0), n-1)
	m.scrollToCursor()
}

func (m *model) clampCursor() {
	n := len(m.filtered())
	m.row = min(max(m.row, 0), max(n-1, 0))
}

func (m *model) scrollToCursor() {
	v := m.visibleRows()
	if m.row < m.offset {
		m.offset = m.row
	}
	if m.row >= m.offset+v {
		m.offset = m.row - v + 1
	}
	m.offset = max(m.offset, 0)
	// Group headers can push the cursor row below the fold the row math
	// thinks it fits in; walk the window down until it really renders.
	for m.row >= m.endFrom(m.offset) && m.offset < len(m.filtered())-1 {
		m.offset++
	}
}

// selectedName is the skill under the row cursor, or "" on an empty list.
func (m model) selectedName() string {
	fs := m.filtered()
	if m.row >= 0 && m.row < len(fs) {
		return fs[m.row].Name
	}
	return ""
}

// filtered is the visible row list: the snapshot's rows, custom first and
// grouped by source already, narrowed by the filter. The filter matches
// case-insensitively against name and description.
func (m model) filtered() []snapshot.SkillRow {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == "" {
		return m.rows
	}
	out := make([]snapshot.SkillRow, 0, len(m.rows))
	for _, r := range m.rows {
		if strings.Contains(strings.ToLower(r.Name), q) ||
			strings.Contains(strings.ToLower(r.Description), q) {
			out = append(out, r)
		}
	}
	return out
}

// cellState reads one cell's enablement from the rows.
func (m model) cellState(skill, harnessName string) harness.State {
	for _, r := range m.rows {
		if r.Name != skill {
			continue
		}
		if s := r.States[harnessName]; s != "" {
			return harness.State(s)
		}
	}
	return harness.StateOn
}

// stage flips the selected cell's staged intent. Cells that cannot be
// toggled — harnesses without a write side, skills a harness cannot
// discover — say so instead of staging a silent no-op.
func (m *model) stage() {
	name := m.selectedName()
	if name == "" || len(m.harnesses) == 0 {
		return
	}
	h := m.harnesses[m.col]
	if !m.writable[h] {
		m.notice = notice{
			text: fmt.Sprintf("%s has no per-skill disable mechanism — toggle is a no-op", h),
			kind: noticeInfo,
		}
		return
	}
	state := m.cellState(name, h)
	if state == harness.StateAbsent {
		m.notice = notice{
			text: fmt.Sprintf("%q isn't discoverable by %s — nothing to toggle", name, h),
			kind: noticeInfo,
		}
		return
	}
	k := cellKey{skill: name, harness: h}
	if _, ok := m.staged[k]; ok {
		delete(m.staged, k)
		return
	}
	m.staged[k] = state != harness.StateOn
}

// applyStaged pushes every staged change through the core — the state
// file, then projection and sync, the exact code the on/off verbs run —
// and re-reads the configs so the matrix tells the truth immediately.
func (m model) applyStaged() (tea.Model, tea.Cmd) {
	if len(m.staged) == 0 {
		m.notice = notice{text: "nothing staged", kind: noticeInfo}
		return m, nil
	}
	toggles := make([]toggle.Toggle, 0, len(m.staged))
	for k, on := range m.staged {
		toggles = append(toggles, toggle.Toggle{Name: k.skill, Harness: k.harness, On: on})
	}
	applied, _, _, err := toggle.Apply(m.p, toggles)
	if err != nil {
		// Keep the staged set: the user can look at the failure and retry.
		m.notice = notice{text: "apply failed: " + err.Error(), kind: noticeErr}
		return m, nil
	}
	m.staged = map[cellKey]bool{}
	m.notice = notice{
		text: fmt.Sprintf("applied %d change%s — some harnesses pick up config changes on their next session (opencode has no live reload)",
			applied, plural(applied)),
		kind: noticeGood,
	}
	return m, startRefresh(&m, "")
}
