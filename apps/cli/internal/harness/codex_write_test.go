package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
)

// runCodexProject writes fixture (when non-empty), runs Project, and
// returns the report plus the file's exact content afterwards.
func runCodexProject(t *testing.T, home, fixture string, writes []SkillWrite) (WriteReport, string) {
	t.Helper()
	p := paths.New(home)
	if fixture != "" {
		if err := os.MkdirAll(p.CodexDir(), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p.CodexConfig(), []byte(fixture), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := NewCodex(p).Project(writes)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	body, err := os.ReadFile(p.CodexConfig())
	if err != nil {
		t.Fatal(err)
	}
	return rep, string(body)
}

func TestCodexProjectGoldenBlockAppended(t *testing.T) {
	fixture := `# Codex configuration
model = "gpt-5.2"

[features]
web_search = true # user comment
`
	want := `# Codex configuration
model = "gpt-5.2"

[features]
web_search = true # user comment

[[skills.config]]
name = "tdd"
enabled = false
`

	rep, got := runCodexProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config =\n%q\nwant\n%q", got, want)
	}
	wantChanges := []Change{{Skill: "tdd", From: StateOn, To: StateOff}}
	if !reflect.DeepEqual(rep.Changed, wantChanges) {
		t.Errorf("Changed = %v, want %v", rep.Changed, wantChanges)
	}
	if len(rep.Flags) != 0 {
		t.Errorf("Flags = %v, want none", rep.Flags)
	}
}

func TestCodexProjectCreatesMissingConfig(t *testing.T) {
	want := `[[skills.config]]
name = "tdd"
enabled = false
`

	rep, got := runCodexProject(t, filepath.Join(t.TempDir(), "home"), "",
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config =\n%q\nwant\n%q", got, want)
	}
	if len(rep.Changed) != 1 || rep.Changed[0].To != StateOff {
		t.Errorf("Changed = %v, want one flip to off", rep.Changed)
	}
}

func TestCodexProjectFlipsExistingFleetBlock(t *testing.T) {
	fixture := `[[skills.config]]
name = "tdd"
enabled = true # keep an eye on this
`
	want := `[[skills.config]]
name = "tdd"
enabled = false # keep an eye on this
`

	rep, got := runCodexProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config =\n%s\nwant\n%s", got, want)
	}
	if len(rep.Changed) != 1 || rep.Changed[0].From != StateOn || rep.Changed[0].To != StateOff {
		t.Errorf("Changed = %v, want one on->off flip", rep.Changed)
	}
}

func TestCodexProjectSatisfiedOffQuietWhenFleetsOwn(t *testing.T) {
	fixture := `[[skills.config]]
name = "tdd"
enabled = false
`
	rep, got := runCodexProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != fixture {
		t.Errorf("config =\n%s\nwant untouched\n%s", got, fixture)
	}
	if !rep.Empty() {
		t.Errorf("report = %+v, want empty", rep)
	}
}

func TestCodexProjectEnableRemovesFleetBlocks(t *testing.T) {
	fixture := `# config
[[skills.config]]
name = "tdd"
enabled = false

[[skills.config]]
name = "git-helper"
enabled = false

[[skills.config]]
name = "tdd"
enabled = false
`
	want := `# config
[[skills.config]]
name = "git-helper"
enabled = false
`

	rep, got := runCodexProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOn}, {Name: "git-helper", State: StateOff}})
	if got != want {
		t.Errorf("config =\n%q\nwant\n%q", got, want)
	}
	wantChanges := []Change{{Skill: "tdd", From: StateOff, To: StateOn}}
	if !reflect.DeepEqual(rep.Changed, wantChanges) {
		t.Errorf("Changed = %v, want %v", rep.Changed, wantChanges)
	}
}

func TestCodexProjectEnableFlagsForeignEntryStillDisabling(t *testing.T) {
	// A path-selector entry isn't fleet's: removal of fleet's own block
	// still leaves the skill disabled, and the foreign entry is flagged.
	fixture := `[[skills.config]]
name = "tdd"
enabled = false

[[skills.config]]
path = "~/.agents/skills/tdd/SKILL.md"
enabled = false
`
	want := `[[skills.config]]
path = "~/.agents/skills/tdd/SKILL.md"
enabled = false
`

	rep, got := runCodexProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOn}})
	if got != want {
		t.Errorf("config =\n%s\nwant\n%s", got, want)
	}
	if len(rep.Changed) != 0 {
		t.Errorf("Changed = %v, want none (path entry still disables)", rep.Changed)
	}
	wantFlags := []Flag{{Skill: "tdd", Message: "still disabled by a skills.config entry fleet doesn't manage — left alone"}}
	if !reflect.DeepEqual(rep.Flags, wantFlags) {
		t.Errorf("Flags = %v, want %v", rep.Flags, wantFlags)
	}
}

func TestCodexProjectManualEditDriftIsFlaggedNotTouched(t *testing.T) {
	// Blocks for skills fleet isn't tracking survive and are flagged.
	fixture := `[[skills.config]]
name = "manual-skill"
enabled = false
`
	rep, got := runCodexProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	want := fixture + `
[[skills.config]]
name = "tdd"
enabled = false
`
	if got != want {
		t.Errorf("config =\n%q\nwant\n%q", got, want)
	}
	wantFlags := []Flag{{Skill: "manual-skill", Message: "disabled in config but not tracked by fleet's state — left alone"}}
	if !reflect.DeepEqual(rep.Flags, wantFlags) {
		t.Errorf("Flags = %v, want %v", rep.Flags, wantFlags)
	}
}

func TestCodexProjectNonSimpleBlockIsLeftAlone(t *testing.T) {
	// Extra keys make the block foreign to fleet: it stays byte-identical
	// and the skill keeps its state, so the off write is satisfied by it
	// and flagged.
	fixture := `[[skills.config]]
name = "tdd"
enabled = false
priority = 5
`
	rep, got := runCodexProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != fixture {
		t.Errorf("config =\n%s\nwant untouched\n%s", got, fixture)
	}
	wantFlags := []Flag{{Skill: "tdd", Message: "still disabled by a skills.config entry fleet doesn't manage — left alone"}}
	if !reflect.DeepEqual(rep.Flags, wantFlags) {
		t.Errorf("Flags = %v, want %v", rep.Flags, wantFlags)
	}
	if len(rep.Changed) != 0 {
		t.Errorf("Changed = %v, want none", rep.Changed)
	}
}

func TestCodexProjectHeaderCommentPreservedOnRemove(t *testing.T) {
	fixture := `[[skills.config]] # fleet managed
name = "tdd"
enabled = false
`
	rep, got := runCodexProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOn}})
	if strings.TrimSpace(got) != "" {
		t.Errorf("config = %q, want empty", got)
	}
	// Removing fleet's block is itself the off->on flip.
	wantChanges := []Change{{Skill: "tdd", From: StateOff, To: StateOn}}
	if !reflect.DeepEqual(rep.Changed, wantChanges) {
		t.Errorf("Changed = %v, want %v", rep.Changed, wantChanges)
	}
}

func TestCodexProjectMalformedTOMLIsAnError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	if err := os.MkdirAll(p.CodexDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.CodexConfig(), []byte("not [ toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCodex(p).Project([]SkillWrite{{Name: "tdd", State: StateOff}}); err == nil {
		t.Fatal("Project() accepted malformed TOML")
	}
}

func TestCodexProjectNoWritesNoFile(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if _, err := NewCodex(paths.New(home)).Project(nil); err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if _, err := os.Stat(paths.New(home).CodexConfig()); !os.IsNotExist(err) {
		t.Errorf("config created by a no-op sync: %v", err)
	}
}
