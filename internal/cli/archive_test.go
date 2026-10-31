package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/state"
)

// runArchive runs `fleet skill archive` against a fake home and returns
// stdout and the error.
func runArchive(t *testing.T, p *paths.Paths, args ...string) (string, error) {
	t.Helper()
	out := &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"skill", "archive"}, args...))
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	err := root.Execute()
	return out.String(), err
}

func TestArchiveMovesSkillUnlinksEveryHarnessAndDropsState(t *testing.T) {
	p := harnessHome(t)
	collection := filepath.Join(t.TempDir(), "customs")
	writeSkillDir(t, collection, "alpha", "Alpha.")
	writeSkillDir(t, collection, "beta", "Beta.")
	if _, _, err := runAddDir(t, p, collection); err != nil {
		t.Fatalf("add-dir: %v", err)
	}
	// Claude holds a state entry for alpha, which archive must drop.
	runToggle(t, p, "off", "alpha", "--harness", "claude")

	out, err := runArchive(t, p, "alpha")
	if err != nil {
		t.Fatalf("archive: %v\nout=%s", err, out)
	}

	archived := filepath.Join(filepath.Dir(collection), "archived", "alpha")
	if _, serr := os.Stat(filepath.Join(archived, "SKILL.md")); serr != nil {
		t.Errorf("alpha not moved to %s: %v", archived, serr)
	}
	if _, serr := os.Lstat(filepath.Join(collection, "alpha")); !os.IsNotExist(serr) {
		t.Errorf("alpha still in collection: %v", serr)
	}
	if _, serr := os.Stat(filepath.Join(collection, "beta", "SKILL.md")); serr != nil {
		t.Errorf("beta was disturbed: %v", serr)
	}
	if !strings.Contains(out, `archive: archived "`+archived+`"`) {
		t.Errorf("output missing headline:\n%s", out)
	}

	harnessDirs := map[string]string{
		"opencode": p.OpenCodeSkills(),
		"pi":       p.PiSkills(),
		"codex":    p.CodexSkills(),
		"claude":   p.ClaudeSkills(),
		"cursor":   p.CursorSkills(),
		"bob":      p.BobSkills(),
	}
	for h, dir := range harnessDirs {
		want := h + `: unlinked "alpha" → ` + filepath.Join(collection, "alpha")
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
		if _, rerr := os.Lstat(filepath.Join(dir, "alpha")); !os.IsNotExist(rerr) {
			t.Errorf("link %s/alpha not removed: %v", dir, rerr)
		}
		if _, rerr := os.Lstat(filepath.Join(dir, "beta")); rerr != nil {
			t.Errorf("beta link in %s was disturbed: %v", dir, rerr)
		}
	}

	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatalf("state file: %v", err)
	}
	if slices.Contains(st.Names(), "alpha") {
		t.Errorf("state still holds alpha: %v", st.Names())
	}

	if ls := runLs(t, p); strings.Contains(ls, "alpha") {
		t.Errorf("ls still lists the archived skill:\n%s", ls)
	}
}

func TestArchiveFallbackSkillGoesToFleetHomeArchived(t *testing.T) {
	p := harnessHome(t)
	writeSkillDir(t, p.FleetHomeSkills(), "notes", "Notes.")
	if _, _, err := runSync(t, p); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if _, err := runArchive(t, p, "notes"); err != nil {
		t.Fatalf("archive: %v", err)
	}

	archived := filepath.Join(p.FleetConfigDir(), "archived", "notes")
	if _, serr := os.Stat(filepath.Join(archived, "SKILL.md")); serr != nil {
		t.Errorf("notes not moved to %s: %v", archived, serr)
	}
	if _, serr := os.Lstat(filepath.Join(p.FleetHomeSkills(), "notes")); !os.IsNotExist(serr) {
		t.Errorf("notes still in the fallback: %v", serr)
	}
	if _, rerr := os.Lstat(filepath.Join(p.OpenCodeSkills(), "notes")); !os.IsNotExist(rerr) {
		t.Errorf("fallback link not removed: %v", rerr)
	}
}

func TestArchiveFailedMoveLeavesLinksAndStateAlone(t *testing.T) {
	p := harnessHome(t)
	collection := filepath.Join(t.TempDir(), "customs")
	writeSkillDir(t, collection, "alpha", "Alpha.")
	if _, _, err := runAddDir(t, p, collection); err != nil {
		t.Fatalf("add-dir: %v", err)
	}
	runToggle(t, p, "off", "alpha", "--harness", "claude")
	stateBefore := readFile(t, p.FleetStateFile())
	// A file where archived/ must go makes the move impossible.
	if err := os.WriteFile(filepath.Join(filepath.Dir(collection), "archived"), []byte("in the way"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := runArchive(t, p, "alpha"); err == nil {
		t.Fatal("archive into a blocked archived/ should fail")
	}
	if _, serr := os.Stat(filepath.Join(collection, "alpha", "SKILL.md")); serr != nil {
		t.Errorf("failed archive lost alpha from the collection: %v", serr)
	}
	for _, dir := range []string{p.OpenCodeSkills(), p.PiSkills(), p.CodexSkills(), p.ClaudeSkills(), p.CursorSkills(), p.BobSkills()} {
		if _, rerr := os.Lstat(filepath.Join(dir, "alpha")); rerr != nil {
			t.Errorf("failed archive unlinked %s/alpha: %v", dir, rerr)
		}
	}
	if got := readFile(t, p.FleetStateFile()); got != stateBefore {
		t.Errorf("failed archive changed the state file:\nbefore:\n%s\nafter:\n%s", stateBefore, got)
	}
}
