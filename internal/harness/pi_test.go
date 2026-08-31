package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zacong/fleet/internal/paths"
)

func writePiSettings(t *testing.T, home, body string) {
	t.Helper()
	path := paths.New(home).PiSettings()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPiReadsExactPathExclusions(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writePiSettings(t, home, `{
		"theme": "dark",
		"skills": [
			"~/.claude/skills",
			"-skills/ui-ux-pro-max/SKILL.md",
			"-skills/web-design-guidelines/SKILL.md"
		]
	}`)

	res, err := NewPi(paths.New(home)).Read([]string{"tdd", "ui-ux-pro-max", "web-design-guidelines"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	want := map[string]State{
		"tdd":                   StateOn,
		"ui-ux-pro-max":         StateOff,
		"web-design-guidelines": StateOff,
	}
	for name, wantState := range want {
		if res.States[name] != wantState {
			t.Errorf("state for %s = %q, want %q", name, res.States[name], wantState)
		}
	}
}

func TestPiReadsGlobExclusions(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writePiSettings(t, home, `{"skills": ["!git-*", "-skills/tdd/SKILL.md"]}`)

	res, err := NewPi(paths.New(home)).Read([]string{"git-helper", "git-secret", "tdd", "other"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	want := map[string]State{
		"git-helper": StateOff,
		"git-secret": StateOff,
		"tdd":        StateOff,
		"other":      StateOn,
	}
	for name, wantState := range want {
		if res.States[name] != wantState {
			t.Errorf("state for %s = %q, want %q", name, res.States[name], wantState)
		}
	}
}

func TestPiPlainPathEntriesNeverDisable(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writePiSettings(t, home, `{"skills": ["~/.claude/skills", "/opt/company-skills"]}`)

	res, err := NewPi(paths.New(home)).Read([]string{"tdd"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if res.States["tdd"] != StateOn {
		t.Errorf("state = %q, want on", res.States["tdd"])
	}
}

func TestPiMalformedSettingsIsAnError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writePiSettings(t, home, `{"skills": [`)

	if _, err := NewPi(paths.New(home)).Read([]string{"tdd"}); err == nil {
		t.Fatal("Read() on malformed settings.json should fail")
	}
}
