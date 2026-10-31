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
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zzacong/fleet/internal/buildinfo"
	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/selfupdate"
	"github.com/zzacong/fleet/internal/snapshot"
	fleetsync "github.com/zzacong/fleet/internal/sync"
	"github.com/zzacong/fleet/internal/toggle"
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
	// linkToggle names the harnesses whose only lever over a custom skill
	// is its managed link: those cells are toggleable for customs even
	// though they have no config write side.
	linkToggle map[string]bool

	rows   []snapshot.SkillRow
	col    int // harness column cursor
	row    int // skill row cursor, into the filtered rows
	offset int // first visible filtered row (scroll window)

	filter      textinput.Model
	filterFocus bool
	staged      map[cellKey]bool // value is the staged target: on or off

	phase          phase // refreshing while a reload runs
	updateTarget   string
	helpOpen       bool
	notice         notice
	pendingArchive *pendingArchive // an archive awaiting y; nil when no prompt is up
	inlineErr      inlineError     // a refusal drawn under its row
	quiet          bool            // suppress the hero banner

	width, height int
	seq           int // guards against late snapshot messages

	// checks is the background update work the current snapshot left
	// pending; checking marks the skills whose badge is still waiting on it,
	// and checkWarnings collects its failures for one notice at the end.
	checks        *snapshot.Checks
	checking      map[string]bool
	checkWarnings []string
	// releasing is true until the Releases footer check lands; it keeps the
	// status line saying checks are running.
	releasing bool
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
	m.linkToggle = map[string]bool{}
	for _, a := range harness.Installed(m.p) {
		m.adapters = append(m.adapters, a)
		m.harnesses = append(m.harnesses, string(a.Harness()))
		m.writable[string(a.Harness())] = a.CanProject()
		m.linkToggle[string(a.Harness())] = harness.LinkToggleable(a)
	}
	if m.col >= len(m.harnesses) {
		m.col = 0
	}
}

// newTreeClient is a var so tests can script the GitHub seam.
var newTreeClient = snapshot.DefaultTreeClient

// loadSnapshot builds the matrix's snapshot without waiting on GitHub: the
// cache fills the badges it can, and the rest come back as Checks for the
// background run. force makes every repo pending (the manual refresh).
func loadSnapshot(p *paths.Paths, force bool) (*snapshot.Loaded, error) {
	return snapshot.Load(p, newTreeClient(p), force)
}

// reloadMode says whether a reload trusts the update cache or re-checks
// every repo.
type reloadMode int

const (
	reloadCached reloadMode = iota // only stale or missing repos go to the network
	reloadFresh                    // manual refresh: every repo is re-checked
)

// snapshotMsg carries a finished reload. notice is shown on success; empty
// keeps whatever notice is already up (an apply confirmation must survive
// the refresh that follows it).
type snapshotMsg struct {
	seq    int
	loaded *snapshot.Loaded
	notice string
	err    error
}

// checkMsg is one repo tree's finished background check. index is its
// position in the checks, so the next group can start when this one lands.
type checkMsg struct {
	seq    int
	index  int
	result snapshot.Result
}

// releaseMsg is the Releases footer check, finished in the background.
type releaseMsg struct {
	line string
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
// command — then the snapshot the matrix renders. Nothing here waits on
// GitHub or the Releases API: Init starts those in the background.
func prepare(p *paths.Paths, quiet bool) (model, error) {
	reports, err := fleetsync.Run(p)
	if err != nil {
		return model{}, fmt.Errorf("sync: %w", err)
	}
	loaded, err := loadSnapshot(p, false)
	if err != nil {
		return model{}, err
	}
	m := newModel(p, loaded.Report, quiet)
	m.adoptChecks(loaded.Checks, loaded.Warnings)
	m.releasing = true
	if s := syncNotice(reports); s != "" {
		m.notice = notice{text: s, kind: noticeInfo}
	}
	m.flushWarnings()
	return m, nil
}

// adoptChecks swaps in the update work a load left behind, marks its skills
// as waiting on a result, and queues the load's own warnings to surface once
// those checks finish.
func (m *model) adoptChecks(checks *snapshot.Checks, warnings []string) {
	m.checks = checks
	m.checking = make(map[string]bool, len(checks.Pending()))
	for _, name := range checks.Pending() {
		m.checking[name] = true
	}
	m.checkWarnings = slices.Clone(warnings)
}

// dropChecks abandons the pending update work after a failed reload: its
// results can never land, so the skills stop waiting on them.
func (m *model) dropChecks() {
	m.checks = nil
	m.checking = nil
	m.checkWarnings = nil
}

// flushWarnings puts the queued update problems on the notice line once no
// skill is still waiting on a check. Repeated lines collapse into one.
func (m *model) flushWarnings() {
	if len(m.checking) > 0 || len(m.checkWarnings) == 0 {
		return
	}
	lines := slices.Clone(m.checkWarnings)
	slices.Sort(lines)
	m.appendNotice(strings.Join(slices.Compact(lines), " · "), noticeErr)
	m.checkWarnings = nil
}

// appendNotice adds line to the notice line, joined to any notice already
// up so launch still speaks with one voice.
func (m *model) appendNotice(line string, kind noticeKind) {
	if m.notice.text == "" {
		m.notice = notice{text: line, kind: kind}
		return
	}
	m.notice.text += " · " + line
}

// checkCmd runs group i of the pending checks in the background. The result
// comes back as a checkMsg tagged with the current reload's seq, so a run a
// newer reload superseded is dropped when it lands.
func (m model) checkCmd(i int) tea.Cmd {
	if m.checks == nil || i >= m.checks.Len() {
		return nil
	}
	seq, checks := m.seq, m.checks
	return func() tea.Msg {
		return checkMsg{seq: seq, index: i, result: checks.Run(context.Background(), i)}
	}
}

// releaseCmd runs the Releases footer check in the background. The line is
// empty when there is nothing to say.
func releaseCmd(p *paths.Paths) tea.Cmd {
	return func() tea.Msg {
		return releaseMsg{line: fleetUpdateLine(p)}
	}
}

// newReleaseClient is the Releases seam for the TUI footer: tests stub it
// so launch never touches the network (docs/testing.md: the network is a
// seam, not a dependency).
var newReleaseClient = selfupdate.NewHTTPReleaseClient

// fleetUpdateLine returns the one-line TUI update notice, or "" when the
// binary is current, a dev build, or the check fails. Best-effort with a
// short timeout; the TUI runs it in the background (releaseCmd), and the
// 24h cache makes the common case instant.
func fleetUpdateLine(p *paths.Paths) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	latest, ok := selfupdate.Check(ctx, p.FleetVersionCheckFile(), buildinfo.Version, newReleaseClient(""))
	if !ok {
		return ""
	}
	return selfupdate.FooterLine(buildinfo.Version, latest)
}

// syncNotice compresses sync's reports into one launch line.
func syncNotice(reports []fleetsync.Report) string {
	var cleaned, removed, linked, unlinked, changed, flagged int
	for _, r := range reports {
		cleaned += len(r.Cleaned)
		removed += len(r.Removed)
		linked += len(r.Linked)
		unlinked += len(r.Unlinked)
		changed += len(r.Changed)
		flagged += len(r.Flags)
	}
	if cleaned+removed+linked+unlinked+changed+flagged == 0 {
		return ""
	}
	var parts []string
	if cleaned > 0 {
		parts = append(parts, fmt.Sprintf("%d legacy entr%s removed", cleaned, pick(cleaned, "y", "ies")))
	}
	if removed > 0 {
		parts = append(parts, fmt.Sprintf("%d redundant link%s removed", removed, plural(removed)))
	}
	if linked > 0 {
		parts = append(parts, fmt.Sprintf("%d custom link%s added", linked, plural(linked)))
	}
	if unlinked > 0 {
		parts = append(parts, fmt.Sprintf("%d custom link%s hidden", unlinked, plural(unlinked)))
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

// Init starts the work the first frame did not wait for: the update checks
// the cache could not answer, and the Releases footer check.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.checkCmd(0), releaseCmd(m.p))
}

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

	case checkMsg:
		return m.applyCheck(msg)

	case releaseMsg:
		m.releasing = false
		if msg.line != "" {
			m.appendNotice(msg.line, noticeInfo)
		}
		return m, nil

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
		m.dropChecks()
		return m, nil
	}
	m.setSnapshot(msg.loaded.Report)
	m.adoptChecks(msg.loaded.Checks, msg.loaded.Warnings)
	if msg.notice != "" {
		m.notice = notice{text: msg.notice, kind: noticeGood}
	}
	m.flushWarnings()
	return m, m.checkCmd(0)
}

// applyCheck lands one repo tree's badges. A result from a superseded
// reload is dropped. Each landed group starts the next; the last one also
// surfaces the failures it collected, once.
func (m model) applyCheck(msg checkMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.seq {
		return m, nil
	}
	for i := range m.rows {
		name := m.rows[i].Name
		if badge, ok := msg.result.Outdated[name]; ok {
			m.rows[i].Outdated = badge
			delete(m.checking, name)
		}
	}
	m.checkWarnings = append(m.checkWarnings, msg.result.Warnings...)
	if next := m.checkCmd(msg.index + 1); next != nil {
		return m, next
	}
	m.flushWarnings()
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
// "refreshing…" label while it runs. The load itself is quick, so the
// phase ends with it; any repos still checking fill in afterwards.
func startRefresh(m *model, onDone string, mode reloadMode) tea.Cmd {
	m.seq++
	m.phase = phaseRefreshing
	seq, p, force := m.seq, m.p, mode == reloadFresh
	return func() tea.Msg {
		loaded, err := loadSnapshot(p, force)
		return snapshotMsg{seq: seq, loaded: loaded, notice: onDone, err: err}
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
	m.inlineErr = inlineError{}

	if ask := m.pendingArchive; ask != nil {
		m.pendingArchive = nil
		if key == "y" {
			return m.archive(ask)
		}
		m.notice = notice{text: "archive cancelled", kind: noticeInfo}
		return m, nil
	}

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
	// update, a refresh, a staged apply, an archive — wait for idle.
	// Navigation, the filter, staging, help, and quit still work.
	if m.phase == phaseUpdating {
		switch key {
		case "u", "U", "r", "enter", "A":
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
	case "U":
		if name := m.selectedName(); name == "" {
			m.notice = notice{text: "no skill selected", kind: noticeInfo}
			return m, nil
		} else if m.isCustom(name) {
			m.notice = notice{text: fmt.Sprintf("%q is a custom skill — nothing to update", name), kind: noticeInfo}
			return m, nil
		} else {
			return m, startUpdateOne(&m, m.selectedName())
		}
	case "A":
		return m.askArchive()
	case "r":
		return m, startRefresh(&m, "refreshed", reloadFresh)
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

// isCustom reports whether the named skill is a custom skill (lives in the
// repo, not the canonical store). Custom skills have nothing to update.
func (m model) isCustom(name string) bool {
	for _, r := range m.rows {
		if r.Name == name {
			return r.Custom
		}
	}
	return false
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
// toggled — harnesses with no config write side and no custom-link lever,
// skills a harness cannot discover — say so instead of staging a silent
// no-op. A custom skill on a link-toggleable harness is toggleable through
// its managed link even when the link is currently absent (that absence is
// the "off").
func (m *model) stage() {
	name := m.selectedName()
	if name == "" || len(m.harnesses) == 0 {
		return
	}
	h := m.harnesses[m.col]
	linkCustom := m.linkToggle[h] && m.isCustom(name)
	if !m.writable[h] && !linkCustom {
		m.notice = notice{
			text: fmt.Sprintf("%s has no per-skill disable mechanism — toggle is a no-op", h),
			kind: noticeInfo,
		}
		return
	}
	state := m.cellState(name, h)
	if state == harness.StateAbsent && !linkCustom {
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
	return m, startRefresh(&m, "", reloadCached)
}
