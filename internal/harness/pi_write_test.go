package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
)

// runPiProject writes fixture (when non-empty), runs Project, and returns
// the report plus the file's exact content afterwards.
func runPiProject(t *testing.T, home, fixture string, writes []SkillWrite) (WriteReport, string) {
	t.Helper()
	p := paths.New(home)
	if fixture != "" {
		if err := os.MkdirAll(filepath.Dir(p.PiSettings()), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p.PiSettings(), []byte(fixture), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := NewPi(p).Project(writes)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	body, err := os.ReadFile(p.PiSettings())
	if err != nil {
		t.Fatal(err)
	}
	return rep, string(body)
}

func TestPiProjectGoldenExclusionAppended(t *testing.T) {
	fixture := `{
  "theme": "dark",
  "skills": [
    "./team-skills",
    "-skills/git-helper/SKILL.md"
  ],
  "retry": { "attempts": 3 }
}`
	want := `{
  "theme": "dark",
  "skills": [
    "./team-skills",
    "-skills/git-helper/SKILL.md",
    "-skills/tdd/SKILL.md"
  ],
  "retry": { "attempts": 3 }
}`

	// The state file tracks both skills as disabled; git-helper's entry
	// already holds, tdd's is added.
	rep, got := runPiProject(t, filepath.Join(t.TempDir(), "home"), fixture,
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

func TestPiProjectPreservesUnknownKeysAndKeyOrder(t *testing.T) {
	// Strict JSON: no comments anywhere, but unknown keys and their order
	// survive untouched.
	fixture := `{"zeta": 1, "alpha": {"nested": true}, "skills": []}`
	want := `{"zeta": 1, "alpha": {"nested": true}, "skills": ["-skills/tdd/SKILL.md"]}`

	if rep, got := runPiProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}}); got != want || len(rep.Changed) != 1 {
		t.Errorf("config = %s, changed = %v; want %s and one change", got, rep.Changed, want)
	}
}

func TestPiProjectCreatesMissingSettings(t *testing.T) {
	want := `{"skills": ["-skills/tdd/SKILL.md"]}` + "\n"

	rep, got := runPiProject(t, filepath.Join(t.TempDir(), "home"), "",
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config = %q, want %q", got, want)
	}
	if len(rep.Changed) != 1 || rep.Changed[0].To != StateOff {
		t.Errorf("Changed = %v, want one flip to off", rep.Changed)
	}
}

func TestPiProjectSatisfiedOffByExactEntryIsQuiet(t *testing.T) {
	fixture := `{"skills": ["-skills/tdd/SKILL.md"]}`
	rep, got := runPiProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != fixture {
		t.Errorf("config =\n%s\nwant untouched\n%s", got, fixture)
	}
	if !rep.Empty() {
		t.Errorf("report = %+v, want empty", rep)
	}
}

func TestPiProjectEnableRemovesExactEntryOnly(t *testing.T) {
	fixture := `{
  "skills": [
    "./team-skills",
    "-skills/tdd/SKILL.md",
    "!t*"
  ]
}`

	// The !t* glob isn't fleet's: it survives and still disables the skill.
	rep, got := runPiProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOn}})
	want := `{
  "skills": [
    "./team-skills",
    "!t*"
  ]
}`
	if got != want {
		t.Errorf("config =\n%s\nwant\n%s", got, want)
	}
	if len(rep.Changed) != 0 {
		t.Errorf("Changed = %v, want none (glob still excludes)", rep.Changed)
	}
	wantFlags := []Flag{{Skill: "tdd", Message: "still excluded by a !glob entry — left alone"}}
	if !reflect.DeepEqual(rep.Flags, wantFlags) {
		t.Errorf("Flags = %v, want %v", rep.Flags, wantFlags)
	}
}

func TestPiProjectEnableWithoutGlobLeftoverFlips(t *testing.T) {
	fixture := `{"skills": ["./team-skills", "-skills/tdd/SKILL.md"]}`
	want := `{"skills": ["./team-skills"]}`

	rep, got := runPiProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOn}})
	if got != want {
		t.Errorf("config = %s, want %s", got, want)
	}
	wantChanges := []Change{{Skill: "tdd", From: StateOff, To: StateOn}}
	if !reflect.DeepEqual(rep.Changed, wantChanges) {
		t.Errorf("Changed = %v, want %v", rep.Changed, wantChanges)
	}
}

func TestPiProjectManualEditDriftIsFlaggedNotTouched(t *testing.T) {
	// Manual exclusions for untracked skills survive and are flagged; the
	// glob is flagged as foreign.
	fixture := `{"skills": ["-skills/manual-skill/SKILL.md", "!gen-*"]}`
	rep, got := runPiProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	want := `{"skills": ["-skills/manual-skill/SKILL.md", "!gen-*", "-skills/tdd/SKILL.md"]}`
	if got != want {
		t.Errorf("config = %s, want %s", got, want)
	}
	wantFlags := []Flag{
		{Skill: "manual-skill", Message: "disabled in config but not tracked by fleet's state — left alone"},
		{Message: `skills entry "!gen-*" is not fleet's — left alone`},
	}
	if !reflect.DeepEqual(rep.Flags, wantFlags) {
		t.Errorf("Flags =\n%v\nwant\n%v", rep.Flags, wantFlags)
	}
}

func TestPiProjectGlobSatisfiedOffIsFlaggedOnce(t *testing.T) {
	// Off satisfied by a glob: the write adds nothing and the glob is
	// flagged (once — the sweep doesn't repeat the write's report).
	fixture := `{"skills": ["!tdd", "./team-skills"]}`
	rep, got := runPiProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != fixture {
		t.Errorf("config =\n%s\nwant untouched\n%s", got, fixture)
	}
	wantFlags := []Flag{{Skill: "tdd", Message: "still excluded by a !glob entry — left alone"}}
	if !reflect.DeepEqual(rep.Flags, wantFlags) {
		t.Errorf("Flags = %v, want %v", rep.Flags, wantFlags)
	}
	if len(rep.Changed) != 0 {
		t.Errorf("Changed = %v, want none", rep.Changed)
	}
}

func TestPiProjectCommentsAreRejected(t *testing.T) {
	// pi settings are strict JSON; a comment means fleet can't write the
	// file without corrupting it for pi.
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	if err := os.MkdirAll(filepath.Dir(p.PiSettings()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.PiSettings(), []byte("{ // note\n}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPi(p).Project([]SkillWrite{{Name: "tdd", State: StateOff}}); err == nil {
		t.Fatal("Project() accepted a commented settings file")
	}
}

func TestPiProjectSkillsNotAnArrayIsAnError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	if err := os.MkdirAll(filepath.Dir(p.PiSettings()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.PiSettings(), []byte(`{"skills": {"paths": []}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPi(p).Project([]SkillWrite{{Name: "tdd", State: StateOff}}); err == nil {
		t.Fatal("Project() accepted a non-array skills key")
	}
}

func TestPiProjectNoWritesNoFile(t *testing.T) {
	// Ambient sync with nothing to say must not create the settings file.
	home := filepath.Join(t.TempDir(), "home")
	if _, err := NewPi(paths.New(home)).Project(nil); err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if _, err := os.Stat(paths.New(home).PiSettings()); !os.IsNotExist(err) {
		t.Errorf("settings file created by a no-op sync: %v", err)
	}
}
