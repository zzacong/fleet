package customs

import (
	"os"
	"path/filepath"
	"testing"
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
