package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/state"
)

// adoptHome builds a fake home with every harness installed and a fake
// repo root, with the given skills in the canonical store.
func adoptHome(t *testing.T, storeSkills ...string) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	repo := filepath.Join(t.TempDir(), "repo")
	p := paths.WithRepo(home, repo)
	for _, name := range storeSkills {
		writeSkillDir(t, p.SkillsStore(), name, "Does "+name+" things.")
	}
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func runAdopt(t *testing.T, p *paths.Paths, args ...string) (string, error) {
	t.Helper()
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"skill", "adopt"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestAdoptMovesWiresAndLinksEveryHarness(t *testing.T) {
	// The ticket's integration scenario: fake home + fake repo root; adopt;
	// verify every harness's config/link state; verify the moved skill
	// still parses as a skill.
	p := adoptHome(t, "my-notes", "tdd")
	repoSkills := p.RepoSkills()

	out, err := runAdopt(t, p, "my-notes")
	if err != nil {
		t.Fatalf("fleet skill adopt my-notes: %v", err)
	}

	// The move: out of the canonical store, into the repo, byte-identical
	// SKILL.md.
	if _, err := os.Stat(filepath.Join(p.SkillsStore(), "my-notes")); !os.IsNotExist(err) {
		t.Errorf("the skill is still in the canonical store: %v", err)
	}
	moved, err := os.ReadFile(filepath.Join(repoSkills, "my-notes", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(moved), "name: my-notes") {
		t.Errorf("the moved skill is no longer parseable as itself:\n%s", moved)
	}
	if _, err := os.Stat(filepath.Join(p.SkillsStore(), "tdd")); err != nil {
		t.Errorf("a skill that was not adopted must not move: %v", err)
	}

	// opencode: the repo path wired as a V1 skill source (the fresh config
	// has no dialect markers, so V1 is the safe default).
	oc := readFile(t, p.OpenCodeConfig())
	if !strings.Contains(oc, `"skills"`) || !strings.Contains(oc, `"`+repoSkills+`"`) {
		t.Errorf("opencode config missing the wired repo path:\n%s", oc)
	}
	// pi: a plain path entry in the skills array.
	pi := readFile(t, p.PiSettings())
	if !strings.Contains(pi, `"`+repoSkills+`"`) {
		t.Errorf("pi settings missing the wired repo path:\n%s", pi)
	}
	// Managed links in every link-based harness, pointing at the repo.
	for name, dir := range map[string]string{
		"codex":  p.CodexSkills(),
		"claude": p.ClaudeSkills(),
		"cursor": p.CursorSkills(),
		"bob":    p.BobSkills(),
	} {
		got, err := os.Readlink(filepath.Join(dir, "my-notes"))
		if err != nil || got != filepath.Join(repoSkills, "my-notes") {
			t.Errorf("%s link = %q, %v; want %q", name, got, err, filepath.Join(repoSkills, "my-notes"))
		}
	}
	// No double-visibility: the canonical store is empty and stays empty.
	if entries, err := os.ReadDir(p.SkillsStore()); err != nil || len(entries) != 1 || entries[0].Name() != "tdd" {
		t.Errorf("canonical store = %v, %v; want just tdd", entries, err)
	}

	// The command reports what it did.
	if !strings.Contains(out, `adopted "my-notes"`) {
		t.Errorf("output missing adopted headline:\n%s", out)
	}
	for _, want := range []string{
		`opencode: wired "` + repoSkills,
		`pi: wired "` + repoSkills,
		`codex: linked "my-notes"`,
		`bob: linked "my-notes"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "moved "+filepath.Join(p.SkillsStore(), "my-notes")) {
		t.Errorf("output should not contain moved line:\n%s", out)
	}

	// The adopted skill still parses as a skill, marked custom with no
	// source, and every harness now sees it.
	jsonOut := runLs(t, p, "--json")
	var report struct {
		Skills []struct {
			Name     string            `json:"name"`
			Custom   bool              `json:"custom"`
			Source   string            `json:"source"`
			Descr    string            `json:"description"`
			States   map[string]string `json:"states"`
			Outdated *bool             `json:"outdated"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &report); err != nil {
		t.Fatalf("ls --json broke after adoption: %v\n%s", err, jsonOut)
	}
	var found bool
	for _, s := range report.Skills {
		if s.Name != "my-notes" {
			continue
		}
		found = true
		if !s.Custom || s.Source != "" {
			t.Errorf("my-notes = custom %v, source %q; want custom with no source", s.Custom, s.Source)
		}
		if s.Descr != "Does my-notes things." {
			t.Errorf("my-notes description = %q; the moved skill lost its frontmatter", s.Descr)
		}
		for _, h := range []string{"opencode", "pi", "codex", "claude", "cursor", "bob"} {
			if s.States[h] != "on" {
				t.Errorf("my-notes/%s = %q, want on", h, s.States[h])
			}
		}
		if s.Outdated != nil {
			t.Error("custom skills are never checked; outdated must be null")
		}
	}
	if !found {
		t.Errorf("my-notes missing from ls after adoption:\n%s", jsonOut)
	}
}

func TestAdoptIsIdempotent(t *testing.T) {
	p := adoptHome(t, "my-notes")

	if _, err := runAdopt(t, p, "my-notes"); err != nil {
		t.Fatal(err)
	}
	before := map[string]string{
		"opencode": readFile(t, p.OpenCodeConfig()),
		"pi":       readFile(t, p.PiSettings()),
	}

	out, err := runAdopt(t, p, "my-notes")
	if err != nil {
		t.Fatalf("re-adopting an adopted skill must succeed: %v", err)
	}
	if !strings.Contains(out, "already adopted") {
		t.Errorf("output should say the skill is already adopted:\n%s", out)
	}
	if strings.Contains(out, "wired") || strings.Contains(out, "linked") {
		t.Errorf("a no-op adopt reported work it did not do:\n%s", out)
	}
	for name, want := range before {
		if got := readFile(t, map[string]string{
			"opencode": p.OpenCodeConfig(),
			"pi":       p.PiSettings(),
		}[name]); got != want {
			t.Errorf("%s config changed on a no-op adopt:\n%s\nwas\n%s", name, got, want)
		}
	}
}

func TestAdoptRepointsTheSkillsCLIsLinkAtTheRepo(t *testing.T) {
	// Adopting an installed (forked) skill: the skills CLI's per-agent link
	// points at the canonical store, which the move empties. Adopt must
	// take the link over.
	p := adoptHome(t, "my-notes")
	if err := os.MkdirAll(p.CodexSkills(), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(p.SkillsStore(), "my-notes")
	if err := os.Symlink(stale, filepath.Join(p.CodexSkills(), "my-notes")); err != nil {
		t.Fatal(err)
	}

	out, err := runAdopt(t, p, "my-notes")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(filepath.Join(p.CodexSkills(), "my-notes")); got != filepath.Join(p.RepoSkills(), "my-notes") {
		t.Errorf("codex link = %q, want the repo", got)
	}
	if !strings.Contains(out, `codex: repointed "my-notes" (was `+stale+`)`) {
		t.Errorf("output should report the repoint:\n%s", out)
	}
}

func TestAdoptLeavesARealDirectoryAloneAndSaysSo(t *testing.T) {
	p := adoptHome(t, "my-notes")
	real := filepath.Join(p.ClaudeSkills(), "my-notes")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := runAdopt(t, p, "my-notes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not a link — left alone") {
		t.Errorf("output should explain the skip:\n%s", out)
	}
	if _, err := os.Lstat(real); err != nil {
		t.Errorf("the user's directory was touched: %v", err)
	}
}

func TestAdoptKeepsExistingDisablesWorking(t *testing.T) {
	// Disables recorded before adoption target the skill's name; they must
	// survive the move untouched.
	p := adoptHome(t, "my-notes")
	if err := runToggleErr(t, p, "off", "my-notes", "--harness", "opencode"); err != nil {
		t.Fatal(err)
	}

	if _, err := runAdopt(t, p, "my-notes"); err != nil {
		t.Fatal(err)
	}

	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDisabled("my-notes", "opencode") {
		t.Error("adoption lost the recorded disable")
	}
	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, `"my-notes": "deny"`) {
		t.Errorf("opencode deny rule did not survive adoption:\n%s", body)
	}
	jsonOut := runLs(t, p, "--json")
	if !strings.Contains(jsonOut, `"opencode": "off"`) {
		t.Errorf("ls should still show my-notes off for opencode:\n%s", jsonOut)
	}
}

func TestAdoptOnlyTouchesInstalledHarnesses(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	repo := filepath.Join(t.TempDir(), "repo")
	p := paths.WithRepo(home, repo)
	writeSkillDir(t, p.SkillsStore(), "my-notes", "desc.")
	if err := os.MkdirAll(p.OpenCodeDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p.CodexDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := runAdopt(t, p, "my-notes"); err != nil {
		t.Fatal(err)
	}

	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, p.RepoSkills()) {
		t.Errorf("opencode was not wired:\n%s", body)
	}
	if got, err := os.Readlink(filepath.Join(p.CodexSkills(), "my-notes")); err != nil {
		t.Errorf("codex link missing: %v", err)
	} else if got != filepath.Join(p.RepoSkills(), "my-notes") {
		t.Errorf("codex link = %q", got)
	}
	for _, dir := range []string{p.PiDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("an uninstalled harness's config dir was created: %s", dir)
		}
	}
}

func TestAdoptByFrontmatterNameKeepsTheDirName(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	repo := filepath.Join(t.TempDir(), "repo")
	p := paths.WithRepo(home, repo)
	skillDir := filepath.Join(p.SkillsStore(), "notes")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: my-notes\ndescription: Notes.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := runAdopt(t, p, "my-notes"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.RepoSkills(), "notes", "SKILL.md")); err != nil {
		t.Errorf("the directory must keep its name: %v", err)
	}
}

func TestAdoptRefusesUnknownSkillsAndAmbiguity(t *testing.T) {
	p := adoptHome(t, "other")
	if _, err := runAdopt(t, p, "my-notes"); err == nil {
		t.Error("adopting an unknown skill should fail")
	}

	// A copy left in both places is ambiguous; adopt refuses instead of
	// guessing.
	writeSkillDir(t, p.RepoSkills(), "other", "desc.")
	if _, err := runAdopt(t, p, "other"); err == nil || !strings.Contains(err.Error(), "both") {
		t.Errorf("adopt with the skill in both places should refuse, got %v", err)
	}
}

func TestAdoptWithoutARepoExplainsHowToFixIt(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	writeSkillDir(t, p.SkillsStore(), "my-notes", "desc.")
	if err := os.MkdirAll(p.OpenCodeDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := runAdopt(t, p, "my-notes")
	if err == nil || !strings.Contains(err.Error(), "FLEET_REPO") {
		t.Errorf("error = %v, want the no-repo hint", err)
	}
}

func TestAdoptIsReversibleByHand(t *testing.T) {
	// Moving the directory back into the store must not break fleet: ls
	// still works and shows the skill, adopt can redo the migration.
	p := adoptHome(t, "my-notes")
	if _, err := runAdopt(t, p, "my-notes"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(p.RepoSkills(), "my-notes"), filepath.Join(p.SkillsStore(), "my-notes")); err != nil {
		t.Fatal(err)
	}

	jsonOut := runLs(t, p, "--json")
	if !strings.Contains(jsonOut, `"name": "my-notes"`) || !strings.Contains(jsonOut, `"custom": true`) {
		t.Errorf("ls broke after a hand-reversal:\n%s", jsonOut)
	}

	if _, err := runAdopt(t, p, "my-notes"); err != nil {
		t.Fatalf("re-adopting after a hand-reversal should just work: %v", err)
	}
}
