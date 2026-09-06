package pull

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

func newTestPaths(home string) *paths.Paths { return paths.New(home) }

func TestDeriveDirNameFromURL(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://github.com/user/repo.git", "repo"},
		{"https://github.com/user/repo", "repo"},
		{"https://github.com/user/repo/", "repo"},
		{"https://github.com/user/repo///", "repo"},
		{"https://github.com/user/repo.git/", "repo"},
		{"git@github.com:user/repo.git", "repo"},
		{"git@github.com:user/repo", "repo"},
		{"git@github.com:repo.git", "repo"},
		{"ssh://git@github.com/user/repo.git", "repo"},
		{"file:///tmp/myrepo", "myrepo"},
		{"file:///tmp/myrepo.git", "myrepo"},
		{"/tmp/local/path/mycustoms", "mycustoms"},
		{"mycustoms", "mycustoms"},
		{"mycustoms.git", "mycustoms"},
	}
	for _, c := range cases {
		if got := DeriveDirName(c.url); got != c.want {
			t.Errorf("DeriveDirName(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestDeriveDirNameEmptyWhenNothingToDerive(t *testing.T) {
	for _, url := range []string{"", "   ", "/", "///"} {
		if got := DeriveDirName(url); got != "" {
			t.Errorf("DeriveDirName(%q) = %q, want empty", url, got)
		}
	}
}

func TestDefaultPathDerivesFleetHomeSlot(t *testing.T) {
	home := t.TempDir()
	p := newTestPaths(home)
	got := DefaultPath(p, "https://github.com/user/my-customs.git")
	want := filepath.Join(p.FleetReposDir(), "my-customs")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestSameRemoteIgnoresTrailingSlashAndDotGit(t *testing.T) {
	if !SameRemote("https://example.com/repo", "https://example.com/repo.git") {
		t.Error("SameRemote should ignore .git suffix")
	}
	if !SameRemote("https://example.com/repo/", "https://example.com/repo") {
		t.Error("SameRemote should ignore trailing slash")
	}
	if SameRemote("https://example.com/a", "https://example.com/b") {
		t.Error("SameRemote(a, b) = true, want false")
	}
}

func TestEnsureTrackedInsideFleetHomeWritesNoConfig(t *testing.T) {
	home := t.TempDir()
	p := newTestPaths(home)
	inside := filepath.Join(p.FleetReposDir(), "customs")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	wrote, err := EnsureTracked(p, inside)
	if err != nil {
		t.Fatalf("EnsureTracked() error = %v", err)
	}
	if wrote {
		t.Error("EnsureTracked inside fleet home should not write")
	}
	if _, err := os.Stat(p.FleetConfigFile()); !os.IsNotExist(err) {
		t.Errorf("config file should not exist, stat err = %v", err)
	}
}

func TestEnsureTrackedOutsideAppendsOnceWithoutReordering(t *testing.T) {
	home := t.TempDir()
	p := newTestPaths(home)
	outsideA := filepath.Join(t.TempDir(), "a")
	outsideB := filepath.Join(t.TempDir(), "b")
	for _, dir := range []string{outsideA, outsideB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := EnsureTracked(p, outsideA); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureTracked(p, outsideB); err != nil {
		t.Fatal(err)
	}
	// Re-ensuring an existing entry never reorders.
	if _, err := EnsureTracked(p, outsideA); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	got := f.SkillsRepos()
	if len(got) != 2 || got[0] != outsideA || got[1] != outsideB {
		t.Errorf("SkillsRepos() = %q, want [%q %q] in order", got, outsideA, outsideB)
	}
}

func TestEnsureCollectionCreatesWithFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, created, err := EnsureCollection(root)
	if err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}
	if dir != filepath.Join(root, "skills") {
		t.Errorf("collection = %q, want skills subdir", dir)
	}
	if !created {
		t.Error("created = false, want true for a missing collection")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("collection dir not created: %v", err)
	}
	// Second call reports no creation.
	_, created, err = EnsureCollection(root)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("created = true on existing collection, want false")
	}
}

// stubRunner is the injected git seam for pull-flow tests: it records
// requested operations and answers from scripted values. No test shells
// out to real git.
type stubRunner struct {
	clones []string
	pulls  []string

	remote string
	status string
	before string
	after  string

	cloneErr  error
	remoteErr error
	statusErr error
	pullErr   error
	beforeErr error
	afterErr  error

	revCalls int
}

func (s *stubRunner) Clone(url, path string) error {
	s.clones = append(s.clones, url+" -> "+path)
	if s.cloneErr != nil {
		return s.cloneErr
	}
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		return err
	}
	return nil
}

func (s *stubRunner) RemoteURL(path string) (string, error) {
	if s.remoteErr != nil {
		return "", s.remoteErr
	}
	return s.remote, nil
}

func (s *stubRunner) Status(path string) (string, error) {
	if s.statusErr != nil {
		return "", s.statusErr
	}
	return s.status, nil
}

func (s *stubRunner) PullFFOnly(path string) error {
	s.pulls = append(s.pulls, path)
	return s.pullErr
}

func (s *stubRunner) RevParse(path string) (string, error) {
	s.revCalls++
	if s.revCalls == 1 {
		if s.beforeErr != nil {
			return "", s.beforeErr
		}
		return s.before, nil
	}
	if s.afterErr != nil {
		return "", s.afterErr
	}
	return s.after, nil
}

func pullTestHome(t *testing.T) *paths.Paths {
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

func TestCloneNewIntoDefaultSlotWritesNoConfigAndWarnsOnEmptyCollection(t *testing.T) {
	p := pullTestHome(t)
	url := "https://example.com/team-customs.git"
	dest := DefaultPath(p, url)
	stub := &stubRunner{}

	res, err := CloneNew(p, stub, url, dest)
	if err != nil {
		t.Fatalf("CloneNew() error = %v", err)
	}
	if res.Outcome != OutcomeCloned {
		t.Errorf("outcome = %q, want cloned", res.Outcome)
	}
	if len(stub.clones) != 1 {
		t.Fatalf("clones = %v, want one clone", stub.clones)
	}
	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		t.Errorf("clone did not create checkout: %v", err)
	}
	// Inside fleet home: no config write.
	if _, err := os.Stat(p.FleetConfigFile()); !os.IsNotExist(err) {
		t.Errorf("config file should not exist after inside-home clone, err = %v", err)
	}
	// Empty remote: collection created, so a warning is reported.
	if !res.CollectionCreated {
		t.Error("CollectionCreated = false, want true for empty clone")
	}
	if _, err := os.Stat(filepath.Join(dest, "skills")); err != nil {
		t.Errorf("collection not ensured: %v", err)
	}
	// Wired into config-path harnesses even when empty.
	if body, _ := os.ReadFile(p.OpenCodeConfig()); !contains(body, filepath.Join(dest, "skills")) {
		t.Errorf("opencode not wired to collection:\n%s", body)
	}
}

func TestCloneNewOutsideAppendsRepoRoot(t *testing.T) {
	p := pullTestHome(t)
	outside := filepath.Join(t.TempDir(), "outside-customs")
	stub := &stubRunner{}

	if _, err := CloneNew(p, stub, "https://example.com/repo.git", outside); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	got := f.SkillsRepos()
	if len(got) != 1 || got[0] != filepath.Clean(outside) {
		t.Errorf("SkillsRepos() = %q, want [%q]", got, outside)
	}
}

func TestUpdateExistingSameRemoteFastForwards(t *testing.T) {
	p := pullTestHome(t)
	repo := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	url := "https://example.com/team.git"
	stub := &stubRunner{remote: url, before: "aaa", after: "bbb"}

	res, err := UpdateExisting(p, stub, url, repo, false)
	if err != nil {
		t.Fatalf("UpdateExisting() error = %v", err)
	}
	if res.Outcome != OutcomeUpdated {
		t.Errorf("outcome = %q, want updated", res.Outcome)
	}
	if len(stub.pulls) != 1 {
		t.Errorf("pulls = %v, want one fast-forward pull", stub.pulls)
	}
	// Re-cloning onto an existing path never touches config.
	if _, err := os.Stat(p.FleetConfigFile()); !os.IsNotExist(err) {
		t.Errorf("update should not write config, stat err = %v", err)
	}
}

func TestUpdateExistingCurrentWhenAlreadyUpToDate(t *testing.T) {
	p := pullTestHome(t)
	repo := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	url := "https://example.com/team.git"
	stub := &stubRunner{remote: url, before: "same", after: "same"}

	res, err := UpdateExisting(p, stub, url, repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != OutcomeCurrent {
		t.Errorf("outcome = %q, want current", res.Outcome)
	}
}

func TestUpdateExistingDifferentRemoteFailsUnlessForced(t *testing.T) {
	p := pullTestHome(t)
	repo := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stub := &stubRunner{remote: "https://example.com/other.git", before: "a", after: "a"}

	if _, err := UpdateExisting(p, stub, "https://example.com/team.git", repo, false); err == nil {
		t.Fatal("different remote without force should fail")
	} else if !containsStr(err.Error(), "--force") {
		t.Errorf("error = %v, want --force hint", err)
	}
	if len(stub.pulls) != 0 {
		t.Errorf("pull ran despite remote mismatch: %v", stub.pulls)
	}

	stub.pulls = nil
	if _, err := UpdateExisting(p, stub, "https://example.com/team.git", repo, true); err != nil {
		t.Errorf("different remote with force should proceed, got %v", err)
	}
	if len(stub.pulls) != 1 {
		t.Errorf("forced pull did not run: %v", stub.pulls)
	}
}

func TestUpdateExistingDirtyFailsSurfacingState(t *testing.T) {
	p := pullTestHome(t)
	repo := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stub := &stubRunner{remote: "https://example.com/team.git", status: " M skills/foo/SKILL.md"}

	_, err := UpdateExisting(p, stub, "https://example.com/team.git", repo, false)
	if err == nil {
		t.Fatal("dirty tree should fail")
	}
	if !containsStr(err.Error(), "M skills/foo/SKILL.md") {
		t.Errorf("error should surface working-tree state, got %v", err)
	}
	if len(stub.pulls) != 0 {
		t.Errorf("pull ran on dirty tree: %v", stub.pulls)
	}
}

func TestUpdateExistingDivergedFails(t *testing.T) {
	p := pullTestHome(t)
	repo := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stub := &stubRunner{
		remote:  "https://example.com/team.git",
		before:  "aaa",
		after:   "aaa",
		pullErr: errors.New("Not possible to fast-forward, aborting"),
	}

	_, err := UpdateExisting(p, stub, "https://example.com/team.git", repo, false)
	if err == nil {
		t.Fatal("diverged pull should fail")
	}
	if !containsStr(err.Error(), "fast-forward") {
		t.Errorf("error should surface diverged state, got %v", err)
	}
}

func TestUpdateExistingMissingGitFailsCleanly(t *testing.T) {
	p := pullTestHome(t)
	repo := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stub := &stubRunner{remoteErr: ErrGitMissing}

	_, err := UpdateExisting(p, stub, "https://example.com/team.git", repo, false)
	if err == nil {
		t.Fatal("missing git should fail")
	}
	if !errors.Is(err, ErrGitMissing) {
		t.Errorf("error = %v, want ErrGitMissing", err)
	}
}

func contains(b []byte, s string) bool { return containsStr(string(b), s) }

func containsStr(hay, needle string) bool { return strings.Contains(hay, needle) }

func TestWireHomeLinksCollectionSkills(t *testing.T) {
	p := pullTestHome(t)
	repo := filepath.Join(p.FleetReposDir(), "team")
	collection := filepath.Join(repo, "skills", "my-notes")
	if err := os.MkdirAll(collection, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(collection, "SKILL.md"), []byte("---\nname: my-notes\ndescription: Notes.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stub := &stubRunner{remote: "https://example.com/team.git", before: "a", after: "a"}

	if _, err := UpdateExisting(p, stub, "https://example.com/team.git", repo, false); err != nil {
		t.Fatal(err)
	}
	if body, _ := os.ReadFile(p.OpenCodeConfig()); !contains(body, filepath.Join(repo, "skills")) {
		t.Errorf("opencode not wired:\n%s", body)
	}
	if got, err := os.Readlink(filepath.Join(p.CodexSkills(), "my-notes")); err != nil || got != filepath.Join(repo, "skills", "my-notes") {
		t.Errorf("codex link = %q, %v; want collection skill", got, err)
	}
}

func TestPullAllSkipsNonGitWarnsAndContinuesPastFailure(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	t.Setenv("FLEET_REPO", "")
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Explicit non-git entry: a plain directory with no .git.
	nonGit := filepath.Join(t.TempDir(), "plain")
	if err := os.MkdirAll(nonGit, 0o755); err != nil {
		t.Fatal(err)
	}
	f, _ := config.Load(p.FleetConfigFile())
	f.SetSkillsRepos([]string{nonGit})
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}
	// Fleet-home slots: one good (updated), one good (current).
	goodUpdated := filepath.Join(p.FleetReposDir(), "a-team")
	goodCurrent := filepath.Join(p.FleetReposDir(), "b-team")
	for _, dir := range []string{goodUpdated, goodCurrent} {
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Make the stub answer per-repo: first fleet-home call updates,
	// second is current. Non-git never reaches the runner.
	wrapped := &perRepoStub{revs: []string{"x", "y", "same", "same"}}

	results, err := PullAll(p, wrapped)
	if err != nil {
		t.Fatalf("PullAll() error = %v", err)
	}
	byRepo := map[string]Outcome{}
	for _, r := range results {
		byRepo[r.Repo] = r.Outcome
	}
	if byRepo[filepath.Clean(nonGit)] != OutcomeSkipped {
		t.Errorf("non-git outcome = %q, want skipped (results=%+v)", byRepo[filepath.Clean(nonGit)], results)
	}
	if byRepo[filepath.Clean(goodUpdated)] != OutcomeUpdated {
		t.Errorf("updated repo outcome = %q, want updated", byRepo[filepath.Clean(goodUpdated)])
	}
	if byRepo[filepath.Clean(goodCurrent)] != OutcomeCurrent {
		t.Errorf("current repo outcome = %q, want current", byRepo[filepath.Clean(goodCurrent)])
	}
	// Order follows the tracked set: explicit first, then fleet-home alphabetical.
	if len(results) != 3 || results[0].Repo != filepath.Clean(nonGit) {
		t.Errorf("results order = %+v, want explicit non-git first", results)
	}
}

type perRepoStub struct {
	revs   []string
	revN   int
	remote string
}

func (s *perRepoStub) Clone(url, path string) error {
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		return err
	}
	return nil
}
func (s *perRepoStub) RemoteURL(path string) (string, error) {
	return s.remote, nil
}
func (s *perRepoStub) Status(path string) (string, error) { return "", nil }
func (s *perRepoStub) PullFFOnly(path string) error {
	return nil
}
func (s *perRepoStub) RevParse(path string) (string, error) {
	if s.revN >= len(s.revs) {
		return "same", nil
	}
	v := s.revs[s.revN]
	s.revN++
	return v, nil
}

func TestPullOneFreshVsExistingAndNonGitError(t *testing.T) {
	p := pullTestHome(t)
	url := "https://example.com/team.git"
	dest := DefaultPath(p, url)
	stub := &stubRunner{remote: url, before: "a", after: "b"}

	res, err := PullOne(p, stub, url, dest, false)
	if err != nil {
		t.Fatalf("PullOne fresh error = %v", err)
	}
	if res.Outcome != OutcomeCloned {
		t.Errorf("fresh outcome = %q, want cloned", res.Outcome)
	}
	// Existing path with same remote fast-forwards.
	stub.before, stub.after = "b", "b"
	stub.revCalls = 0
	res, err = PullOne(p, stub, url, dest, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != OutcomeCurrent {
		t.Errorf("existing outcome = %q, want current", res.Outcome)
	}
	// Existing non-git path fails (bare would skip).
	plain := filepath.Join(t.TempDir(), "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := PullOne(p, stub, url, plain, false); err == nil {
		t.Error("PullOne onto non-git path should fail")
	}
}

func TestPullAllMissingGitAbortsCleanly(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	t.Setenv("FLEET_REPO", "")
	repo := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stub := &stubRunner{statusErr: ErrGitMissing}
	if _, err := PullAll(p, stub); !errors.Is(err, ErrGitMissing) {
		t.Errorf("PullAll missing git error = %v, want ErrGitMissing", err)
	}
}
