package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zacong/fleet/internal/paths"
)

func writeClaudeSettings(t *testing.T, home, body string) {
	t.Helper()
	path := paths.New(home).ClaudeSettings()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func linkSkill(t *testing.T, home, name string) {
	t.Helper()
	skillsDir := paths.New(home).ClaudeSkills()
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, ".agents", "skills", name), filepath.Join(skillsDir, name)); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeStateDependsOnLinkAndOverride(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeClaudeSettings(t, home, `{
		"skillOverrides": {
			"tdd": "off",
			"frontend-design": "user-invocable-only"
		}
	}`)
	// linked: tdd (overridden off), frontend-design (soft override), other (plain)
	linkSkill(t, home, "tdd")
	linkSkill(t, home, "frontend-design")
	linkSkill(t, home, "other")

	res, err := NewClaude(paths.New(home)).Read([]string{"tdd", "frontend-design", "other", "unlinked"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	want := map[string]State{
		"tdd":             StateOff,
		"frontend-design": StateOn, // not "off", so still discoverable
		"other":           StateOn,
		"unlinked":        StateAbsent, // claude has no native canonical access
	}
	for name, wantState := range want {
		if res.States[name] != wantState {
			t.Errorf("state for %s = %q, want %q", name, res.States[name], wantState)
		}
	}

	wantLinked := []string{"frontend-design", "other", "tdd"}
	if len(res.Linked) != len(wantLinked) {
		t.Fatalf("Linked = %v, want %v", res.Linked, wantLinked)
	}
	for i, name := range wantLinked {
		if res.Linked[i] != name {
			t.Errorf("Linked[%d] = %q, want %q", i, res.Linked[i], name)
		}
	}
}

func TestClaudeWithoutSettingsOrLinksReportsAbsent(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")

	res, err := NewClaude(paths.New(home)).Read([]string{"tdd"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if res.States["tdd"] != StateAbsent {
		t.Errorf("state = %q, want absent", res.States["tdd"])
	}
}

func TestClaudeMalformedSettingsIsAnError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeClaudeSettings(t, home, `{"skillOverrides":`)

	if _, err := NewClaude(paths.New(home)).Read([]string{"tdd"}); err == nil {
		t.Fatal("Read() on malformed settings.json should fail")
	}
}
