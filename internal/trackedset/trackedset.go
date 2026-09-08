// Package trackedset owns the tracked set of versioned custom-skill
// homes: the ordered repo roots every reader resolves. The order is the
// FLEET_REPO env override when set (prepended, highest precedence, kept
// working in code only and never persisted), then the config's explicit
// non-fleet-home list in order, then every fleet-home checkout slot
// present on disk (immediate child directories of the fleet-home checkout
// parent, alphabetical by directory name). Entries are deduped by cleaned
// path with the first occurrence winning. The unversioned fleet-home
// fallback is not part of the set; display and adopt layers add it where
// they need it.
//
// This is the only writer of the explicit list (Remember/Forget) and the
// only reader of the ordering (List/CollectionDirs/Resolve). Snapshot,
// doctor, adopt, pull, and drop call in instead of re-deriving it.
package trackedset

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

// List resolves the ordered tracked set of versioned custom-skill homes
// (repo roots). A missing checkout parent means no auto-tracked slots,
// not an error. The retired single-pointer file key is never read:
// old-key-only configs resolve as if unset.
func List(p *paths.Paths) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(path string) {
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		if seen[clean] {
			return
		}
		seen[clean] = true
		out = append(out, clean)
	}
	if env := os.Getenv("FLEET_REPO"); env != "" {
		abs, err := filepath.Abs(env)
		if err != nil {
			return nil, err
		}
		add(abs)
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return nil, err
	}
	for _, repo := range f.SkillsRepos() {
		add(repo)
	}
	entries, err := os.ReadDir(p.FleetReposDir())
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		add(filepath.Join(p.FleetReposDir(), name))
	}
	return out, nil
}

// CollectionDirs returns each tracked repo root's skills-collection subdir
// in List order. Entry i is the collection of List entry i: List already
// dedupes by cleaned root, and distinct cleaned roots yield distinct
// collections, so the two slices stay parallel and callers needing root
// labels can zip them.
func CollectionDirs(p *paths.Paths) ([]string, error) {
	repos, err := List(p)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(repos))
	for _, root := range repos {
		out = append(out, filepath.Clean(filepath.Join(root, "skills")))
	}
	return out, nil
}

// Remember applies the config-write rule: repo roots landing inside fleet
// home write no config (auto-tracked by convention); roots outside append
// to the explicit list (no duplicates, appending preserves existing
// order). It reports whether the file was written.
func Remember(p *paths.Paths, repoRoot string) (bool, error) {
	clean := filepath.Clean(repoRoot)
	if p.InsideFleetHome(clean) {
		return false, nil
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return false, err
	}
	for _, existing := range f.SkillsRepos() {
		if filepath.Clean(existing) == clean {
			return false, nil
		}
	}
	f.SetSkillsRepos(append(f.SkillsRepos(), clean))
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		return false, err
	}
	return true, nil
}

// Forget removes repoRoot from the explicit `skillsRepos` list, preserving
// the order of the rest. It saves only when the entry was present, so
// forgetting a convention-tracked checkout never creates or rewrites the
// config file. It reports whether the file was written.
func Forget(p *paths.Paths, repoRoot string) (bool, error) {
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return false, err
	}
	kept := make([]string, 0, len(f.SkillsRepos()))
	removed := false
	for _, existing := range f.SkillsRepos() {
		if filepath.Clean(existing) == repoRoot {
			removed = true
			continue
		}
		kept = append(kept, existing)
	}
	if !removed {
		return false, nil
	}
	f.SetSkillsRepos(kept)
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		return false, err
	}
	return true, nil
}

// Resolve maps a `<path-or-name>` argument onto the tracked set: either an
// exact repo-root path match or a fleet-home slot name. Relative paths are
// absolutized against the working directory before matching. An unknown
// target errors listing the tracked repos.
func Resolve(p *paths.Paths, arg string) (string, error) {
	repos, err := List(p)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(arg)
	for _, r := range repos {
		if r == clean {
			return r, nil
		}
	}
	if !filepath.IsAbs(clean) {
		if abs, absErr := filepath.Abs(clean); absErr == nil {
			for _, r := range repos {
				if r == abs {
					return r, nil
				}
			}
		}
		if slot := filepath.Join(p.FleetReposDir(), clean); slot != clean {
			for _, r := range repos {
				if r == slot {
					return r, nil
				}
			}
		}
	}
	return "", unknownTargetError(arg, repos)
}

func unknownTargetError(arg string, repos []string) error {
	if len(repos) == 0 {
		return fmt.Errorf("unknown target %q: no skills repos are tracked", arg)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "unknown target %q: tracked repos:", arg)
	for _, r := range repos {
		b.WriteString("\n- ")
		b.WriteString(r)
	}
	return errors.New(b.String())
}
