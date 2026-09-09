package customs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakeVisibleWiresAndLinksEverySkill(t *testing.T) {
	p, collection := adoptHome(t, nil, []string{"my-notes", "other"})
	// AdoptTo's old test helper writes into the tracked repo; reuse the
	// collection directly — MakeVisible is collection-level.
	_ = collection

	vis, err := MakeVisible(p, collection)
	if err != nil {
		t.Fatal(err)
	}
	if len(vis.Wired) != 2 {
		t.Errorf("wired = %v, want opencode and pi", vis.Wired)
	}
	// 2 skills x 4 link harnesses.
	if len(vis.Linked) != 8 {
		t.Errorf("linked = %d entries, want 8 (2 skills x 4 harnesses)", len(vis.Linked))
	}
	if body, _ := os.ReadFile(p.OpenCodeConfig()); !strings.Contains(string(body), collection) {
		t.Errorf("opencode not wired:\n%s", body)
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
	if len(vis.Wired) != 0 || len(vis.Linked) != 0 {
		t.Errorf("second run reported wired=%v linked=%v, want none", vis.Wired, vis.Linked)
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
	if len(wd.Unwired) != 2 {
		t.Errorf("unwired = %v, want opencode and pi", wd.Unwired)
	}
	if len(wd.Unlinked) != 4 {
		t.Errorf("unlinked = %v, want 4 (1 skill x 4 harnesses)", wd.Unlinked)
	}
	if _, err := os.Lstat(filepath.Join(p.CodexSkills(), "my-notes")); !os.IsNotExist(err) {
		t.Errorf("codex link should be gone, err = %v", err)
	}
}
