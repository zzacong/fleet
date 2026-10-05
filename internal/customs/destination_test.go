package customs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

// destinationHome builds a fake home with no retired repo pointer, the
// given tracked roots' skills/ collections in config, and harness dirs
// installed. Store skills are written for adopt-move tests.
func destinationHome(t *testing.T, roots []string, storeSkills ...string) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	cfgDir := filepath.Dir(p.FleetConfigFile())
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if len(roots) > 0 {
		f, err := config.Load(p.FleetConfigFile())
		if err != nil {
			t.Fatal(err)
		}
		collections := make([]string, len(roots))
		for i, root := range roots {
			collections[i] = filepath.Join(root, "skills")
		}
		f.SetSkillsDirs(collections)
		if err := config.Save(p.FleetConfigFile(), f); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range storeSkills {
		writeSkill(t, p.SkillsStore(), name)
	}
	return p
}

func TestAdoptToMovesStoreSkillIntoExplicitTarget(t *testing.T) {
	p := destinationHome(t, nil, "my-notes")
	target := filepath.Join(t.TempDir(), "one-off", "skills")

	rep, err := AdoptTo(p, "my-notes", target)
	if err != nil {
		t.Fatalf("AdoptTo() error = %v", err)
	}
	if !rep.Moved {
		t.Error("AdoptTo should have moved the store skill")
	}
	if rep.To != filepath.Join(target, "my-notes") {
		t.Errorf("rep.To = %q, want %q", rep.To, filepath.Join(target, "my-notes"))
	}
	if _, err := os.Stat(filepath.Join(target, "my-notes", "SKILL.md")); err != nil {
		t.Errorf("skill not moved into explicit target: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.SkillsStore(), "my-notes")); !os.IsNotExist(err) {
		t.Errorf("skill still in canonical store: %v", err)
	}
	// AdoptTo never writes config.
	if _, err := os.Stat(p.FleetConfigFile()); !os.IsNotExist(err) {
		t.Errorf("AdoptTo wrote config: %v", err)
	}
}

func TestAdoptToProceedsIntoUnscannedTarget(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "tracked")
	p := destinationHome(t, []string{outside}, "my-notes")
	unscanned := filepath.Join(t.TempDir(), "elsewhere", "skills")

	rep, err := AdoptTo(p, "my-notes", unscanned)
	if err != nil {
		t.Fatalf("AdoptTo into unscanned target should proceed, got %v", err)
	}
	if !rep.Moved || rep.To != filepath.Join(unscanned, "my-notes") {
		t.Errorf("rep = %+v, want move into unscanned target", rep)
	}
}

func TestAdoptToReEnsuresWhenAlreadyThere(t *testing.T) {
	p := destinationHome(t, nil)
	writeSkill(t, p.FleetHomeSkills(), "my-notes")

	rep, err := AdoptTo(p, "my-notes", p.FleetHomeSkills())
	if err != nil {
		t.Fatalf("AdoptTo() error = %v", err)
	}
	if rep.Moved {
		t.Error("nothing should move for an already-adopted skill")
	}
	if rep.To != filepath.Join(p.FleetHomeSkills(), "my-notes") {
		t.Errorf("rep.To = %q, want fleet-home", rep.To)
	}
}

func TestAdoptToReEnsuresDespiteSameHomeDuplicateName(t *testing.T) {
	// Two directories in one collection declare the same frontmatter name.
	// That is not double presence across homes, so adopt re-ensures the
	// directory match instead of refusing.
	p := destinationHome(t, nil)
	writeSkill(t, p.FleetHomeSkills(), "foo")
	alias := filepath.Join(p.FleetHomeSkills(), "bar", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(alias), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: foo\ndescription: alias\n---\n\n# bar\n"
	if err := os.WriteFile(alias, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := AdoptTo(p, "foo", p.FleetHomeSkills())
	if err != nil {
		t.Fatalf("same-home duplicate must not read as double presence: %v", err)
	}
	if rep.To != filepath.Join(p.FleetHomeSkills(), "foo") {
		t.Errorf("rep.To = %q, want the directory match", rep.To)
	}
}

func TestAdoptToRefusesDoublePresenceAcrossTrackedSet(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "tracked")
	p := destinationHome(t, []string{outside}, "my-notes")
	writeSkill(t, filepath.Join(outside, "skills"), "my-notes")

	_, err := AdoptTo(p, "my-notes", p.FleetHomeSkills())
	if err == nil || !strings.Contains(err.Error(), "both") {
		t.Errorf("AdoptTo with store+tracked copies should refuse, got %v", err)
	}
}
