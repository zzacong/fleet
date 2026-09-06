package customs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
)

// destinationHome builds a fake home with no legacy repo pointer, the given
// explicit repo roots in config, and the given fleet-home checkout slots on
// disk. Store skills are written for adopt-move tests.
func destinationHome(t *testing.T, explicit []string, slots []string, storeSkills ...string) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	t.Setenv("FLEET_REPO", "")
	cfgDir := filepath.Dir(p.FleetConfigFile())
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if len(explicit) > 0 {
		quoted := make([]string, len(explicit))
		for i, r := range explicit {
			quoted[i] = `"` + r + `"`
		}
		body := `{"skillsRepos": [` + strings.Join(quoted, ", ") + `]}`
		if err := os.WriteFile(p.FleetConfigFile(), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range slots {
		if err := os.MkdirAll(filepath.Join(p.FleetReposDir(), name), 0o755); err != nil {
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

func TestAdoptCandidatesOrdersTrackedThenFallback(t *testing.T) {
	home := t.TempDir()
	outsideB := filepath.Join(home, "explicit-b")
	outsideA := filepath.Join(home, "explicit-a")
	p := destinationHome(t, []string{outsideB, outsideA}, []string{"zeta", "alpha"})
	t.Setenv("FLEET_REPO", "")

	got, err := AdoptCandidates(p)
	if err != nil {
		t.Fatalf("AdoptCandidates() error = %v", err)
	}
	want := []string{
		filepath.Join(outsideB, "skills"),
		filepath.Join(outsideA, "skills"),
		filepath.Join(p.FleetReposDir(), "alpha", "skills"),
		filepath.Join(p.FleetReposDir(), "zeta", "skills"),
		p.FleetHomeSkills(),
	}
	if len(got) != len(want) {
		t.Fatalf("AdoptCandidates() = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AdoptCandidates() = %q, want %q", got, want)
		}
	}
}

func TestAdoptCandidatesFallbackOnlyWhenNothingTracked(t *testing.T) {
	p := destinationHome(t, nil, nil)
	t.Setenv("FLEET_REPO", "")

	got, err := AdoptCandidates(p)
	if err != nil {
		t.Fatalf("AdoptCandidates() error = %v", err)
	}
	if len(got) != 1 || got[0] != p.FleetHomeSkills() {
		t.Errorf("AdoptCandidates() = %q, want [%q]", got, p.FleetHomeSkills())
	}
}

func TestAdoptToMovesStoreSkillIntoExplicitTarget(t *testing.T) {
	p := destinationHome(t, nil, nil, "my-notes")
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
	p := destinationHome(t, []string{outside}, nil, "my-notes")
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
	p := destinationHome(t, nil, nil)
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

func TestAdoptToRefusesDoublePresenceAcrossTrackedSet(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "tracked")
	p := destinationHome(t, []string{outside}, nil, "my-notes")
	writeSkill(t, filepath.Join(outside, "skills"), "my-notes")

	_, err := AdoptTo(p, "my-notes", p.FleetHomeSkills())
	if err == nil || !strings.Contains(err.Error(), "both") {
		t.Errorf("AdoptTo with store+tracked copies should refuse, got %v", err)
	}
}
