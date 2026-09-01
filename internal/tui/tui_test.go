package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/outdated"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/snapshot"
	"github.com/zzacong/fleet/internal/state"
)

// tuiHome builds a home with every harness installed, two installed skills
// ("tdd", "git-helper") with lockfile provenance, and a repo custom
// ("my-notes") — the grouping the matrix renders.
func tuiHome(t *testing.T) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	repo := filepath.Join(t.TempDir(), "repo")
	p := paths.WithRepo(home, repo)
	writeSkillDir(t, p.SkillsStore(), "tdd", "Red-green-refactor workflow for tests.")
	writeSkillDir(t, p.SkillsStore(), "git-helper", "Wraps common git workflows.")
	writeSkillDir(t, p.RepoSkills(), "my-notes", "Personal note-taking conventions.")
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeLock(t, p)
	return p
}

func writeSkillDir(t *testing.T, store, dir, description string) {
	t.Helper()
	path := filepath.Join(store, dir, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + dir + "\ndescription: " + description + "\n---\n\n# " + dir + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeLock(t *testing.T, p *paths.Paths) {
	t.Helper()
	lock := `{"version": 3, "skills": {
		"tdd": {
			"source": "mattpocock/skills",
			"sourceType": "github",
			"skillPath": "skills/tdd/SKILL.md",
			"skillFolderHash": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		"git-helper": {
			"source": "mattpocock/skills",
			"sourceType": "github",
			"skillPath": "skills/git-helper/SKILL.md",
			"skillFolderHash": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}
	}}`
	if err := os.WriteFile(p.SkillLock(), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
}

// noNetwork is the stubbed GitHub seam: any fetch is a test failure. The
// matrix must work from the lockfile and the configs alone.
type noNetwork struct{}

func (noNetwork) FetchTree(context.Context, string, string, string, string) (outdated.TreeResponse, error) {
	return outdated.TreeResponse{}, errors.New("tests must not reach the network")
}

func stubTreeClient(t *testing.T) {
	t.Helper()
	prev := newTreeClient
	newTreeClient = func(*paths.Paths) outdated.TreeClient { return noNetwork{} }
	t.Cleanup(func() { newTreeClient = prev })
}

// newTestModel builds the model the way Run's prepare step does, minus the
// program itself.
func newTestModel(t *testing.T, p *paths.Paths) model {
	t.Helper()
	stubTreeClient(t)
	report, err := loadSnapshot(p)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	return newModel(p, report, false)
}

// The keys the tests drive the model with.

var (
	keySpace = tea.KeyPressMsg{Code: tea.KeySpace}
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc   = tea.KeyPressMsg{Code: tea.KeyEsc}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
	keyRight = tea.KeyPressMsg{Code: tea.KeyRight}
	keySlash = tea.KeyPressMsg{Code: '/'}
	keyR     = tea.KeyPressMsg{Code: 'r'}
	keyU     = tea.KeyPressMsg{Code: 'u'}
)

func sendKey(m model, k tea.KeyPressMsg) model {
	m2, _ := m.Update(k)
	return m2.(model)
}

func typeText(m model, s string) model {
	for _, r := range s {
		m = sendKey(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// selectSkill narrows the list to one skill and closes the filter again:
// the selection lands on it, ready for staging. (Space types into the
// filter while it holds focus, so it must be closed first.)
func selectSkill(m model, query string) model {
	m = sendKey(m, keySlash)
	m = typeText(m, query)
	return sendKey(m, keyEsc)
}

// moveRight shifts the column cursor times columns to the right.
func moveRight(m model, times int) model {
	for i := 0; i < times; i++ {
		m = sendKey(m, keyRight)
	}
	return m
}

// settle runs a command and feeds its message back into the model — the
// same loop tea.Run would drive, without a terminal.
func settle(t *testing.T, m model, cmd tea.Cmd) model {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command to settle")
	}
	msg := cmd()
	m2, next := m.Update(msg)
	if next != nil {
		t.Fatalf("unexpected follow-up command after %T", msg)
	}
	return m2.(model)
}

// pressRefresh presses r and settles the reload it schedules.
func pressRefresh(t *testing.T, m model) model {
	t.Helper()
	m2, cmd := m.Update(keyR)
	return settle(t, m2.(model), cmd)
}

func findRow(t *testing.T, m model, name string) *snapshot.SkillRow {
	t.Helper()
	for i := range m.rows {
		if m.rows[i].Name == name {
			return &m.rows[i]
		}
	}
	t.Fatalf("row %q not found", name)
	return nil
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(body)
}

func requireMissing(t *testing.T, path, what string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("%s exists after the operation (%s)", what, path)
	}
}

// seedDisable records a disable in the state file, the way the on/off
// verbs do, without projecting it.
func seedDisable(t *testing.T, p *paths.Paths, skill, harnessName string) {
	t.Helper()
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	st.SetDisabled(skill, harnessName)
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}
}

// seedLink creates a skills-CLI-style per-agent link into the canonical
// store — the redundant kind sync exists to remove.
func seedLink(t *testing.T, p *paths.Paths, skillsDir, name string) {
	t.Helper()
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(p.SkillsStore(), name), filepath.Join(skillsDir, name)); err != nil {
		t.Fatal(err)
	}
}

// Column indices in harness.All's order.
const (
	colOpenCode = 0
	colPi       = 1
	colCodex    = 2
	colClaude   = 3
	colCursor   = 4
	colBob      = 5
)

func TestStageThenApplyWritesStateAndHarnessConfigs(t *testing.T) {
	p := tuiHome(t)
	m := newTestModel(t, p)

	// Focus the tdd row: the repo custom and git-helper don't match.
	m = selectSkill(m, "tdd")
	if got := m.selectedName(); got != "tdd" {
		t.Fatalf("selected %q, want tdd", got)
	}

	// Stage opencode (col 0), move two right, stage codex (col 2).
	m = sendKey(m, keySpace)
	m = moveRight(m, 2)
	m = sendKey(m, keySpace)
	if len(m.staged) != 2 {
		t.Fatalf("staged %d cells, want 2", len(m.staged))
	}

	// Nothing is written until enter.
	requireMissing(t, p.FleetStateFile(), "state file")

	m2, cmd := m.Update(keyEnter)
	m = m2.(model)

	// The state file records exactly the staged cells: the sparse schema
	// lists only disables, and only for harnesses with a write side.
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDisabled("tdd", "opencode") || !st.IsDisabled("tdd", "codex") {
		t.Errorf("state does not record the staged disables: %v", st.Names())
	}
	for _, h := range []string{"pi", "claude", "cursor", "bob"} {
		if st.IsDisabled("tdd", h) {
			t.Errorf("state disables tdd for %s without a staged change", h)
		}
	}

	// Each harness got its own native off marker; the others were untouched.
	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, `"tdd": "deny"`) {
		t.Errorf("opencode config:\n%s", body)
	}
	if body := readFile(t, p.CodexConfig()); !strings.Contains(body, `name = "tdd"`) || !strings.Contains(body, "enabled = false") {
		t.Errorf("codex config:\n%s", body)
	}
	if body := readFile(t, p.PiSettings()); strings.Contains(body, "tdd") {
		t.Errorf("pi touched without a staged change:\n%s", body)
	}
	requireMissing(t, p.ClaudeSettings(), "claude settings")

	// The confirmation names the count and the next-session note.
	if !strings.Contains(m.notice.text, "applied 2 changes") || !strings.Contains(m.notice.text, "next session") {
		t.Errorf("apply notice: %q", m.notice.text)
	}

	// The reload that follows lands the new states in the rows.
	m = settle(t, m, cmd)
	row := findRow(t, m, "tdd")
	if row.States["opencode"] != string(harness.StateOff) || row.States["codex"] != string(harness.StateOff) {
		t.Errorf("rows after apply: opencode=%s codex=%s, want off/off", row.States["opencode"], row.States["codex"])
	}
	if row.States["pi"] != string(harness.StateOn) {
		t.Errorf("rows after apply: pi=%s, want on", row.States["pi"])
	}
}

func TestApplyEnableStripsMarkersBeforeSync(t *testing.T) {
	p := tuiHome(t)
	// Seed: tdd disabled for pi, projected by the adapter itself.
	seedDisable(t, p, "tdd", "pi")
	if _, err := harness.NewPi(p).Project([]harness.SkillWrite{{Name: "tdd", State: harness.StateOff}}); err != nil {
		t.Fatal(err)
	}

	m := newTestModel(t, p)
	m = selectSkill(m, "tdd")
	m = moveRight(m, colPi)
	m = sendKey(m, keySpace) // the cell is off; staging targets on

	m2, cmd := m.Update(keyEnter)
	m = m2.(model)

	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if st.IsDisabled("tdd", "pi") {
		t.Error("state still disables tdd/pi after the enable")
	}
	if body := readFile(t, p.PiSettings()); strings.Contains(body, "-skills/tdd/SKILL.md") {
		t.Errorf("pi exclusion survived the enable (sync would flag it):\n%s", body)
	}

	m = settle(t, m, cmd)
	row := findRow(t, m, "tdd")
	if row.States["pi"] != string(harness.StateOn) {
		t.Errorf("pi=%s after enable, want on", row.States["pi"])
	}
}

func TestEscDiscardsStagedWithoutWriting(t *testing.T) {
	p := tuiHome(t)
	m := newTestModel(t, p)

	m = selectSkill(m, "tdd")
	m = sendKey(m, keySpace)
	if len(m.staged) != 1 {
		t.Fatalf("staged %d cells, want 1", len(m.staged))
	}
	m = sendKey(m, keyEsc)
	if len(m.staged) != 0 {
		t.Fatalf("staged %d cells after esc, want 0", len(m.staged))
	}

	requireMissing(t, p.FleetStateFile(), "state file")
	requireMissing(t, p.OpenCodeConfig(), "opencode config")
}

func TestTogglesForCursorAndBobAreExplainedNoOps(t *testing.T) {
	p := tuiHome(t)
	for _, tc := range []struct {
		name   string
		column int
	}{
		{"cursor", colCursor},
		{"bob", colBob},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t, p)
			m = selectSkill(m, "tdd")
			m = moveRight(m, tc.column)
			m = sendKey(m, keySpace)

			if len(m.staged) != 0 {
				t.Fatalf("staged a cell in %s, which has no write side", tc.name)
			}
			if !strings.Contains(m.notice.text, "no per-skill disable mechanism") || !strings.Contains(m.notice.text, tc.name) {
				t.Errorf("notice: %q", m.notice.text)
			}

			// Enter with nothing staged writes nothing.
			m2, cmd := m.Update(keyEnter)
			m = m2.(model)
			if cmd != nil {
				t.Error("enter after a blocked stage returned a command")
			}
			if !strings.Contains(m.notice.text, "nothing staged") {
				t.Errorf("notice: %q", m.notice.text)
			}
			requireMissing(t, p.FleetStateFile(), "state file")
		})
	}
}

func TestToggleOnUndiscoverableClaudeCellSaysSo(t *testing.T) {
	p := tuiHome(t)
	m := newTestModel(t, p)

	m = selectSkill(m, "tdd")
	m = moveRight(m, colClaude)
	m = sendKey(m, keySpace)
	if len(m.staged) != 0 {
		t.Fatal("staged a cell claude cannot discover")
	}
	if !strings.Contains(m.notice.text, `isn't discoverable by claude`) {
		t.Errorf("notice: %q", m.notice.text)
	}

	// A link is claude's only discovery path. Once it exists, the same
	// cell toggles — and refresh (through the core) is what makes the
	// matrix see it.
	seedLink(t, p, p.ClaudeSkills(), "tdd")
	m = pressRefresh(t, m)
	m = sendKey(m, keySpace)
	if len(m.staged) != 1 {
		t.Fatalf("staged %d cells after the link appeared, want 1", len(m.staged))
	}

	m2, cmd := m.Update(keyEnter)
	m = m2.(model)
	if body := readFile(t, p.ClaudeSettings()); !strings.Contains(body, `"tdd": "off"`) {
		t.Errorf("claude settings:\n%s", body)
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDisabled("tdd", "claude") {
		t.Error("state does not record tdd/claude")
	}
	m = settle(t, m, cmd)
	row := findRow(t, m, "tdd")
	if row.States["claude"] != string(harness.StateOff) {
		t.Errorf("claude=%s after apply, want off", row.States["claude"])
	}
}

func TestFilterNarrowsRowsAndSelectionFollows(t *testing.T) {
	p := tuiHome(t)
	m := newTestModel(t, p)

	// Rows render custom-first, then by source repo: my-notes, then
	// git-helper and tdd from mattpocock/skills.
	if got := m.selectedName(); got != "my-notes" {
		t.Fatalf("first row %q, want my-notes (custom first)", got)
	}
	m = sendKey(m, keyDown)
	if got := m.selectedName(); got != "git-helper" {
		t.Fatalf("second row %q, want git-helper", got)
	}

	// Filtering to git-helper and tdd keeps the selection on its skill
	// (followed by name, not clamped by index — clamping would land on
	// tdd, one row down).
	m = sendKey(m, keySlash)
	m = typeText(m, "workflow")
	if got := m.selectedName(); got != "git-helper" {
		t.Fatalf("selection after filter %q, want git-helper", got)
	}

	// Esc clears the filter and the selection stays put.
	m = sendKey(m, keyEsc)
	if len(m.filtered()) != 3 {
		t.Fatalf("%d rows after esc, want 3", len(m.filtered()))
	}
	if got := m.selectedName(); got != "git-helper" {
		t.Fatalf("selection after esc %q, want git-helper", got)
	}

	// Filtering away the selection clamps to the nearest row.
	m = sendKey(m, keySlash)
	m = typeText(m, "git")
	if got := m.selectedName(); got != "git-helper" || len(m.filtered()) != 1 {
		t.Fatalf("filtered selection %q (%d rows), want git-helper (1)", got, len(m.filtered()))
	}
}

func TestRefreshRebuildsRowsThroughTheCore(t *testing.T) {
	p := tuiHome(t)
	m := newTestModel(t, p)

	// Out of band: the state file and the config change behind the
	// matrix's back, exactly as a CLI verb in another terminal would.
	seedDisable(t, p, "git-helper", "codex")
	if _, err := harness.NewCodex(p).Project([]harness.SkillWrite{{Name: "git-helper", State: harness.StateOff}}); err != nil {
		t.Fatal(err)
	}

	m = pressRefresh(t, m)
	row := findRow(t, m, "git-helper")
	if row.States["codex"] != string(harness.StateOff) {
		t.Errorf("codex=%s after refresh, want off", row.States["codex"])
	}
}

func TestApplyRunsSyncAndRemovesRedundantLinks(t *testing.T) {
	p := tuiHome(t)
	seedLink(t, p, p.OpenCodeSkills(), "tdd")

	m := newTestModel(t, p)
	m = selectSkill(m, "tdd")
	m = sendKey(m, keySpace)
	m2, cmd := m.Update(keyEnter)
	m = m2.(model)

	// The apply went through sync, not just the state file.
	requireMissing(t, filepath.Join(p.OpenCodeSkills(), "tdd"), "redundant link")
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDisabled("tdd", "opencode") {
		t.Error("state does not record tdd/opencode")
	}
	m = settle(t, m, cmd)
	row := findRow(t, m, "tdd")
	if row.States["opencode"] != string(harness.StateOff) {
		t.Errorf("opencode=%s after apply, want off", row.States["opencode"])
	}
}

func TestPrepareSyncsBeforeTheFirstFrame(t *testing.T) {
	p := tuiHome(t)
	seedDisable(t, p, "tdd", "opencode")
	seedLink(t, p, p.OpenCodeSkills(), "tdd")

	stubTreeClient(t)
	m, err := prepare(p, false)
	if err != nil {
		t.Fatal(err)
	}

	// Sync converged the config with the state file and cleaned the link
	// before the matrix rendered anything.
	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, `"tdd": "deny"`) {
		t.Errorf("opencode config after launch sync:\n%s", body)
	}
	requireMissing(t, filepath.Join(p.OpenCodeSkills(), "tdd"), "redundant link")
	if !strings.Contains(m.notice.text, "sync:") {
		t.Errorf("launch notice: %q", m.notice.text)
	}
	row := findRow(t, m, "tdd")
	if row.States["opencode"] != string(harness.StateOff) {
		t.Errorf("opencode=%s in the first frame, want off", row.States["opencode"])
	}
}

func TestViewCarriesBannerStatusAndHelp(t *testing.T) {
	p := tuiHome(t)
	stubTreeClient(t)
	report, err := loadSnapshot(p)
	if err != nil {
		t.Fatal(err)
	}
	loud := newModel(p, report, false)
	quiet := newModel(p, report, true)

	// The hero banner belongs to the TUI launch; --quiet suppresses it.
	if v := loud.View().Content; !strings.Contains(v, "███████╗") {
		t.Error("view without the hero banner")
	}
	if v := quiet.View().Content; strings.Contains(v, "███████╗") {
		t.Error("--quiet view still shows the hero banner")
	}

	// The banner spells FLEET: five glyphs, and after L's bottom bar both
	// E glyphs carry a full-width bottom bar on the fifth line and a full
	// width ╚══════╝ on the sixth — not F's bare ██║/╚═╝ legs. Regressed
	// once into "FLFET"; pin the spelling.
	art := strings.Split(strings.TrimRight(strings.TrimPrefix(bannerArt, "\n"), "\n"), "\n")
	if len(art) != 6 {
		t.Fatalf("banner has %d lines, want 6", len(art))
	}
	const eBottom, eFoot = "███████╗", "╚══════╝"
	if strings.Count(art[4], eBottom) != 3 || strings.Count(art[5], eFoot) != 3 {
		t.Errorf("banner does not spell FLEET — line 5 = %q, line 6 = %q", art[4], art[5])
	}

	// The status bar counts total, installed, custom, outdated — and the
	// staged count appears once something is staged.
	if v := quiet.View().Content; !strings.Contains(v, "3 skills · 2 installed · 1 custom · 0 outdated") {
		t.Errorf("status bar missing counts:\n%s", v)
	}
	quiet = selectSkill(quiet, "tdd")
	quiet = sendKey(quiet, keySpace)
	if v := quiet.View().Content; !strings.Contains(v, "1 staged") {
		t.Error("status bar missing the staged count")
	}

	// The help overlay carries the limitation notes.
	help := quiet.helpOverlay()
	if !strings.Contains(help, "no per-skill off switch") {
		t.Error("help missing the cursor/bob note")
	}
	if !strings.Contains(help, "opencode has no live reload") {
		t.Error("help missing the next-session note")
	}
}

func TestCursorRowRendersOnAFullPageWithGroupHeaders(t *testing.T) {
	p := tuiHome(t)
	// Three skills across two more source repos plus unlocked customs:
	// four groups, so four header lines compete for the body budget.
	for i := 0; i < 10; i++ {
		writeSkillDir(t, p.SkillsStore(), fmt.Sprintf("skill-%02d", i), "Filler skill for scrolling.")
	}
	for i := 10; i < 20; i++ {
		writeSkillDir(t, p.SkillsStore(), fmt.Sprintf("skill-%02d", i), "Filler skill for scrolling.")
	}
	lock := `{"version": 3, "skills": {`
	for i := 0; i < 10; i++ {
		lock += fmt.Sprintf(`"skill-%02d": {"source": "vercel-labs/skills", "sourceType": "github", "skillPath": "skills/skill-%02d/SKILL.md", "skillFolderHash": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},`, i, i)
	}
	for i := 10; i < 20; i++ {
		lock += fmt.Sprintf(`"skill-%02d": {"source": "vercel-labs/skills-2", "sourceType": "github", "skillPath": "skills/skill-%02d/SKILL.md", "skillFolderHash": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},`, i, i)
	}
	lock += `"tdd": {"source": "mattpocock/skills", "sourceType": "github", "skillPath": "skills/tdd/SKILL.md", "skillFolderHash": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}`
	if err := os.WriteFile(p.SkillLock(), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestModel(t, p)
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m = m2.(model)

	// Walk to the bottom of the list and make sure the cursor row really
	// renders — the row math alone would leave it pushed out by headers.
	for i := 0; i < 40; i++ {
		m = sendKey(m, keyDown)
	}
	if got := m.selectedName(); got != "skill-19" {
		t.Fatalf("bottom row %q, want skill-19", got)
	}
	view := m.View().Content
	if !strings.Contains(view, "skill-19") {
		t.Error("the cursor row did not render on a full page")
	}
	if !strings.Contains(view, "· vercel-labs/skills") {
		t.Error("group headers missing from the page")
	}
}

func TestQuitKeys(t *testing.T) {
	p := tuiHome(t)
	m := newTestModel(t, p)

	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'q'}); cmd == nil {
		t.Error("q did not quit")
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); cmd == nil {
		t.Error("ctrl+c did not quit")
	}
	// While the filter holds text, q is a character, not a quit.
	m = sendKey(m, keySlash)
	m = typeText(m, "tdd")
	m = sendKey(m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	if v := m.filter.Value(); !strings.Contains(v, "tddq") {
		t.Errorf("filter value %q, want the q appended", v)
	}
}
