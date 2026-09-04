package drop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/pull"
)

// stubRunner is the injected git seam for drop tests: it answers `git
// status --porcelain` from scripted values and records whether the dirty
// check ran. No test shells out to real git.
type stubRunner struct {
	status    string
	statusErr error

	statusCalls []string
}

func (s *stubRunner) Clone(url, path string) error { return nil }

func (s *stubRunner) RemoteURL(path string) (string, error) { return "", nil }

func (s *stubRunner) Status(path string) (string, error) {
	s.statusCalls = append(s.statusCalls, path)
	if s.statusErr != nil {
		return "", s.statusErr
	}
	return s.status, nil
}

func (s *stubRunner) PullFFOnly(path string) error { return nil }

func (s *stubRunner) RevParse(path string) (string, error) { return "", nil }

func dropTestPaths(t *testing.T) *paths.Paths {
	t.Helper()
	t.Setenv("FLEET_REPO", "")
	return paths.New(filepath.Join(t.TempDir(), "home"))
}

func trackExplicit(t *testing.T, p *paths.Paths, repos ...string) {
	t.Helper()
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetSkillsRepos(repos)
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}
}

func trackedOrFail(t *testing.T, p *paths.Paths) []string {
	t.Helper()
	repos, err := p.TrackedRepos()
	if err != nil {
		t.Fatal(err)
	}
	return repos
}

func TestResolveExactPath(t *testing.T) {
	p := dropTestPaths(t)
	outside := filepath.Join(t.TempDir(), "customs")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	trackExplicit(t, p, outside)

	got, err := Resolve(p, outside)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != filepath.Clean(outside) {
		t.Errorf("Resolve() = %q, want %q", got, outside)
	}
}

func TestResolveSlotName(t *testing.T) {
	p := dropTestPaths(t)
	slot := filepath.Join(p.FleetReposDir(), "team-customs")
	if err := os.MkdirAll(slot, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(p, "team-customs")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != filepath.Clean(slot) {
		t.Errorf("Resolve() = %q, want %q", got, slot)
	}
}

func TestResolveUnknownListsTracked(t *testing.T) {
	p := dropTestPaths(t)
	outside := filepath.Join(t.TempDir(), "customs")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	trackExplicit(t, p, outside)
	slot := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(slot, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve(p, filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("Resolve(unknown) should fail")
	}
	for _, want := range []string{filepath.Clean(outside), filepath.Clean(slot)} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should list tracked repo %q, got: %v", want, err)
		}
	}
}

func TestDropExplicitUnlistsPreservingOrderDiskUntouched(t *testing.T) {
	p := dropTestPaths(t)
	dir := t.TempDir()
	repoA := filepath.Join(dir, "a")
	repoB := filepath.Join(dir, "b")
	repoC := filepath.Join(dir, "c")
	for _, r := range []string{repoA, repoB, repoC} {
		if err := os.MkdirAll(r, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(repoB, "skills", "keep", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	trackExplicit(t, p, repoA, repoB, repoC)

	res, err := Drop(p, &stubRunner{}, repoB, false)
	if err != nil {
		t.Fatalf("Drop() error = %v", err)
	}
	if res.Repo != filepath.Clean(repoB) {
		t.Errorf("Repo = %q, want %q", res.Repo, repoB)
	}
	if res.FleetHome {
		t.Error("FleetHome = true for an explicit repo, want false")
	}
	got := trackedOrFail(t, p)
	// Only the config list remains (no fleet-home slots in this home).
	if len(got) != 2 || got[0] != filepath.Clean(repoA) || got[1] != filepath.Clean(repoC) {
		t.Errorf("tracked after drop = %q, want [%q %q] in order", got, repoA, repoC)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("explicit drop touched disk: %v", err)
	}
}

func TestDropFleetHomeDeletesCheckout(t *testing.T) {
	p := dropTestPaths(t)
	slot := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(filepath.Join(slot, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Drop(p, &stubRunner{}, "team", false)
	if err != nil {
		t.Fatalf("Drop() error = %v", err)
	}
	if !res.FleetHome {
		t.Error("FleetHome = false for a fleet-home checkout, want true")
	}
	if _, err := os.Stat(slot); !os.IsNotExist(err) {
		t.Errorf("checkout still on disk after drop, stat err = %v", err)
	}
	if repos := trackedOrFail(t, p); len(repos) != 0 {
		t.Errorf("tracked after fleet-home drop = %q, want empty", repos)
	}
}

func TestDropDirtyFailsSurfacingState(t *testing.T) {
	p := dropTestPaths(t)
	slot := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(filepath.Join(slot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stub := &stubRunner{status: " M skills/foo/SKILL.md"}

	_, err := Drop(p, stub, "team", false)
	if err == nil {
		t.Fatal("dirty tree should fail without --force")
	}
	if !strings.Contains(err.Error(), "M skills/foo/SKILL.md") {
		t.Errorf("error should surface working-tree state, got: %v", err)
	}
	if _, statErr := os.Stat(slot); statErr != nil {
		t.Errorf("failed drop deleted the checkout: %v", statErr)
	}
}

func TestDropDirtyForceProceeds(t *testing.T) {
	p := dropTestPaths(t)
	slot := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(filepath.Join(slot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stub := &stubRunner{status: " M skills/foo/SKILL.md"}

	if _, err := Drop(p, stub, "team", true); err != nil {
		t.Errorf("dirty tree with force should proceed, got: %v", err)
	}
	if len(stub.statusCalls) != 0 {
		t.Errorf("force should skip the dirty check, calls = %v", stub.statusCalls)
	}
	if _, err := os.Stat(slot); !os.IsNotExist(err) {
		t.Errorf("forced drop did not delete checkout, stat err = %v", err)
	}
}

func TestDropMissingGitWarnsAndProceeds(t *testing.T) {
	p := dropTestPaths(t)
	slot := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(filepath.Join(slot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stub := &stubRunner{statusErr: pull.ErrGitMissing}

	res, err := Drop(p, stub, "team", false)
	if err != nil {
		t.Fatalf("missing git should warn and proceed, got: %v", err)
	}
	if res.Warning == "" {
		t.Error("Warning = empty, want the missing-git warning")
	}
	if _, err := os.Stat(slot); !os.IsNotExist(err) {
		t.Errorf("drop did not delete checkout, stat err = %v", err)
	}
}

func TestDropNonGitSkipsDirtyCheck(t *testing.T) {
	p := dropTestPaths(t)
	outside := filepath.Join(t.TempDir(), "plain")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	trackExplicit(t, p, outside)
	stub := &stubRunner{}

	if _, err := Drop(p, stub, outside, false); err != nil {
		t.Fatalf("Drop() error = %v", err)
	}
	if len(stub.statusCalls) != 0 {
		t.Errorf("non-git repo should skip the dirty check, calls = %v", stub.statusCalls)
	}
}

func TestDropAdoptTargetInsideFailsEvenForced(t *testing.T) {
	p := dropTestPaths(t)
	slot := filepath.Join(p.FleetReposDir(), "team")
	collection := filepath.Join(slot, "skills")
	if err := os.MkdirAll(collection, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetAdoptTarget(collection)
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}

	for _, force := range []bool{false, true} {
		_, err := Drop(p, &stubRunner{}, "team", force)
		if err == nil {
			t.Fatalf("adoptTarget inside target should fail (force=%v)", force)
		}
		if !strings.Contains(err.Error(), "fleet config set adopt-target") {
			t.Errorf("error should hint at re-pointing adopt-target, got: %v", err)
		}
	}
	if _, err := os.Stat(slot); err != nil {
		t.Errorf("blocked drop deleted the checkout: %v", err)
	}
}

func TestDropAdoptTargetElsewhereProceeds(t *testing.T) {
	p := dropTestPaths(t)
	slot := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(slot, 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(p.FleetReposDir(), "other", "skills")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetAdoptTarget(other)
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}

	if _, err := Drop(p, &stubRunner{}, "team", false); err != nil {
		t.Errorf("adoptTarget elsewhere should not block, got: %v", err)
	}
}

func TestDropAdoptTargetInsideExplicitFails(t *testing.T) {
	p := dropTestPaths(t)
	outside := filepath.Join(t.TempDir(), "customs")
	collection := filepath.Join(outside, "skills")
	if err := os.MkdirAll(collection, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetSkillsRepos([]string{outside})
	f.SetAdoptTarget(collection)
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}

	_, err = Drop(p, &stubRunner{}, outside, true)
	if err == nil {
		t.Fatal("adoptTarget inside explicit target should fail even forced")
	}
	if !strings.Contains(err.Error(), "fleet config set adopt-target") {
		t.Errorf("error should hint at re-pointing adopt-target, got: %v", err)
	}
	if got := trackedOrFail(t, p); len(got) != 1 {
		t.Errorf("blocked drop rewrote the tracked set: %q", got)
	}
}

func TestDropStatusErrorFails(t *testing.T) {
	p := dropTestPaths(t)
	slot := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(filepath.Join(slot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stub := &stubRunner{statusErr: errors.New("git status: corrupt index")}

	if _, err := Drop(p, stub, "team", false); err == nil {
		t.Error("status failure should fail the drop")
	} else if errors.Is(err, pull.ErrGitMissing) {
		t.Errorf("non-git-missing status error misreported: %v", err)
	}
	if _, err := os.Stat(slot); err != nil {
		t.Errorf("failed drop deleted the checkout: %v", err)
	}
}

func TestResolveRelativePath(t *testing.T) {
	p := dropTestPaths(t)
	parent := t.TempDir()
	outside := filepath.Join(parent, "customs")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	trackExplicit(t, p, outside)
	t.Chdir(parent)

	got, err := Resolve(p, "customs")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != filepath.Clean(outside) {
		t.Errorf("Resolve() = %q, want %q", got, outside)
	}
}

func TestDropEnvOnlyFailsWithHint(t *testing.T) {
	p := dropTestPaths(t)
	envRepo := filepath.Join(t.TempDir(), "env-customs")
	if err := os.MkdirAll(filepath.Join(envRepo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLEET_REPO", envRepo)

	_, err := Drop(p, &stubRunner{}, envRepo, false)
	if err == nil {
		t.Fatal("env-only repo should fail")
	}
	if !strings.Contains(err.Error(), "FLEET_REPO") {
		t.Errorf("error should name FLEET_REPO, got: %v", err)
	}
	if _, statErr := os.Stat(p.FleetConfigFile()); !os.IsNotExist(statErr) {
		t.Errorf("failed drop wrote a config file: %v", statErr)
	}
	if _, statErr := os.Stat(envRepo); statErr != nil {
		t.Errorf("failed drop touched the checkout: %v", statErr)
	}
}

func TestDropAdoptTargetEqualRepoFails(t *testing.T) {
	p := dropTestPaths(t)
	slot := filepath.Join(p.FleetReposDir(), "team")
	if err := os.MkdirAll(slot, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetAdoptTarget(slot)
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}

	if _, err := Drop(p, &stubRunner{}, "team", false); err == nil {
		t.Error("adoptTarget equal to the repo should fail")
	} else if !strings.Contains(err.Error(), "fleet config set adopt-target") {
		t.Errorf("error should hint at re-pointing adopt-target, got: %v", err)
	}
}

func TestDropMissingCollectionProceeds(t *testing.T) {
	p := dropTestPaths(t)
	outside := filepath.Join(t.TempDir(), "customs")
	if err := os.MkdirAll(filepath.Join(outside, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	trackExplicit(t, p, outside)

	res, err := Drop(p, &stubRunner{}, outside, false)
	if err != nil {
		t.Fatalf("Drop() error = %v", err)
	}
	if res.Collection != filepath.Join(outside, "skills") {
		t.Errorf("Collection = %q, want the skills subdir", res.Collection)
	}
	if got := trackedOrFail(t, p); len(got) != 0 {
		t.Errorf("repo not unlisted: %q", got)
	}
}
