package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zacong/fleet/internal/paths"
)

func writeCodexConfig(t *testing.T, home, body string) {
	t.Helper()
	path := paths.New(home).CodexConfig()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCodexReadsEnabledFlags(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeCodexConfig(t, home, `# codex config
model = "gpt-5.4"

[[skills.config]]
name = "tdd"
enabled = false

[[skills.config]]
name = "git-helper"
enabled = true
`)

	res, err := NewCodex(paths.New(home)).Read([]string{"tdd", "git-helper", "other"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	want := map[string]State{"tdd": StateOff, "git-helper": StateOn, "other": StateOn}
	for name, wantState := range want {
		if res.States[name] != wantState {
			t.Errorf("state for %s = %q, want %q", name, res.States[name], wantState)
		}
	}
}

func TestCodexLaterEntriesOverrideEarlier(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeCodexConfig(t, home, `
[[skills.config]]
name = "tdd"
enabled = false

[[skills.config]]
name = "tdd"
enabled = true
`)

	res, err := NewCodex(paths.New(home)).Read([]string{"tdd"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if res.States["tdd"] != StateOn {
		t.Errorf("state = %q, want on (later entry wins)", res.States["tdd"])
	}
}

func TestCodexReadsPathSelectors(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeCodexConfig(t, home, `
[[skills.config]]
path = "/home/fake/.agents/skills/git-helper/SKILL.md"
enabled = false
`)

	res, err := NewCodex(paths.New(home)).Read([]string{"git-helper"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if res.States["git-helper"] != StateOff {
		t.Errorf("state = %q, want off (path selector names the skill)", res.States["git-helper"])
	}
}

func TestCodexEntryWithoutEnabledFlagChangesNothing(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeCodexConfig(t, home, `
[[skills.config]]
name = "tdd"
`)

	res, err := NewCodex(paths.New(home)).Read([]string{"tdd"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if res.States["tdd"] != StateOn {
		t.Errorf("state = %q, want on", res.States["tdd"])
	}
}

func TestCodexMalformedConfigIsAnError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeCodexConfig(t, home, `[[skills.config]]`+"\nname = ")

	if _, err := NewCodex(paths.New(home)).Read([]string{"tdd"}); err == nil {
		t.Fatal("Read() on malformed TOML should fail")
	}
}
