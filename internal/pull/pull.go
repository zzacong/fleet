// Package pull owns the `skill pull` flow: cloning a customs repo into
// the auto-tracked fleet-home slot (or an explicit path) and
// fast-forwarding every tracked repo on a bare pull. Git itself runs
// behind the injected Runner seam so tests never shell out to real git.
package pull

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/scan"
	"github.com/zzacong/fleet/internal/trackedset"
)

// DeriveDirName derives a checkout directory name from a git URL: the
// final path or colon segment, with trailing slashes and a trailing .git
// suffix stripped. Anything git accepts as a URL passes through untouched
// everywhere else; only the directory name is derived here.
func DeriveDirName(url string) string {
	s := strings.TrimSpace(url)
	s = strings.TrimRight(s, "/")
	if s == "" {
		return ""
	}
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		s = s[idx+1:]
	}
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		s = s[idx+1:]
	}
	s = strings.TrimSuffix(s, ".git")
	return s
}

// DefaultPath derives the fleet-home checkout slot for a URL: the derived
// directory name under the fleet-home checkout parent.
func DefaultPath(p *paths.Paths, url string) string {
	return filepath.Join(p.FleetReposDir(), DeriveDirName(url))
}

// SameRemote reports whether two remote URLs name the same repo for pull's
// guard: trailing slashes and a trailing .git suffix are ignored. The
// comparison is otherwise exact; pull never rewrites URLs.
func SameRemote(a, b string) bool {
	return normalizeRemote(a) == normalizeRemote(b)
}

func normalizeRemote(url string) string {
	s := strings.TrimSpace(url)
	s = strings.TrimRight(s, "/")
	s = strings.TrimSuffix(s, ".git")
	return s
}

// ErrGitMissing is the clean dependency error when the git binary is
// absent. Every Runner method returns it (wrapped with context) instead
// of an exec lookup failure so the CLI can hint what to install.
var ErrGitMissing = errors.New("git not found in PATH: install git to use skill pull")

// Runner is the single new seam pull adds: git behind an injected
// interface, mirroring the skills-CLI runner pattern. The real runner is
// Exec (shells out with inherited stdio so the user's SSH agent and
// credential helper just work; fleet adds no credential flags). Tests
// inject a stub recording requested operations.
type Runner interface {
	// Clone runs `git clone <url> <path>` with inherited stdio.
	Clone(url, path string) error
	// RemoteURL returns the checkout's origin URL (`git remote get-url origin`).
	RemoteURL(path string) (string, error)
	// Status returns `git status --porcelain` output: empty means clean.
	Status(path string) (string, error)
	// PullFFOnly runs `git pull --ff-only` with inherited stdio.
	PullFFOnly(path string) error
	// RevParse returns the checkout's HEAD hash (`git rev-parse HEAD`).
	RevParse(path string) (string, error)
}

// Exec is the production Runner: os/exec with inherited stdio for the
// mutating commands (clone, pull) and captured output for the queries.
type Exec struct{}

// Clone implements Runner.
func (Exec) Clone(url, path string) error {
	if err := checkGit(); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", url, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return gitErr(err)
	}
	return nil
}

// RemoteURL implements Runner.
func (Exec) RemoteURL(path string) (string, error) {
	if err := checkGit(); err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", "-C", path, "remote", "get-url", "origin")
	cmd.Stdin = os.Stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git remote get-url origin in %s: %v: %s", path, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Status implements Runner.
func (Exec) Status(path string) (string, error) {
	if err := checkGit(); err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	cmd.Stdin = os.Stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git status in %s: %v: %s", path, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// PullFFOnly implements Runner.
func (Exec) PullFFOnly(path string) error {
	if err := checkGit(); err != nil {
		return err
	}
	cmd := exec.Command("git", "-C", path, "pull", "--ff-only")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return gitErr(err)
	}
	return nil
}

// RevParse implements Runner.
func (Exec) RevParse(path string) (string, error) {
	if err := checkGit(); err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", "-C", path, "rev-parse", "HEAD")
	cmd.Stdin = os.Stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse HEAD in %s: %v: %s", path, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func checkGit() error {
	if _, err := exec.LookPath("git"); err != nil {
		return ErrGitMissing
	}
	return nil
}

func gitErr(err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return ErrGitMissing
	}
	var lookErr *exec.Error
	if errors.As(err, &lookErr) {
		return ErrGitMissing
	}
	return err
}

// EnsureCollection ensures the collection subdir (<repo>/skills) exists,
// creating it on demand for the empty-private-repo first run. It reports
// whether the directory was created so the caller can warn.
func EnsureCollection(repoRoot string) (string, bool, error) {
	dir := filepath.Join(repoRoot, "skills")
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return "", false, fmt.Errorf("%s exists and is not a directory", dir)
		}
		return dir, false, nil
	}
	if !os.IsNotExist(err) {
		return "", false, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	return dir, true, nil
}

// Outcome is one pulled repo's result for the per-repo report.
type Outcome string

const (
	// OutcomeCloned: a fresh clone into a new path.
	OutcomeCloned Outcome = "cloned"
	// OutcomeUpdated: a fast-forward that moved HEAD.
	OutcomeUpdated Outcome = "updated"
	// OutcomeCurrent: already up to date, nothing moved.
	OutcomeCurrent Outcome = "current"
	// OutcomeSkipped: a non-git tracked entry on a bare pull.
	OutcomeSkipped Outcome = "skipped"
	// OutcomeFailed: the repo could not be fast-forwarded.
	OutcomeFailed Outcome = "failed"
)

// Result is one repo's pull outcome. CollectionCreated reports the
// empty-repo first run so the caller can warn; Detail carries the skip
// reason or the failure message for skipped/failed rows.
type Result struct {
	Repo              string
	Outcome           Outcome
	Detail            string
	CollectionCreated bool
}

// IsGitRepo reports whether path looks like a git checkout: a .git entry
// (directory in a normal checkout, file in a worktree) exists. It never
// shells out.
func IsGitRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

// WireHome wires the collection dir into the config-path harnesses and
// links every skill it holds for the link-based harnesses, so pulled
// customs are discoverable immediately. It mirrors adopt's tail.
func WireHome(p *paths.Paths, collectionDir string) error {
	if _, err := harness.WireSkillSource(p, collectionDir); err != nil {
		return err
	}
	skills, err := scan.ScanStore(collectionDir)
	if err != nil {
		return err
	}
	for _, s := range skills {
		if _, err := harness.LinkCustomSkill(p, s.Dir, filepath.Join(collectionDir, s.Dir)); err != nil {
			return err
		}
	}
	return nil
}

// CloneNew fresh-clones url into dest (which must not exist), registers
// outside-home checkouts, ensures the collection, and wires the home. It
// returns a Cloned result; CollectionCreated warns on the empty-repo
// first run.
func CloneNew(p *paths.Paths, runner Runner, url, dest string) (*Result, error) {
	clean := filepath.Clean(dest)
	if _, err := os.Stat(clean); err == nil {
		return nil, fmt.Errorf("%s already exists", clean)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		return nil, err
	}
	if err := runner.Clone(url, clean); err != nil {
		if errors.Is(err, ErrGitMissing) {
			return nil, err
		}
		return nil, fmt.Errorf("clone %s into %s: %w", url, clean, err)
	}
	if _, err := trackedset.Remember(p, clean); err != nil {
		return nil, err
	}
	collection, created, err := EnsureCollection(clean)
	if err != nil {
		return nil, err
	}
	if err := WireHome(p, collection); err != nil {
		return nil, err
	}
	return &Result{Repo: clean, Outcome: OutcomeCloned, CollectionCreated: created}, nil
}

// UpdateExisting fast-forwards an existing checkout (fast-forward only, no
// stash, merge, rebase, or reset). An empty url skips the remote guard
// (bare pull); otherwise a different remote fails unless forced. Dirty or
// diverged trees fail surfacing state. Re-cloning onto an existing path
// never touches config.
func UpdateExisting(p *paths.Paths, runner Runner, url, repoPath string, force bool) (*Result, error) {
	clean := filepath.Clean(repoPath)
	if !IsGitRepo(clean) {
		return nil, fmt.Errorf("%s is not a git checkout", clean)
	}
	if url != "" {
		remote, err := runner.RemoteURL(clean)
		if err != nil {
			return nil, err
		}
		if !SameRemote(remote, url) && !force {
			return nil, fmt.Errorf("%s points at a different remote %q (want %q): use --force to pull anyway", clean, remote, url)
		}
	}
	status, err := runner.Status(clean)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(status) != "" {
		return nil, fmt.Errorf("%s has uncommitted changes — resolve by hand (fleet never stashes):\n%s", clean, strings.TrimSpace(status))
	}
	before, err := runner.RevParse(clean)
	if err != nil {
		if errors.Is(err, ErrGitMissing) {
			return nil, err
		}
		before = ""
	}
	if err := runner.PullFFOnly(clean); err != nil {
		if errors.Is(err, ErrGitMissing) {
			return nil, err
		}
		return nil, fmt.Errorf("pull %s: %w (resolve by hand; fleet never merges or rebases)", clean, err)
	}
	after, err := runner.RevParse(clean)
	if err != nil {
		if errors.Is(err, ErrGitMissing) {
			return nil, err
		}
		after = ""
	}
	outcome := OutcomeCurrent
	if before != after {
		outcome = OutcomeUpdated
	}
	collection, created, err := EnsureCollection(clean)
	if err != nil {
		return nil, err
	}
	if err := WireHome(p, collection); err != nil {
		return nil, err
	}
	return &Result{Repo: clean, Outcome: outcome, CollectionCreated: created}, nil
}

// PullOne clones url into dest when dest is absent, or fast-forwards dest
// when it already holds a checkout. A present non-git path is an error
// (bare pull skips those instead). It never reorders config.
func PullOne(p *paths.Paths, runner Runner, url, dest string, force bool) (*Result, error) {
	clean := filepath.Clean(dest)
	if _, err := os.Stat(clean); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		return CloneNew(p, runner, url, clean)
	}
	if !IsGitRepo(clean) {
		return nil, fmt.Errorf("%s exists and is not a git checkout", clean)
	}
	return UpdateExisting(p, runner, url, clean, force)
}

// PullAll fast-forwards every tracked repo (explicit list order, then
// fleet-home slots alphabetically, env prepended — via the tracked set).
// Non-git entries become Skipped results with a warning; other per-repo
// failures become Failed results and the run continues. Only a missing git
// binary or a tracked-set load failure aborts the whole run.
func PullAll(p *paths.Paths, runner Runner) ([]Result, error) {
	repos, err := trackedset.List(p)
	if err != nil {
		return nil, err
	}
	var out []Result
	for _, repo := range repos {
		clean := filepath.Clean(repo)
		if !IsGitRepo(clean) {
			out = append(out, Result{Repo: clean, Outcome: OutcomeSkipped, Detail: "not a git checkout"})
			continue
		}
		res, err := UpdateExisting(p, runner, "", clean, false)
		if err != nil {
			if errors.Is(err, ErrGitMissing) {
				return out, err
			}
			out = append(out, Result{Repo: clean, Outcome: OutcomeFailed, Detail: err.Error()})
			continue
		}
		out = append(out, *res)
	}
	return out, nil
}
