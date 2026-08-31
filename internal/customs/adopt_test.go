package customs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zacong/fleet/internal/paths"
)

// adoptHome builds a fake home with every harness installed and a fake
// repo, plus the given skills in the canonical store and (already) in the
// repo.
func adoptHome(t *testing.T, storeSkills, repoSkills []string) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	repo := filepath.Join(t.TempDir(), "repo")
	p := paths.WithRepo(home, repo)

	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range storeSkills {
		writeSkill(t, p.SkillsStore(), name)
	}
	for _, name := range repoSkills {
		writeSkill(t, p.RepoSkills(), name)
	}
	return p
}

func writeSkill(t *testing.T, store, name string) {
	t.Helper()
	path := filepath.Join(store, name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: Does " + name + " things.\n---\n\n# " + name + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAdoptMovesTheSkillIntoTheRepoAndWiresEveryHarness(t *testing.T) {
	p := adoptHome(t, []string{"my-notes"}, nil)
	storeDir := filepath.Join(p.SkillsStore(), "my-notes")
	repoDir := filepath.Join(p.RepoSkills(), "my-notes")

	rep, err := Adopt(p, "my-notes")
	if err != nil {
		t.Fatal(err)
	}

	// The move: source gone, repo copy parses as the same skill.
	if _, err := os.Stat(storeDir); !os.IsNotExist(err) {
		t.Errorf("the skill is still in the canonical store: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(repoDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); !strings.Contains(got, "name: my-notes") {
		t.Errorf("the moved skill lost its frontmatter:\n%s", got)
	}
	if !rep.Moved || rep.From != storeDir || rep.To != repoDir {
		t.Errorf("report move = %+v, want %s → %s", rep, storeDir, repoDir)
	}

	// The repo path is wired into the two config-path harnesses.
	if len(rep.Wired) != 2 {
		t.Fatalf("wired = %v, want opencode and pi", rep.Wired)
	}
	for _, w := range rep.Wired {
		if !w.Changed || w.Where == "" {
			t.Errorf("wiring result not actionable: %+v", w)
		}
	}

	// Every link-based harness reaches the skill at the repo.
	if len(rep.Linked) != 4 {
		t.Fatalf("linked = %v, want one per codex/claude/cursor/bob", rep.Linked)
	}
	for _, dir := range []string{p.CodexSkills(), p.ClaudeSkills(), p.CursorSkills(), p.BobSkills()} {
		if got, err := os.Readlink(filepath.Join(dir, "my-notes")); err != nil || got != repoDir {
			t.Errorf("link in %s = %q, %v; want %q", dir, got, err, repoDir)
		}
	}
}

func TestAdoptKeepsTheCanonicalStoreFreeOfLinks(t *testing.T) {
	p := adoptHome(t, []string{"my-notes"}, nil)

	if _, err := Adopt(p, "my-notes"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(p.SkillsStore())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the canonical store kept entries %v after adoption", entries)
	}
}

func TestAdoptIsIdempotentWhenTheSkillIsAlreadyInTheRepo(t *testing.T) {
	p := adoptHome(t, nil, []string{"my-notes"})
	repoDir := filepath.Join(p.RepoSkills(), "my-notes")

	rep, err := Adopt(p, "my-notes")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Moved {
		t.Error("nothing should move for an already-adopted skill")
	}

	// The wiring and links are still ensured: a partial earlier run may
	// have left them undone.
	if len(rep.Wired) != 2 || len(rep.Linked) != 4 {
		t.Errorf("wired = %v, linked = %v", rep.Wired, rep.Linked)
	}
	if got, err := os.Readlink(filepath.Join(p.BobSkills(), "my-notes")); err != nil || got != repoDir {
		t.Errorf("bob link = %q, %v; want %q", got, err, repoDir)
	}

	// A third run has nothing left to say.
	rep, err = Adopt(p, "my-notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Wired) != 0 || len(rep.Linked) != 0 {
		t.Errorf("third run reported wired=%v linked=%v, want none", rep.Wired, rep.Linked)
	}
}

func TestAdoptRefusesAmbiguityAndUnknownSkills(t *testing.T) {
	t.Run("unknown skill", func(t *testing.T) {
		p := adoptHome(t, []string{"other"}, nil)
		if _, err := Adopt(p, "my-notes"); err == nil {
			t.Error("adopting an unknown skill should fail")
		}
	})

	t.Run("skill in both places", func(t *testing.T) {
		p := adoptHome(t, []string{"my-notes"}, []string{"my-notes"})
		_, err := Adopt(p, "my-notes")
		if err == nil || !strings.Contains(err.Error(), "both") {
			t.Errorf("error = %v, want a both-places refusal", err)
		}
	})

	t.Run("no repo", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "home")
		p := paths.New(home)
		writeSkill(t, p.SkillsStore(), "my-notes")
		_, err := Adopt(p, "my-notes")
		if err == nil || !strings.Contains(err.Error(), "repo") {
			t.Errorf("error = %v, want a no-repo failure", err)
		}
	})
}

func TestAdoptMatchesTheFrontmatterNameToo(t *testing.T) {
	// The dir is "notes" but the frontmatter says "my-notes"; adopt by
	// either, keeping the dir name as-is.
	p := adoptHome(t, nil, nil)
	skillDir := filepath.Join(p.SkillsStore(), "notes")
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nname: my-notes\ndescription: Notes.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := Adopt(p, "my-notes")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Moved || rep.Skill != "notes" {
		t.Errorf("report = %+v, want the notes dir moved", rep)
	}
	if _, err := os.Stat(filepath.Join(p.RepoSkills(), "notes", "SKILL.md")); err != nil {
		t.Errorf("the moved dir should keep its name: %v", err)
	}
}
