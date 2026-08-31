package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zacong/fleet/internal/paths"
)

func TestCursorDerivedStateIsAlwaysOn(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(paths.New(home).CursorDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	// No config file exists to read; Cursor scans the canonical store and
	// has no per-skill disable mechanism.
	res, err := NewCursor(paths.New(home)).Read([]string{"tdd", "git-helper"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if res.States["tdd"] != StateOn || res.States["git-helper"] != StateOn {
		t.Errorf("states = %v, want every skill on", res.States)
	}
	if res.Dialect != "" || res.SkillSources != nil || res.Linked != nil {
		t.Errorf("unexpected read result extras: %+v", res)
	}
}

func TestBobDerivedStateIsAlwaysOnWithLinkPresenceReported(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(paths.New(home).BobDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	// Bob only reaches custom skills through ~/.bob/skills links; for
	// canonical skills the links are redundant, but their presence is the
	// observable the adapter reports.
	skillsDir := paths.New(home).BobSkills()
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, ".agents", "skills", "tdd"), filepath.Join(skillsDir, "tdd")); err != nil {
		t.Fatal(err)
	}

	res, err := NewBob(paths.New(home)).Read([]string{"tdd", "git-helper"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if res.States["tdd"] != StateOn || res.States["git-helper"] != StateOn {
		t.Errorf("states = %v, want every skill on (Bob has no disable mechanism)", res.States)
	}
	if len(res.Linked) != 1 || res.Linked[0] != "tdd" {
		t.Errorf("Linked = %v, want [tdd]", res.Linked)
	}
}
