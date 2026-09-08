// Package drop owns the pure `skill drop` domain operation: resolving a
// `<path-or-name>` target against the tracked set, then either unlisting an
// explicit repo (config rewrite only, disk untouched) or deleting a
// fleet-home checkout from disk. The dirty-tree guard runs behind pull's
// Runner seam so tests never shell out to real git. Harness unwire/unlink
// and sync happen at the CLI layer, not here.
package drop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/pull"
	"github.com/zzacong/fleet/internal/trackedset"
)

// Result describes a completed drop for the CLI layer: Repo is the cleaned
// repo root that was dropped, FleetHome reports whether it was a fleet-home
// checkout deleted from disk (false means an explicit repo unlisted from
// config), Collection is its skills-collection subdir (for harness cleanup),
// and Warning carries the non-fatal missing-git notice when set.
type Result struct {
	Repo       string
	FleetHome  bool
	Collection string
	Warning    string
}

// Drop resolves arg against the tracked set and drops it: explicit repos
// are removed from the `skillsRepos` config list (order of the rest
// preserved, disk untouched); fleet-home checkouts are deleted from disk.
// Guards: a dirty working tree fails surfacing `git status --porcelain`
// output unless force (a missing git binary warns and proceeds instead); an
// adoptTarget pointing inside the target always fails with a re-point hint,
// even when forced. It uses pull's Runner seam for the dirty check, so
// pull.Exec works as the production runner.
func Drop(p *paths.Paths, runner pull.Runner, arg string, force bool) (*Result, error) {
	repo, err := trackedset.Resolve(p, arg)
	if err != nil {
		return nil, err
	}
	fleetHome := p.InsideFleetHome(repo)

	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return nil, err
	}
	// A repo tracked only via the FLEET_REPO env override has no
	// persistent membership to remove: unlisting skips it and there is no
	// checkout to delete, so dropping it would print success while
	// changing nothing.
	if !trackedPersistently(p, f, repo) {
		return nil, fmt.Errorf("cannot drop %q: it is tracked only via FLEET_REPO — unset FLEET_REPO to stop tracking it", repo)
	}
	if target := f.AdoptTarget(); target != "" {
		cleanTarget := filepath.Clean(target)
		if cleanTarget == repo || strings.HasPrefix(cleanTarget, repo+string(filepath.Separator)) {
			return nil, fmt.Errorf("cannot drop %q: adopt target %q is inside it — re-point first with: fleet config set adopt-target <skills-dir>", repo, target)
		}
	}

	warning := ""
	if !force && pull.IsGitRepo(repo) {
		status, err := runner.Status(repo)
		if err != nil {
			if errors.Is(err, pull.ErrGitMissing) {
				warning = "git not found in PATH: skipped dirty check"
			} else {
				return nil, err
			}
		} else if strings.TrimSpace(status) != "" {
			return nil, fmt.Errorf("%s has uncommitted changes — resolve by hand (fleet never stashes):\n%s", repo, strings.TrimSpace(status))
		}
	}

	if _, err := trackedset.Forget(p, repo); err != nil {
		return nil, err
	}
	if fleetHome {
		// fleetHome derives from InsideFleetHome(repo) on the unchanged
		// repo above, so this delete can only ever target the fleet home.
		if err := os.RemoveAll(repo); err != nil {
			return nil, err
		}
	}
	return &Result{
		Repo:       repo,
		FleetHome:  fleetHome,
		Collection: filepath.Join(repo, "skills"),
		Warning:    warning,
	}, nil
}

// trackedPersistently reports whether repo belongs to the on-disk tracked
// set: the explicit config list, or a fleet-home checkout slot present on
// disk. Anything else reached Resolve only through the FLEET_REPO env
// override, which no drop can untrack.
func trackedPersistently(p *paths.Paths, f *config.File, repo string) bool {
	for _, existing := range f.SkillsRepos() {
		if filepath.Clean(existing) == repo {
			return true
		}
	}
	if filepath.Dir(repo) == p.FleetReposDir() {
		if info, err := os.Stat(repo); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}
