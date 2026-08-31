// The update-all tests: `u` drives the exact machinery the `fleet skill
// update` verb runs — the wrapped skills CLI call through the injected
// runner seam (the binary does not exist here), the post-run sync, then a
// matrix refresh — and reports from fleet's own state, never the CLI's
// prose. A failed run surfaces its raw output and leaves the model
// usable.

package tui

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zacong/fleet/internal/harness"
	"github.com/zacong/fleet/internal/skillscli"
)

// stubSkillsRunner replaces the runner seam for one test, the way the
// update verb's tests swap theirs.
func stubSkillsRunner(t *testing.T, run func(skillscli.Invocation) (skillscli.Result, error)) {
	t.Helper()
	prev := newSkillsRunner
	newSkillsRunner = func() skillscli.Runner { return skillscli.RunnerFunc(run) }
	t.Cleanup(func() { newSkillsRunner = prev })
}

// pressU presses u and lets every command it schedules settle: the
// wrapped run, then — on success — the refresh that follows it.
func pressU(t *testing.T, m model) model {
	t.Helper()
	m2, cmd := m.Update(keyU)
	m = m2.(model)
	if cmd == nil {
		t.Fatal("u did not start the update")
	}
	for i := 0; cmd != nil && i < 4; i++ {
		msg := cmd()
		m2, cmd = m.Update(msg)
		m = m2.(model)
	}
	if cmd != nil {
		t.Fatal("the update never settled")
	}
	return m
}

func TestUpdateAllRunsTheWrappedUpdateThenSyncAndRefreshes(t *testing.T) {
	p := tuiHome(t)
	// tdd is disabled for pi and the marker is projected: the wrapped run
	// disturbs it, the post-run sync re-applies it.
	seedDisable(t, p, "tdd", "pi")
	if _, err := harness.NewPi(p).Project([]harness.SkillWrite{{Name: "tdd", State: harness.StateOff}}); err != nil {
		t.Fatal(err)
	}

	var inv skillscli.Invocation
	stubSkillsRunner(t, func(i skillscli.Invocation) (skillscli.Result, error) {
		inv = i
		// The wrapped run's side effects: opencode's redundant link comes
		// back, pi's exclusion is stripped, and a new skill lands in the
		// store — the mess sync cleans and the refresh must surface.
		seedLink(t, p, p.OpenCodeSkills(), "tdd")
		if err := os.WriteFile(p.PiSettings(), []byte(`{"skills": []}`), 0o644); err != nil {
			t.Fatal(err)
		}
		writeSkillDir(t, p.SkillsStore(), "fresh-skill", "Installed by the wrapped run.")
		return skillscli.Result{Stdout: "✔ updated 1 skill"}, nil
	})

	m := newTestModel(t, p)
	m = pressU(t, m)

	// The wrapped call went through the seam with the explicit flags.
	if inv.Exe != "skills" || !reflect.DeepEqual(inv.Args, []string{"update", "-g", "-y"}) {
		t.Errorf("wrapped call = %s %v, want skills update -g -y", inv.Exe, inv.Args)
	}

	// The post-run sync ran: the redundant link is gone again and pi's
	// exclusion is back — disabled stays disabled.
	requireMissing(t, filepath.Join(p.OpenCodeSkills(), "tdd"), "redundant link")
	if body := readFile(t, p.PiSettings()); !strings.Contains(body, "-skills/tdd/SKILL.md") {
		t.Errorf("pi exclusion not re-applied:\n%s", body)
	}

	// The notice reports fleet's own post-run state — the store scan and
	// lockfile — never the CLI's prose. The prose claims one updated
	// skill; the store scan says three.
	if m.notice.kind != noticeGood {
		t.Error("the success notice is not styled as good")
	}
	if !strings.Contains(m.notice.text, "updated all — 3 skills in "+p.SkillsStore()+" (2 installed, 1 custom)") {
		t.Errorf("update notice: %q", m.notice.text)
	}
	if strings.Contains(m.notice.text, "updated 1 skill") {
		t.Errorf("the skills CLI's prose leaked into the notice: %q", m.notice.text)
	}

	// The matrix refreshed: the skill the wrapped run installed is a row,
	// and the re-applied disable shows in the cells.
	row := findRow(t, m, "fresh-skill")
	if row.States["pi"] != string(harness.StateOn) {
		t.Errorf("fresh-skill pi=%s, want on", row.States["pi"])
	}
	tdd := findRow(t, m, "tdd")
	if tdd.States["pi"] != string(harness.StateOff) {
		t.Errorf("tdd pi=%s after update-all, want off (sync re-applied the disable)", tdd.States["pi"])
	}
}

func TestUpdateAllSurfacesAFailedWrappedRunAndStaysUsable(t *testing.T) {
	p := tuiHome(t)
	calls := 0
	stubSkillsRunner(t, func(skillscli.Invocation) (skillscli.Result, error) {
		calls++
		if calls == 1 {
			return skillscli.Result{Stdout: "updating tdd...", Stderr: "? unexpected prompt"}, errors.New("exit status 1")
		}
		return skillscli.Result{}, nil
	})

	m := newTestModel(t, p)
	m = pressU(t, m)

	// The failure names the invocation and shows the CLI's captured
	// output raw — flattened onto the notice's one line.
	for _, want := range []string{"update failed", "skills update -g -y", "exit status 1", "updating tdd...", "? unexpected prompt"} {
		if !strings.Contains(m.notice.text, want) {
			t.Errorf("failure notice missing %q: %q", want, m.notice.text)
		}
	}
	if m.notice.kind != noticeErr {
		t.Error("the failure notice is not styled as an error")
	}

	// The verb's stop-on-failure rule: no sync ran, nothing was written.
	requireMissing(t, p.FleetStateFile(), "state file")
	requireMissing(t, p.OpenCodeConfig(), "opencode config")

	// The model is not wedged: the same key runs the update again and
	// lands the success path.
	m = pressU(t, m)
	if calls != 2 {
		t.Fatalf("%d wrapped runs, want 2", calls)
	}
	if !strings.Contains(m.notice.text, "updated all") {
		t.Errorf("retry notice: %q", m.notice.text)
	}
}

func TestUpdateAllHoldsTheCoreKeysWhileItRuns(t *testing.T) {
	p := tuiHome(t)
	stubSkillsRunner(t, func(skillscli.Invocation) (skillscli.Result, error) {
		return skillscli.Result{}, nil
	})

	m := newTestModel(t, p)
	m2, cmd := m.Update(keyU)
	m = m2.(model)
	if cmd == nil {
		t.Fatal("u did not start the update")
	}
	if m.phase != phaseUpdating {
		t.Fatalf("phase %v, want updating", m.phase)
	}

	// The busy line is the feedback while the synchronous run owns the
	// core.
	if v := m.View().Content; !strings.Contains(v, "updating all skills…") {
		t.Errorf("view missing the busy line:\n%s", v)
	}

	// While the update runs, u, r, and enter wait for idle — no second
	// wrapped run, no refresh of a store mid-rewrite, no racing apply.
	for _, k := range []tea.KeyPressMsg{keyU, keyR, keyEnter} {
		if _, cmd := m.Update(k); cmd != nil {
			t.Errorf("%v started a core operation while the update ran", k)
		}
	}
	// Navigation is model-local and still works.
	m = sendKey(m, keyDown)
	if got := m.selectedName(); got != "git-helper" {
		t.Errorf("selection after moving mid-update %q, want git-helper", got)
	}

	// The update lands, schedules the refresh, and the summary notice
	// survives it.
	m2, cmd = m.Update(cmd())
	m = m2.(model)
	if cmd == nil {
		t.Fatal("the update did not schedule a refresh")
	}
	if m.phase != phaseRefreshing {
		t.Fatalf("phase %v after the update landed, want refreshing", m.phase)
	}
	m2, cmd = m.Update(cmd())
	m = m2.(model)
	if cmd != nil {
		t.Fatal("unexpected follow-up after the refresh")
	}
	if !strings.Contains(m.notice.text, "updated all") {
		t.Errorf("update notice: %q", m.notice.text)
	}
	if v := m.View().Content; !strings.Contains(v, "updated all") {
		t.Errorf("view missing the update notice:\n%s", v)
	}
}

func TestUpdateAllIsListedInHelpAndFooter(t *testing.T) {
	p := tuiHome(t)
	m := newTestModel(t, p)

	if h := m.helpOverlay(); !strings.Contains(h, "update all installed skills") {
		t.Errorf("help overlay missing the update-all row:\n%s", h)
	}
	if f := m.footer(); !strings.Contains(f, "u update all") {
		t.Errorf("footer missing the update-all hint: %q", f)
	}
}
