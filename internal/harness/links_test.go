package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zacong/fleet/internal/paths"
)

// linksHome builds a home with every harness installed.
func linksHome(t *testing.T) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// linkDirs are the harness skills dirs that take managed custom-skill
// links, keyed by harness name.
func linkDirs(p *paths.Paths) map[string]string {
	return map[string]string{
		"codex":  p.CodexSkills(),
		"claude": p.ClaudeSkills(),
		"cursor": p.CursorSkills(),
		"bob":    p.BobSkills(),
	}
}

func readLink(t *testing.T, path string) string {
	t.Helper()
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("Readlink(%s): %v", path, err)
	}
	return target
}

func TestLinkCustomSkillLinksEveryLinkBasedHarnessAtTheRepo(t *testing.T) {
	p := linksHome(t)
	target := "/repo/skills/my-notes"

	res, err := LinkCustomSkill(p, "my-notes", target)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 4 {
		t.Fatalf("results = %v, want one per codex/claude/cursor/bob", res)
	}
	for _, r := range res {
		if r.Change.Action != LinkCreated {
			t.Errorf("%s action = %q, want created", r.Harness, r.Change.Action)
		}
	}

	for name, dir := range linkDirs(p) {
		if got := readLink(t, filepath.Join(dir, "my-notes")); got != target {
			t.Errorf("%s link target = %q, want %q", name, got, target)
		}
	}

	// opencode and pi discover customs through their config paths; the
	// canonical store must stay untouched (a link there would make
	// opencode/pi see the skill twice).
	if _, err := os.Lstat(filepath.Join(p.SkillsStore(), "my-notes")); !os.IsNotExist(err) {
		t.Error("a link was created inside the canonical store")
	}
	if _, err := os.Stat(p.OpenCodeConfig()); !os.IsNotExist(err) {
		t.Error("opencode config created by a link-only operation")
	}
}

func TestLinkCustomSkillRepointsStaleLinksAtTheRepo(t *testing.T) {
	p := linksHome(t)
	// The skills CLI's auto-link for an installed skill points at the
	// canonical store; after adoption that target is gone, so the managed
	// link must take over.
	if err := os.MkdirAll(p.CodexSkills(), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(p.SkillsStore(), "my-notes")
	if err := os.Symlink(stale, filepath.Join(p.CodexSkills(), "my-notes")); err != nil {
		t.Fatal(err)
	}

	res, err := LinkCustomSkill(p, "my-notes", "/repo/skills/my-notes")
	if err != nil {
		t.Fatal(err)
	}
	var repointed bool
	for _, r := range res {
		if r.Harness == Codex && r.Change.Action == LinkRepointed && r.Change.From == stale {
			repointed = true
		}
	}
	if !repointed {
		t.Errorf("codex link not repointed with its old target reported: %v", res)
	}
	if got := readLink(t, filepath.Join(p.CodexSkills(), "my-notes")); got != "/repo/skills/my-notes" {
		t.Errorf("codex link target = %q, want the repo", got)
	}
}

func TestLinkCustomSkillIsIdempotentAndNeverClobbersRealFiles(t *testing.T) {
	p := linksHome(t)
	target := "/repo/skills/my-notes"

	if _, err := LinkCustomSkill(p, "my-notes", target); err != nil {
		t.Fatal(err)
	}
	// A second run changes nothing: unchanged links are omitted.
	res, err := LinkCustomSkill(p, "my-notes", target)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Errorf("second run reported %v, want nothing", res)
	}

	// A real directory where a link would go is the user's: skipped, not
	// replaced.
	real := filepath.Join(p.ClaudeSkills(), "my-notes")
	if err := os.Remove(real); err != nil { // drop the managed link first
		t.Fatal(err)
	}
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "SKILL.md"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = LinkCustomSkill(p, "my-notes", target)
	if err != nil {
		t.Fatal(err)
	}
	var skipped bool
	for _, r := range res {
		if r.Harness == Claude && r.Change.Action == LinkSkipped && r.Change.Note != "" {
			skipped = true
		}
	}
	if !skipped {
		t.Errorf("claude real dir not skipped with a note: %v", res)
	}
	if body, err := os.ReadFile(filepath.Join(real, "SKILL.md")); err != nil || string(body) != "mine" {
		t.Errorf("the user's directory was touched: %q, %v", body, err)
	}
}

func TestLinkCustomSkillSkipsUninstalledHarnesses(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	if err := os.MkdirAll(p.CodexDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := LinkCustomSkill(p, "my-notes", "/repo/skills/my-notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Harness != Codex {
		t.Errorf("results = %v, want only codex", res)
	}
	if _, err := os.Stat(p.ClaudeSkills()); !os.IsNotExist(err) {
		t.Error("claude skills dir created although claude is not installed")
	}
}

func TestEveryLinkBasedHarnessHasTheRightSkillsDir(t *testing.T) {
	p := linksHome(t)
	want := map[string]string{
		"codex":  p.CodexSkills(),
		"claude": p.ClaudeSkills(),
		"cursor": p.CursorSkills(),
		"bob":    p.BobSkills(),
	}
	for _, a := range All(p) {
		l, ok := a.(SkillLinker)
		if a.Harness() == OpenCode || a.Harness() == Pi {
			if ok {
				t.Errorf("%s must not take managed links; it wires config paths", a.Harness())
			}
			continue
		}
		if !ok {
			t.Fatalf("%s does not implement SkillLinker", a.Harness())
		}
		// The adapter's own dir is where its link lands: point at a
		// missing target and confirm the link appears there.
		dir := want[string(a.Harness())]
		if _, err := l.LinkSkill("probe", "/repo/skills/probe"); err != nil {
			t.Fatalf("%s LinkSkill: %v", a.Harness(), err)
		}
		if got := readLink(t, filepath.Join(dir, "probe")); got != "/repo/skills/probe" {
			t.Errorf("%s link landed at the wrong dir: %q", a.Harness(), got)
		}
	}
}
