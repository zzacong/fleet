package customs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
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

// fleetHome builds a fake home with no repo (fallback to fleet-home).
func fleetHome(t *testing.T, storeSkills, fleetSkills []string) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range storeSkills {
		writeSkill(t, p.SkillsStore(), name)
	}
	for _, name := range fleetSkills {
		writeSkill(t, p.FleetHomeSkills(), name)
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

func TestAdoptMovesTheSkillIntoFleetHomeWhenNoRepoSet(t *testing.T) {
	p := fleetHome(t, []string{"my-notes"}, nil)
	storeDir := filepath.Join(p.SkillsStore(), "my-notes")
	fleetDir := filepath.Join(p.FleetHomeSkills(), "my-notes")

	rep, err := Adopt(p, "my-notes")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(storeDir); !os.IsNotExist(err) {
		t.Errorf("the skill is still in the canonical store: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fleetDir, "SKILL.md")); err != nil {
		t.Fatalf("skill not moved to fleet-home: %v", err)
	}
	if !rep.Moved || rep.From != storeDir || rep.To != fleetDir {
		t.Errorf("report move = %+v, want %s → %s", rep, storeDir, fleetDir)
	}
	// Wiring should contain fleet-home, not repo (which is empty)
	for _, w := range rep.Wired {
		if !w.Changed {
			t.Errorf("wiring not changed: %+v", w)
		}
	}
	// Links must point into fleet-home, not canonical store
	for _, dir := range []string{p.CodexSkills(), p.ClaudeSkills(), p.CursorSkills(), p.BobSkills()} {
		got, err := os.Readlink(filepath.Join(dir, "my-notes"))
		if err != nil || got != fleetDir {
			t.Errorf("link in %s = %q, %v; want %q", dir, got, err, fleetDir)
		}
		if strings.HasPrefix(got, p.SkillsStore()) {
			t.Errorf("link points into canonical store: %q", got)
		}
	}
}

func TestAdoptCreatesMissingFleetHomeDirOnDemand(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeSkill(t, p.SkillsStore(), "fresh")
	// FleetHomeSkills dir does not exist yet
	if _, err := os.Stat(p.FleetHomeSkills()); !os.IsNotExist(err) {
		t.Fatalf("fleet-home should not exist yet: %v", err)
	}
	rep, err := Adopt(p, "fresh")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Moved {
		t.Error("should have moved")
	}
	if _, err := os.Stat(filepath.Join(p.FleetHomeSkills(), "fresh", "SKILL.md")); err != nil {
		t.Errorf("fleet-home skill not created: %v", err)
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

func TestAdoptIsIdempotentWhenAlreadyInFleetHome(t *testing.T) {
	p := fleetHome(t, nil, []string{"my-notes"})
	fleetDir := filepath.Join(p.FleetHomeSkills(), "my-notes")

	rep, err := Adopt(p, "my-notes")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Moved {
		t.Error("nothing should move for an already-adopted skill in fleet-home")
	}
	if rep.To != fleetDir {
		t.Errorf("rep.To = %q, want %q", rep.To, fleetDir)
	}
	if len(rep.Wired) != 2 || len(rep.Linked) != 4 {
		t.Errorf("wired = %v, linked = %v", rep.Wired, rep.Linked)
	}
	if got, err := os.Readlink(filepath.Join(p.BobSkills(), "my-notes")); err != nil || got != fleetDir {
		t.Errorf("bob link = %q, %v; want %q", got, err, fleetDir)
	}
	// Second call heals to no-op
	rep, err = Adopt(p, "my-notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Wired) != 0 || len(rep.Linked) != 0 {
		t.Errorf("second run reported wired=%v linked=%v, want none", rep.Wired, rep.Linked)
	}
}

func TestAdoptReEnsuresWiringAndLinksAfterPartialFailure(t *testing.T) {
	// Simulate partially failed earlier run: skill already moved to repo but wiring/links missing
	t.Run("repo target", func(t *testing.T) {
		p := adoptHome(t, nil, []string{"my-notes"})
		// Remove wiring/links that Adopt would have created — but Adopt with repo target will recreate them.
		// Initially no wiring, so first Adopt should wire.
		rep, err := Adopt(p, "my-notes")
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Wired) == 0 || len(rep.Linked) == 0 {
			t.Fatalf("first adopt should wire/link, got wired=%v linked=%v", rep.Wired, rep.Linked)
		}
		// Now manually remove a link to simulate partial failure
		if err := os.Remove(filepath.Join(p.CodexSkills(), "my-notes")); err != nil {
			t.Fatal(err)
		}
		rep2, err := Adopt(p, "my-notes")
		if err != nil {
			t.Fatal(err)
		}
		if rep2.Moved {
			t.Error("re-adopt should not move")
		}
		if len(rep2.Linked) != 1 {
			t.Errorf("expected one re-created link, got %v", rep2.Linked)
		}
		got, err := os.Readlink(filepath.Join(p.CodexSkills(), "my-notes"))
		if err != nil || got != filepath.Join(p.RepoSkills(), "my-notes") {
			t.Errorf("codex link after heal = %q, %v", got, err)
		}
	})
	t.Run("fleet-home target", func(t *testing.T) {
		p := fleetHome(t, nil, []string{"my-notes"})
		rep, err := Adopt(p, "my-notes")
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Wired) == 0 || len(rep.Linked) == 0 {
			t.Fatalf("first adopt should wire/link")
		}
		_ = os.Remove(filepath.Join(p.ClaudeSkills(), "my-notes"))
		rep2, err := Adopt(p, "my-notes")
		if err != nil {
			t.Fatal(err)
		}
		if rep2.Moved {
			t.Error("should not move")
		}
		if len(rep2.Linked) == 0 {
			t.Error("should re-create missing claude link")
		}
	})
}

func TestAdoptRefusesAmbiguityAndUnknownSkills(t *testing.T) {
	t.Run("unknown skill", func(t *testing.T) {
		p := adoptHome(t, []string{"other"}, nil)
		if _, err := Adopt(p, "my-notes"); err == nil {
			t.Error("adopting an unknown skill should fail")
		}
	})

	t.Run("skill in both canonical and repo", func(t *testing.T) {
		p := adoptHome(t, []string{"my-notes"}, []string{"my-notes"})
		_, err := Adopt(p, "my-notes")
		if err == nil || !strings.Contains(err.Error(), "both") {
			t.Errorf("error = %v, want a both-places refusal", err)
		}
		if !strings.Contains(err.Error(), p.SkillsStore()) || !strings.Contains(err.Error(), p.RepoSkills()) {
			t.Errorf("error should mention both paths, got %v", err)
		}
	})

	t.Run("skill in both canonical and fleet-home", func(t *testing.T) {
		p := fleetHome(t, []string{"my-notes"}, []string{"my-notes"})
		// add repo so that target would be repo but we have canonical+fleet collision
		// Actually fleetHome has no repo; we manually add a repo path to test double presence when Repo set
		repo := filepath.Join(t.TempDir(), "repo")
		p.Repo = repo
		writeSkill(t, p.FleetHomeSkills(), "my-notes") // already there via fleetHome, but ensure
		writeSkill(t, p.SkillsStore(), "my-notes")
		_, err := Adopt(p, "my-notes")
		if err == nil || !strings.Contains(err.Error(), "both") {
			t.Errorf("error = %v, want both-places", err)
		}
	})

	t.Run("skill in both fleet-home and repo", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "home")
		repo := filepath.Join(t.TempDir(), "repo")
		p := paths.WithRepo(home, repo)
		for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
			_ = os.MkdirAll(dir, 0o755)
		}
		writeSkill(t, p.FleetHomeSkills(), "my-notes")
		writeSkill(t, p.RepoSkills(), "my-notes")
		_, err := Adopt(p, "my-notes")
		if err == nil || !strings.Contains(err.Error(), "both") {
			t.Errorf("error = %v, want both-places for fleet+repo", err)
		}
		if !strings.Contains(err.Error(), p.FleetHomeSkills()) || !strings.Contains(err.Error(), p.RepoSkills()) {
			t.Errorf("error should mention fleet and repo, got %v", err)
		}
	})

	t.Run("no repo now adopts into fleet-home", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "home")
		p := paths.New(home)
		for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
			_ = os.MkdirAll(dir, 0o755)
		}
		writeSkill(t, p.SkillsStore(), "my-notes")
		rep, err := Adopt(p, "my-notes")
		if err != nil {
			t.Fatalf("adopt without repo should succeed into fleet-home, got %v", err)
		}
		if !rep.Moved {
			t.Error("should have moved into fleet-home")
		}
		if rep.To != filepath.Join(p.FleetHomeSkills(), "my-notes") {
			t.Errorf("rep.To = %q, want fleet-home", rep.To)
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

func TestAdoptMatchesFrontmatterNameInFleetHome(t *testing.T) {
	p := fleetHome(t, nil, nil)
	skillDir := filepath.Join(p.SkillsStore(), "notes")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-notes\ndescription: Notes.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Adopt(p, "my-notes")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Moved || rep.Skill != "notes" {
		t.Errorf("report = %+v, want notes dir moved to fleet-home", rep)
	}
	if _, err := os.Stat(filepath.Join(p.FleetHomeSkills(), "notes", "SKILL.md")); err != nil {
		t.Errorf("moved dir should be in fleet-home: %v", err)
	}
}

func TestAdoptDoesNotPointLinksIntoCanonicalStore(t *testing.T) {
	for _, useRepo := range []bool{true, false} {
		t.Run(func() string {
			if useRepo {
				return "repo target"
			}
			return "fleet-home target"
		}(), func(t *testing.T) {
			var p *paths.Paths
			if useRepo {
				p = adoptHome(t, []string{"my-notes"}, nil)
			} else {
				p = fleetHome(t, []string{"my-notes"}, nil)
			}
			rep, err := Adopt(p, "my-notes")
			if err != nil {
				t.Fatal(err)
			}
			for _, l := range rep.Linked {
				if strings.HasPrefix(l.Target, p.SkillsStore()) {
					t.Errorf("link target %q points into canonical store", l.Target)
				}
			}
			// Also check on disk
			target := p.RepoSkills()
			if target == "" {
				target = p.FleetHomeSkills()
			}
			for _, dir := range []string{p.CodexSkills(), p.ClaudeSkills(), p.CursorSkills(), p.BobSkills()} {
				got, err := os.Readlink(filepath.Join(dir, "my-notes"))
				if err != nil {
					t.Fatalf("readlink: %v", err)
				}
				if got != filepath.Join(target, "my-notes") {
					t.Errorf("link points at %q, want %q", got, filepath.Join(target, "my-notes"))
				}
			}
		})
	}
}

func TestAdoptNoStateFileWrite(t *testing.T) {
	p := adoptHome(t, []string{"my-notes"}, nil)
	// Create a state file before adopt
	statePath := p.FleetStateFile()
	_ = os.MkdirAll(filepath.Dir(statePath), 0o755)
	orig := `{"version":1,"disables":{}}`
	if err := os.WriteFile(statePath, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Adopt(p, "my-notes"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != orig {
		t.Errorf("state file changed: got %q want %q", string(body), orig)
	}
}

func TestAdoptCollisionFrontmatterName(t *testing.T) {
	// Canonical has frontmatter my-notes (dir notes), fleet-home has dir my-notes — collision by name
	home := filepath.Join(t.TempDir(), "home")
	repo := filepath.Join(t.TempDir(), "repo")
	p := paths.WithRepo(home, repo)
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		_ = os.MkdirAll(dir, 0o755)
	}
	// canonical: dir "notes" with frontmatter my-notes
	notesDir := filepath.Join(p.SkillsStore(), "notes")
	_ = os.MkdirAll(notesDir, 0o755)
	_ = os.WriteFile(filepath.Join(notesDir, "SKILL.md"), []byte("---\nname: my-notes\n---\n"), 0o644)
	// fleet-home: dir my-notes
	writeSkill(t, p.FleetHomeSkills(), "my-notes")
	_, err := Adopt(p, "my-notes")
	if err == nil || !strings.Contains(err.Error(), "both") {
		t.Errorf("expected double-presence via frontmatter, got %v", err)
	}
}
