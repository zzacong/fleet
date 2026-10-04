package customs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

func TestMakeVisibleLinksEverySkill(t *testing.T) {
	p, collection := adoptHome(t, nil, []string{"my-notes", "other"})

	vis, err := MakeVisible(p, collection)
	if err != nil {
		t.Fatal(err)
	}
	// 2 skills x 6 harnesses.
	if len(vis.Linked) != 12 {
		t.Errorf("linked = %d entries, want 12 (2 skills x 6 harnesses)", len(vis.Linked))
	}
	if got, err := os.Readlink(filepath.Join(p.OpenCodeSkills(), "other")); err != nil {
		t.Errorf("opencode link for other skill missing: %v", err)
	} else if want := filepath.Join(collection, "other"); got != want {
		t.Errorf("opencode link = %q, want %q", got, want)
	}
	if got, err := os.Readlink(filepath.Join(p.CodexSkills(), "other")); err != nil {
		t.Errorf("codex link for other skill missing: %v", err)
	} else if want := filepath.Join(collection, "other"); got != want {
		t.Errorf("codex link = %q, want %q", got, want)
	}
}

func TestMakeVisibleIsIdempotent(t *testing.T) {
	p, collection := adoptHome(t, nil, []string{"my-notes"})

	if _, err := MakeVisible(p, collection); err != nil {
		t.Fatal(err)
	}
	vis, err := MakeVisible(p, collection)
	if err != nil {
		t.Fatal(err)
	}
	if len(vis.Linked) != 0 {
		t.Errorf("second run reported linked=%v, want none", vis.Linked)
	}
}

func TestEnsureVisibleLinksEveryCustomHome(t *testing.T) {
	// The explicit tracked collection and the fleet-home fallback are both
	// scanned custom homes; sync makes both visible in one run.
	p, collection := adoptHome(t, nil, []string{"my-notes"})
	writeSkill(t, p.FleetHomeSkills(), "scratch")

	vis, err := EnsureVisible(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(vis.Linked) != 12 {
		t.Errorf("linked = %d, want 12 (2 skills x 6 harnesses)", len(vis.Linked))
	}
	if _, err := os.Readlink(filepath.Join(p.BobSkills(), "scratch")); err != nil {
		t.Errorf("fallback skill not linked: %v", err)
	}
	if _, err := os.Readlink(filepath.Join(p.BobSkills(), "my-notes")); err != nil {
		t.Errorf("collection skill not linked: %v", err)
	}
	if target, _ := os.Readlink(filepath.Join(p.CodexSkills(), "my-notes")); target != filepath.Join(collection, "my-notes") {
		t.Errorf("collection link target = %q, want the collection", target)
	}
}

func TestWithdrawUndoesMakeVisible(t *testing.T) {
	p, collection := adoptHome(t, nil, []string{"my-notes"})

	if _, err := MakeVisible(p, collection); err != nil {
		t.Fatal(err)
	}
	wd, err := Withdraw(p, collection)
	if err != nil {
		t.Fatal(err)
	}
	if len(wd.Unlinked) != 6 {
		t.Errorf("unlinked = %v, want 6 (1 skill x 6 harnesses)", wd.Unlinked)
	}
	if _, err := os.Lstat(filepath.Join(p.CodexSkills(), "my-notes")); !os.IsNotExist(err) {
		t.Errorf("codex link should be gone, err = %v", err)
	}
}

// trackedHome builds a fake home with every harness installed and the given
// repo roots recorded in config in precedence order. It returns the paths
// and one collection dir per root.
func trackedHome(t *testing.T, roots ...string) (*paths.Paths, []string) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	collections := make([]string, len(roots))
	for i, root := range roots {
		collections[i] = filepath.Join(root, "skills")
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetSkillsDirs(collections)
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}
	return p, collections
}

// harnessSkillDirs returns every installed harness's skills dir, the places
// a managed custom link lands.
func harnessSkillDirs(p *paths.Paths) []string {
	return []string{p.OpenCodeSkills(), p.PiSkills(), p.CodexSkills(), p.ClaudeSkills(), p.CursorSkills(), p.BobSkills()}
}

func TestEnsureVisibleHighestPrecedenceHomeWinsCollision(t *testing.T) {
	// The same name in two tracked homes must link to the first home in
	// tracked order — the same winner the listing shows — in every harness.
	first, second := t.TempDir(), t.TempDir()
	p, collections := trackedHome(t, first, second)
	writeSkill(t, collections[0], "dup")
	writeSkill(t, collections[1], "dup")

	vis, err := EnsureVisible(p)
	if err != nil {
		t.Fatal(err)
	}
	wantTarget := filepath.Join(collections[0], "dup")
	if len(vis.Linked) != 6 {
		t.Errorf("linked = %d entries, want 6 (winner only x 6 harnesses)", len(vis.Linked))
	}
	for _, dir := range harnessSkillDirs(p) {
		if got, err := os.Readlink(filepath.Join(dir, "dup")); err != nil || got != wantTarget {
			t.Errorf("link in %s = %q, %v; want the first tracked home %q", dir, got, err, wantTarget)
		}
	}
}

func TestEnsureVisibleCollisionIsIdempotent(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	p, collections := trackedHome(t, first, second)
	writeSkill(t, collections[0], "dup")
	writeSkill(t, collections[1], "dup")

	if _, err := EnsureVisible(p); err != nil {
		t.Fatal(err)
	}
	vis, err := EnsureVisible(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(vis.Linked) != 0 {
		t.Errorf("second run reported linked=%v, want none", vis.Linked)
	}
}

func TestEnsureVisibleNextHomeTakesOverAfterWinnerRemoved(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	p, collections := trackedHome(t, first, second)
	writeSkill(t, collections[0], "dup")
	writeSkill(t, collections[1], "dup")

	if _, err := EnsureVisible(p); err != nil {
		t.Fatal(err)
	}
	wantFirst := filepath.Join(collections[0], "dup")
	for _, dir := range harnessSkillDirs(p) {
		if got, err := os.Readlink(filepath.Join(dir, "dup")); err != nil || got != wantFirst {
			t.Fatalf("link in %s = %q, %v; want the first tracked home %q", dir, got, err, wantFirst)
		}
	}
	// The winning home disappears from disk; the next tracked home must
	// take over its link on the next sync.
	if err := os.RemoveAll(first); err != nil {
		t.Fatal(err)
	}
	vis, err := EnsureVisible(p)
	if err != nil {
		t.Fatal(err)
	}
	wantTarget := filepath.Join(collections[1], "dup")
	if len(vis.Linked) != 6 {
		t.Errorf("linked = %d entries, want 6 repointed to the next home", len(vis.Linked))
	}
	for _, dir := range harnessSkillDirs(p) {
		if got, err := os.Readlink(filepath.Join(dir, "dup")); err != nil || got != wantTarget {
			t.Errorf("link in %s = %q, %v; want the next tracked home %q", dir, got, err, wantTarget)
		}
	}
}
