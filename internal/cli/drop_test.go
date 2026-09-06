package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/pull"
)

// dropExplicitHome builds a fake home with every harness installed, one
// explicit tracked repo holding a "my-notes" custom skill, the collection
// wired into opencode/pi and linked for the link-based harnesses, plus a
// redundant canonical-store link for sync to clean. It returns the paths,
// the repo root, and the collection dir.
func dropExplicitHome(t *testing.T) (*paths.Paths, string, string) {
	t.Helper()
	p := pullHome(t)
	t.Setenv("FLEET_REPO", "")
	repo := filepath.Join(t.TempDir(), "team-customs")
	collection := filepath.Join(repo, "skills")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkillDir(t, collection, "my-notes", "Notes.")
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetSkillsRepos([]string{repo})
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.WireSkillSource(p, collection); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.LinkCustomSkill(p, "my-notes", filepath.Join(collection, "my-notes")); err != nil {
		t.Fatal(err)
	}
	// Redundant link spam for sync to clean while it is here.
	writeSkillDir(t, p.SkillsStore(), "tdd", "TDD.")
	if err := os.MkdirAll(p.OpenCodeSkills(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(p.SkillsStore(), "tdd"), filepath.Join(p.OpenCodeSkills(), "tdd")); err != nil {
		t.Fatal(err)
	}
	return p, repo, collection
}

func runDrop(t *testing.T, p *paths.Paths, args ...string) (string, string, error) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"skill", "drop"}, args...))
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	t.Setenv("FLEET_REPO", "")
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func TestDropExplicitUnlistsKeepsDiskUnwiresUnlinksAndSyncs(t *testing.T) {
	p, repo, collection := dropExplicitHome(t)
	swapPullRunner(t, &stubPullRunner{})

	out, _, err := runDrop(t, p, repo)
	if err != nil {
		t.Fatalf("drop: %v\nout=%s", err, out)
	}
	// The headline plus the unwired/unlinked lines in palette style.
	for _, want := range []string{
		`drop: removed "` + repo + `"`,
		`opencode: unwired "` + collection + `"`,
		`pi: unwired "` + collection + `"`,
		`codex: unlinked "my-notes"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Unlisted from config, disk untouched.
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.SkillsRepos()) != 0 {
		t.Errorf("SkillsRepos() = %q, want empty", f.SkillsRepos())
	}
	if _, err := os.Stat(filepath.Join(collection, "my-notes", "SKILL.md")); err != nil {
		t.Errorf("explicit drop touched the disk: %v", err)
	}
	// Unwired and unlinked.
	if body := readFile(t, p.OpenCodeConfig()); strings.Contains(body, collection) {
		t.Errorf("opencode still wired:\n%s", body)
	}
	if body := readFile(t, p.PiSettings()); strings.Contains(body, collection) {
		t.Errorf("pi still wired:\n%s", body)
	}
	if _, err := os.Lstat(filepath.Join(p.CodexSkills(), "my-notes")); !os.IsNotExist(err) {
		t.Errorf("codex link still present: %v", err)
	}
	// Sync ran too: the redundant link is gone and reported.
	if _, err := os.Lstat(filepath.Join(p.OpenCodeSkills(), "tdd")); !os.IsNotExist(err) {
		t.Errorf("sync did not remove the redundant link: %v", err)
	}
	if !strings.Contains(out, `sync: opencode: removed redundant link "tdd"`) {
		t.Errorf("output missing the sync report:\n%s", out)
	}
}

func TestDropFleetHomeSlotByNameDeletesDisk(t *testing.T) {
	p := pullHome(t)
	t.Setenv("FLEET_REPO", "")
	repo := filepath.Join(p.FleetReposDir(), "team")
	collection := filepath.Join(repo, "skills")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkillDir(t, collection, "my-notes", "Notes.")
	swapPullRunner(t, &stubPullRunner{})

	out, _, err := runDrop(t, p, "team")
	if err != nil {
		t.Fatalf("drop team: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, `drop: removed "`+repo+`"`) {
		t.Errorf("output missing removed headline:\n%s", out)
	}
	if _, err := os.Stat(repo); !os.IsNotExist(err) {
		t.Errorf("fleet-home checkout not deleted: %v", err)
	}
	// Convention-tracked: dropping writes no config.
	if _, err := os.Stat(p.FleetConfigFile()); !os.IsNotExist(err) {
		t.Errorf("dropping a fleet-home checkout wrote config")
	}
}

func TestDropUnknownTargetFailsCleanly(t *testing.T) {
	p, repo, _ := dropExplicitHome(t)
	swapPullRunner(t, &stubPullRunner{})

	_, _, err := runDrop(t, p, filepath.Join(t.TempDir(), "nope"))
	if err == nil || !strings.Contains(err.Error(), "unknown target") {
		t.Fatalf("unknown target error = %v, want unknown target", err)
	}
	// The tracked repo survives the failed drop.
	f, ferr := config.Load(p.FleetConfigFile())
	if ferr != nil {
		t.Fatal(ferr)
	}
	if len(f.SkillsRepos()) != 1 || f.SkillsRepos()[0] != repo {
		t.Errorf("failed drop changed the tracked list: %q", f.SkillsRepos())
	}
}

func TestDropDirtyFailsUnlessForced(t *testing.T) {
	p, repo, _ := dropExplicitHome(t)
	swapPullRunner(t, &stubPullRunner{status: " M skills/my-notes/SKILL.md"})

	_, _, err := runDrop(t, p, repo)
	if err == nil || !strings.Contains(err.Error(), "M skills/my-notes/SKILL.md") {
		t.Fatalf("dirty error = %v, want working-tree state", err)
	}

	swapPullRunner(t, &stubPullRunner{status: " M skills/my-notes/SKILL.md"})
	out, _, err := runDrop(t, p, "--force", repo)
	if err != nil {
		t.Fatalf("forced drop should succeed, got %v", err)
	}
	if !strings.Contains(out, "removed") {
		t.Errorf("forced drop missing removed headline:\n%s", out)
	}
}

func TestDropAdoptTargetConflictAlwaysFails(t *testing.T) {
	p, repo, collection := dropExplicitHome(t)
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetAdoptTarget(collection)
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}
	swapPullRunner(t, &stubPullRunner{})

	_, _, err = runDrop(t, p, repo)
	if err == nil || !strings.Contains(err.Error(), "adopt target") {
		t.Fatalf("adopt-target error = %v, want re-point hint", err)
	}

	swapPullRunner(t, &stubPullRunner{})
	_, _, err = runDrop(t, p, "--force", repo)
	if err == nil || !strings.Contains(err.Error(), "adopt target") {
		t.Fatalf("forced adopt-target error = %v, want failure even with --force", err)
	}
}

func TestDropMissingGitWarnsAndProceeds(t *testing.T) {
	p, repo, _ := dropExplicitHome(t)
	swapPullRunner(t, &stubPullRunner{statusErr: pull.ErrGitMissing})

	out, errOut, err := runDrop(t, p, repo)
	if err != nil {
		t.Fatalf("missing-git drop: %v\nout=%s\nerr=%s", err, out, errOut)
	}
	if !strings.Contains(errOut, "warning") || !strings.Contains(strings.ToLower(errOut), "git") {
		t.Errorf("stderr missing the missing-git warning:\n%s", errOut)
	}
	if !strings.Contains(out, "removed") {
		t.Errorf("output missing removed headline:\n%s", out)
	}
}

func TestDropHelpMentionsForceAndExamples(t *testing.T) {
	p := pullHome(t)
	out, err := runBare(t, p, "skill", "drop", "--help")
	if err != nil {
		t.Fatalf("drop --help: %v", err)
	}
	for _, want := range []string{"--force", "path-or-name", "fleet skill drop"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q:\n%s", want, out)
		}
	}
}

func TestDropTildePathExpandsAgainstHome(t *testing.T) {
	p := pullHome(t)
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	repo := filepath.Join(fakeHome, "team-customs")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetSkillsRepos([]string{repo})
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}
	swapPullRunner(t, &stubPullRunner{})

	out, _, err := runDrop(t, p, "~/team-customs")
	if err != nil {
		t.Fatalf("tilde drop: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, `drop: removed "`+repo+`"`) {
		t.Errorf("output missing removed headline:\n%s", out)
	}
}
