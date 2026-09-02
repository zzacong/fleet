package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
)

// makeClaudeSkill builds a fake canonical-store skill and a link in
// ~/.claude/skills so claude can discover it.
func makeClaudeSkill(t *testing.T, home, name string) {
	t.Helper()
	p := paths.New(home)
	if err := os.MkdirAll(filepath.Join(p.ClaudeSkills(), name), 0o755); err != nil {
		t.Fatal(err)
	}
}

// runClaudeProject writes fixture (when non-empty), runs Project, and
// returns the report plus the file's exact content afterwards.
func runClaudeProject(t *testing.T, home, fixture string, writes []SkillWrite) (WriteReport, string) {
	t.Helper()
	p := paths.New(home)
	if fixture != "" {
		if err := os.MkdirAll(filepath.Dir(p.ClaudeSettings()), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p.ClaudeSettings(), []byte(fixture), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := NewClaude(p).Project(writes)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	body, err := os.ReadFile(p.ClaudeSettings())
	if err != nil {
		t.Fatal(err)
	}
	return rep, string(body)
}

func TestClaudeProjectGoldenOverrideSet(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	makeClaudeSkill(t, home, "tdd")

	fixture := `{
  "model": "sonnet",
  "skillOverrides": {
    "git-helper": "off"
  }
}`
	want := `{
  "model": "sonnet",
  "skillOverrides": {
    "git-helper": "off",
    "tdd": "off"
  }
}`

	rep, got := runClaudeProject(t, home, fixture,
		[]SkillWrite{{Name: "git-helper", State: StateOff}, {Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config =\n%s\nwant\n%s", got, want)
	}
	wantChanges := []Change{{Skill: "tdd", From: StateOn, To: StateOff}}
	if !reflect.DeepEqual(rep.Changed, wantChanges) {
		t.Errorf("Changed = %v, want %v", rep.Changed, wantChanges)
	}
	if len(rep.Flags) != 0 {
		t.Errorf("Flags = %v, want none", rep.Flags)
	}
}

func TestClaudeProjectCreatesMissingSettings(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	makeClaudeSkill(t, home, "tdd")

	want := `{"skillOverrides": {"tdd": "off"}}` + "\n"

	rep, got := runClaudeProject(t, home, "",
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config = %q, want %q", got, want)
	}
	if len(rep.Changed) != 1 || rep.Changed[0].To != StateOff {
		t.Errorf("Changed = %v, want one flip to off", rep.Changed)
	}
}

func TestClaudeProjectEnableRemovesOverrideAndEmptyObject(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	makeClaudeSkill(t, home, "tdd")

	fixture := `{"model": "sonnet", "skillOverrides": {"tdd": "off"}}`
	want := `{"model": "sonnet"}`

	rep, got := runClaudeProject(t, home, fixture,
		[]SkillWrite{{Name: "tdd", State: StateOn}})
	if got != want {
		t.Errorf("config = %s, want %s", got, want)
	}
	wantChanges := []Change{{Skill: "tdd", From: StateOff, To: StateOn}}
	if !reflect.DeepEqual(rep.Changed, wantChanges) {
		t.Errorf("Changed = %v, want %v", rep.Changed, wantChanges)
	}
}

func TestClaudeProjectEnableKeepsNonEmptyOverrides(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	makeClaudeSkill(t, home, "tdd")
	makeClaudeSkill(t, home, "git-helper")

	fixture := `{"skillOverrides": {"tdd": "off", "git-helper": "off"}}`
	// git-helper's own lead separator survives the deletion verbatim.
	want := `{"skillOverrides": { "git-helper": "off"}}`

	_, got := runClaudeProject(t, home, fixture,
		[]SkillWrite{{Name: "tdd", State: StateOn}, {Name: "git-helper", State: StateOff}})
	if got != want {
		t.Errorf("config = %s, want %s", got, want)
	}
}

func TestClaudeProjectAbsentSkillIsLeftAlone(t *testing.T) {
	// Without a link, claude cannot discover the skill: there is nothing
	// to disable, and fleet writes nothing. Doctor explains the missing
	// link later.
	home := filepath.Join(t.TempDir(), "home")
	fixture := `{"model": "sonnet"}`
	rep, got := runClaudeProject(t, home, fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != fixture {
		t.Errorf("config =\n%s\nwant untouched\n%s", got, fixture)
	}
	if !rep.Empty() {
		t.Errorf("report = %+v, want empty", rep)
	}
}

func TestClaudeProjectEnableRemovesStaleOverrideForUnlinkedSkill(t *testing.T) {
	// A skill claude can no longer discover (no link) with a leftover
	// "off" override: the state says on, so restoring removes the stale
	// entry — otherwise no command could ever converge this config.
	home := filepath.Join(t.TempDir(), "home")

	fixture := `{"model": "sonnet", "skillOverrides": {"tdd": "off"}}`
	want := `{"model": "sonnet"}`

	rep, got := runClaudeProject(t, home, fixture,
		[]SkillWrite{{Name: "tdd", State: StateOn}})
	if got != want {
		t.Errorf("config = %s, want %s", got, want)
	}
	wantChanges := []Change{{Skill: "tdd", From: StateAbsent, To: StateOn}}
	if !reflect.DeepEqual(rep.Changed, wantChanges) {
		t.Errorf("Changed = %v, want %v", rep.Changed, wantChanges)
	}
}

func TestClaudeProjectManualEditDriftIsFlaggedNotTouched(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	makeClaudeSkill(t, home, "tdd")
	makeClaudeSkill(t, home, "manual-skill")

	fixture := `{"skillOverrides": {"manual-skill": "off"}}`
	// tdd is added (managed, read on); manual-skill is flagged and kept.
	want := `{"skillOverrides": {"manual-skill": "off", "tdd": "off"}}`

	rep, got := runClaudeProject(t, home, fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config = %s, want %s", got, want)
	}
	wantFlags := []Flag{{Skill: "manual-skill", Message: "disabled in config but not tracked by fleet's state — left alone"}}
	if !reflect.DeepEqual(rep.Flags, wantFlags) {
		t.Errorf("Flags = %v, want %v", rep.Flags, wantFlags)
	}
}

func TestClaudeProjectNonOffValuesPassSilently(t *testing.T) {
	// "user-invocable-only" still loads the skill: not a disable, so the
	// sweep has nothing to say about it.
	home := filepath.Join(t.TempDir(), "home")
	makeClaudeSkill(t, home, "tdd")
	makeClaudeSkill(t, home, "other")

	fixture := `{"skillOverrides": {"other": "user-invocable-only"}}`
	rep, got := runClaudeProject(t, home, fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	want := `{"skillOverrides": {"other": "user-invocable-only", "tdd": "off"}}`
	if got != want {
		t.Errorf("config = %s, want %s", got, want)
	}
	if len(rep.Flags) != 0 {
		t.Errorf("Flags = %v, want none", rep.Flags)
	}
}

func TestClaudeProjectCommentsAreRejected(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	if err := os.MkdirAll(filepath.Dir(p.ClaudeSettings()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ClaudeSettings(), []byte("{ /* no */ }"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewClaude(p).Project([]SkillWrite{{Name: "tdd", State: StateOff}}); err == nil {
		t.Fatal("Project() accepted a commented settings file")
	}
}

func TestClaudeProjectNoWritesNoFile(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if _, err := NewClaude(paths.New(home)).Project(nil); err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if _, err := os.Stat(paths.New(home).ClaudeSettings()); !os.IsNotExist(err) {
		t.Errorf("settings file created by a no-op sync: %v", err)
	}
}
