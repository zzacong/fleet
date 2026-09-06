package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/pull"
)

// stubPullRunner is the CLI-level git seam: records clones/pulls, answers
// queries from scripted values. No test shells out to real git.
type stubPullRunner struct {
	clones []string
	pulls  []string

	remote    string
	remoteErr error
	status    string
	statusErr error
	before    string
	after     string
	cloneErr  error
	pullErr   error
	revSeq    []string
	revN      int
}

func (s *stubPullRunner) Clone(url, path string) error {
	s.clones = append(s.clones, url+" -> "+path)
	if s.cloneErr != nil {
		return s.cloneErr
	}
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		return err
	}
	return nil
}

func (s *stubPullRunner) RemoteURL(path string) (string, error) {
	if s.remoteErr != nil {
		return "", s.remoteErr
	}
	return s.remote, nil
}

func (s *stubPullRunner) Status(path string) (string, error) {
	if s.statusErr != nil {
		return "", s.statusErr
	}
	return s.status, nil
}

func (s *stubPullRunner) PullFFOnly(path string) error {
	s.pulls = append(s.pulls, path)
	return s.pullErr
}

func (s *stubPullRunner) RevParse(path string) (string, error) {
	if len(s.revSeq) > 0 {
		if s.revN >= len(s.revSeq) {
			return s.revSeq[len(s.revSeq)-1], nil
		}
		v := s.revSeq[s.revN]
		s.revN++
		return v, nil
	}
	if s.revN == 0 {
		s.revN++
		return s.before, nil
	}
	return s.after, nil
}

func swapPullRunner(t *testing.T, r pull.Runner) *stubPullRunner {
	t.Helper()
	prev := newPullRunner
	newPullRunner = func() pull.Runner { return r }
	t.Cleanup(func() { newPullRunner = prev })
	if s, ok := r.(*stubPullRunner); ok {
		return s
	}
	return nil
}

func pullHome(t *testing.T) *paths.Paths {
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

func runPull(t *testing.T, p *paths.Paths, args ...string) (string, string, error) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"skill", "pull"}, args...))
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	t.Setenv("FLEET_REPO", "")
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func TestPullFreshNoPathClonesIntoFleetHomeSlotWithNoConfigWrite(t *testing.T) {
	p := pullHome(t)
	url := "https://example.com/team-customs.git"
	stub := swapPullRunner(t, &stubPullRunner{})

	out, errOut, err := runPull(t, p, url)
	if err != nil {
		t.Fatalf("pull: %v\nout=%s\nerr=%s", err, out, errOut)
	}
	wantRepo := filepath.Join(p.FleetReposDir(), "team-customs")
	if _, err := os.Stat(filepath.Join(wantRepo, ".git")); err != nil {
		t.Errorf("checkout not cloned: %v", err)
	}
	if _, err := os.Stat(p.FleetConfigFile()); !os.IsNotExist(err) {
		t.Errorf("inside-home clone wrote config, want none")
	}
	if _, err := os.Stat(filepath.Join(wantRepo, "skills")); err != nil {
		t.Errorf("collection not ensured: %v", err)
	}
	if !strings.Contains(errOut, "warning") || !strings.Contains(errOut, "skills") {
		t.Errorf("missing empty-collection warning:\n%s", errOut)
	}
	if !strings.Contains(out, "cloned") || !strings.Contains(out, wantRepo) {
		t.Errorf("output missing cloned report:\n%s", out)
	}
	if len(stub.clones) != 1 {
		t.Errorf("clones = %v, want one", stub.clones)
	}
	// Wired for immediate discovery.
	if body, _ := os.ReadFile(p.OpenCodeConfig()); !strings.Contains(string(body), filepath.Join(wantRepo, "skills")) {
		t.Errorf("opencode not wired:\n%s", body)
	}
}

func TestPullExplicitInsideHomeWritesNoConfig(t *testing.T) {
	p := pullHome(t)
	inside := filepath.Join(p.FleetReposDir(), "explicit-team")
	swapPullRunner(t, &stubPullRunner{})

	if _, _, err := runPull(t, p, "https://example.com/repo.git", inside); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.FleetConfigFile()); !os.IsNotExist(err) {
		t.Errorf("inside-home explicit clone wrote config")
	}
}

func TestPullExplicitOutsideAppendsOnceWithoutReordering(t *testing.T) {
	p := pullHome(t)
	outsideA := filepath.Join(t.TempDir(), "a-customs")
	outsideB := filepath.Join(t.TempDir(), "b-customs")
	swapPullRunner(t, &stubPullRunner{})

	if _, _, err := runPull(t, p, "https://example.com/a.git", outsideA); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runPull(t, p, "https://example.com/b.git", outsideB); err != nil {
		t.Fatal(err)
	}
	// Re-pulling the existing outside checkout never reorders.
	stub := &stubPullRunner{remote: "https://example.com/a.git", before: "x", after: "x"}
	swapPullRunner(t, stub)
	if _, _, err := runPull(t, p, "https://example.com/a.git", outsideA); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	got := f.SkillsRepos()
	if len(got) != 2 || got[0] != filepath.Clean(outsideA) || got[1] != filepath.Clean(outsideB) {
		t.Errorf("SkillsRepos() = %q, want order-preserved [a b]", got)
	}
}

func TestPullExistingSameRemoteReportsUpdatedThenCurrent(t *testing.T) {
	p := pullHome(t)
	url := "https://example.com/team.git"
	dest := filepath.Join(p.FleetReposDir(), "team")
	swapPullRunner(t, &stubPullRunner{})
	if _, _, err := runPull(t, p, url); err != nil {
		t.Fatal(err)
	}

	swapPullRunner(t, &stubPullRunner{remote: url, before: "aaa", after: "bbb"})
	out, _, err := runPull(t, p, url, dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "updated") {
		t.Errorf("want updated report:\n%s", out)
	}

	swapPullRunner(t, &stubPullRunner{remote: url, before: "same", after: "same"})
	out, _, err = runPull(t, p, url, dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "current") {
		t.Errorf("want current report:\n%s", out)
	}
}

func TestPullDifferentRemoteFailsUnlessForced(t *testing.T) {
	p := pullHome(t)
	url := "https://example.com/team.git"
	swapPullRunner(t, &stubPullRunner{})
	if _, _, err := runPull(t, p, url); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(p.FleetReposDir(), "team")

	swapPullRunner(t, &stubPullRunner{remote: "https://example.com/other.git", before: "a", after: "a"})
	_, _, err := runPull(t, p, url, dest)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("different remote error = %v, want --force hint", err)
	}

	swapPullRunner(t, &stubPullRunner{remote: "https://example.com/other.git", before: "a", after: "b"})
	if _, _, err := runPull(t, p, "--force", url, dest); err != nil {
		t.Errorf("forced pull should succeed, got %v", err)
	}
}

func TestPullDirtyAndDivergedFailSurfacingState(t *testing.T) {
	p := pullHome(t)
	url := "https://example.com/team.git"
	swapPullRunner(t, &stubPullRunner{})
	if _, _, err := runPull(t, p, url); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(p.FleetReposDir(), "team")

	swapPullRunner(t, &stubPullRunner{remote: url, status: " M skills/foo/SKILL.md"})
	_, _, err := runPull(t, p, url, dest)
	if err == nil || !strings.Contains(err.Error(), "M skills/foo/SKILL.md") {
		t.Errorf("dirty error = %v, want working-tree state", err)
	}

	swapPullRunner(t, &stubPullRunner{remote: url, before: "a", after: "a", pullErr: errors.New("Not possible to fast-forward, aborting")})
	_, _, err = runPull(t, p, url, dest)
	if err == nil || !strings.Contains(err.Error(), "fast-forward") {
		t.Errorf("diverged error = %v, want fast-forward state", err)
	}
}

func TestPullMissingGitFailsCleanly(t *testing.T) {
	p := pullHome(t)
	swapPullRunner(t, &stubPullRunner{cloneErr: pull.ErrGitMissing})
	_, _, err := runPull(t, p, "https://example.com/repo.git")
	if err == nil || !errors.Is(err, pull.ErrGitMissing) {
		t.Errorf("missing git error = %v, want ErrGitMissing", err)
	}
}

func TestBarePullSkipsNonGitWithWarningAndReportsPerRepo(t *testing.T) {
	p := pullHome(t)
	t.Setenv("FLEET_REPO", "")
	plain := filepath.Join(t.TempDir(), "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	f, _ := config.Load(p.FleetConfigFile())
	f.SetSkillsRepos([]string{plain})
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(p.FleetReposDir(), "good")
	if err := os.MkdirAll(filepath.Join(good, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	swapPullRunner(t, &stubPullRunner{before: "a", after: "b"})

	out, errOut, err := runPull(t, p)
	if err != nil {
		t.Fatalf("bare pull: %v\nout=%s\nerr=%s", err, out, errOut)
	}
	if !strings.Contains(out, "skipped") || !strings.Contains(out, plain) {
		t.Errorf("output missing skipped row:\n%s", out)
	}
	if !strings.Contains(out, "updated") || !strings.Contains(out, good) {
		t.Errorf("output missing updated row:\n%s", out)
	}
	if !strings.Contains(errOut, "warning") {
		t.Errorf("stderr missing non-git warning:\n%s", errOut)
	}
}

func TestBarePullRunsSyncAfterWiring(t *testing.T) {
	p := pullHome(t)
	swapPullRunner(t, &stubPullRunner{})
	url := "https://example.com/team.git"
	if _, _, err := runPull(t, p, url); err != nil {
		t.Fatal(err)
	}
	// The pulled collection holds a skill; bare pull must link it and sync.
	skillDir := filepath.Join(p.FleetReposDir(), "team", "skills", "my-notes")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-notes\ndescription: Notes.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	swapPullRunner(t, &stubPullRunner{before: "a", after: "a"})
	out, _, err := runPull(t, p)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(filepath.Join(p.CodexSkills(), "my-notes")); err != nil || got != skillDir {
		t.Errorf("codex link = %q, %v; want %q", got, err, skillDir)
	}
	if !strings.Contains(out, "current") {
		t.Errorf("bare output missing current row:\n%s", out)
	}
}

func TestPullHelpMentionsForceAndBareUpdateAll(t *testing.T) {
	p := pullHome(t)
	out, err := runBare(t, p, "skill", "pull", "--help")
	if err != nil {
		t.Fatalf("pull --help: %v", err)
	}
	for _, want := range []string{"--force", "git-url", "fleet skill pull"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q:\n%s", want, out)
		}
	}
}

func TestBarePullReportsFailedAndContinues(t *testing.T) {
	p := pullHome(t)
	t.Setenv("FLEET_REPO", "")
	bad := filepath.Join(p.FleetReposDir(), "a-bad")
	good := filepath.Join(p.FleetReposDir(), "b-good")
	for _, dir := range []string{bad, good} {
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	swapPullRunner(t, &bareFailStub{})

	out, _, err := runPull(t, p)
	if err == nil {
		t.Fatalf("bare pull with a failure should exit non-zero:\n%s", out)
	}
	if !strings.Contains(out, "failed") || !strings.Contains(out, bad) {
		t.Errorf("output missing failed row:\n%s", out)
	}
	if !strings.Contains(out, "updated") || !strings.Contains(out, good) {
		t.Errorf("output missing updated row for the good repo:\n%s", out)
	}
}

// bareFailStub fails the alphabetically-first repo (dirty) and updates the
// second, proving bare pull continues past per-repo failures.
type bareFailStub struct {
	statusN int
	revN    int
	revs    []string
}

func (s *bareFailStub) Clone(url, path string) error {
	return errors.New("bare pull never clones")
}

func (s *bareFailStub) RemoteURL(path string) (string, error) { return "", nil }

func (s *bareFailStub) Status(path string) (string, error) {
	s.statusN++
	if strings.HasSuffix(path, "a-bad") {
		return " M dirty", nil
	}
	return "", nil
}

func (s *bareFailStub) PullFFOnly(path string) error { return nil }

func (s *bareFailStub) RevParse(path string) (string, error) {
	s.revN++
	if s.revN == 1 {
		return "a", nil
	}
	return "b", nil
}
