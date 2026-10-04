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
// only reader of the ordering (List/CollectionDirs/Resolve). The skill
// index, pull, and drop call in instead of re-deriving it.
//
// Add is the same domain seam for the path-tracked model: it owns the
// explicit skillsDirs collection-dir list, appending one validated
// directory at a time with every add rule in one place. Remove is the
// matching unlist side, preserving the order of the rest. Ticket 05
// collapses List onto the same list.
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
	"github.com/zzacong/fleet/internal/scan"
)

// AddResult reports one Add: Dir is the cleaned absolute collection dir.
// Added is false when the cleaned path was already listed, a safe no-op
// rather than a duplicate append.
type AddResult struct {
	Dir   string
	Added bool
}

// RemoveResult reports one Remove: Dir is the cleaned absolute collection
// dir that was unlisted.
type RemoveResult struct {
	Dir string
}

// Remove unlists a collection dir from the explicit skillsDirs list,
// preserving the order of the rest: the inverse of Add, never of adopt. It
// resolves a leading ~ and a relative path against the working directory
// exactly like Add, and it never stats the target, so a tracked dir missing
// from disk still unlists cleanly. The disk is never touched. An untracked
// path errors listing the tracked dirs.
func Remove(p *paths.Paths, arg string) (*RemoveResult, error) {
	clean, err := resolveDirArg(arg)
	if err != nil {
		return nil, err
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return nil, err
	}
	existing := f.SkillsDirs()
	kept := make([]string, 0, len(existing))
	removed := false
	for _, dir := range existing {
		if filepath.Clean(dir) == clean {
			removed = true
			continue
		}
		kept = append(kept, dir)
	}
	if !removed {
		return nil, untrackedDirError(clean, existing)
	}
	f.SetSkillsDirs(kept)
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		return nil, err
	}
	return &RemoveResult{Dir: clean}, nil
}

// untrackedDirError reports a remove-dir path that is not in the explicit
// list, listing every tracked dir so the typo is obvious.
func untrackedDirError(clean string, tracked []string) error {
	if len(tracked) == 0 {
		return fmt.Errorf("remove-dir: %q is not tracked: no dirs are tracked", clean)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "remove-dir: %q is not tracked: tracked dirs:", clean)
	for _, dir := range tracked {
		b.WriteString("\n- ")
		b.WriteString(filepath.Clean(dir))
	}
	return errors.New(b.String())
}

// Add registers an existing collection dir in the explicit skillsDirs list,
// appending it without reordering. It resolves a leading ~ and a relative
// path against the working directory, then applies every add rule, reporting
// the specific refusal: the path must exist, be a directory, hold at least
// one skill, and scan. It refuses the canonical store, the fleet-home
// fallback, any path inside fleet home, a path already tracked by the
// transitional legacy repo list, a path nested inside or containing a
// tracked dir, and a skill name that collides with another tracked home (an
// explicit dir, a legacy collection, or the fallback). Re-adding a path
// already in the explicit list is a no-op. A collision with the canonical
// store is allowed — custom outranks canonical and doctor reports the
// shadow.
func Add(p *paths.Paths, arg string) (*AddResult, error) {
	clean, err := resolveDirArg(arg)
	if err != nil {
		return nil, err
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return nil, err
	}
	existing := f.SkillsDirs()

	info, err := os.Stat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("add-dir: %q does not exist", clean)
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("add-dir: %q is not a directory", clean)
	}
	if clean == filepath.Clean(p.SkillsStore()) {
		return nil, fmt.Errorf("add-dir: %q is the canonical store — installed skills are not custom", clean)
	}
	if clean == filepath.Clean(p.FleetHomeSkills()) {
		return nil, fmt.Errorf("add-dir: %q is the fleet-home fallback — it is always tracked", clean)
	}
	if p.InsideFleetHome(clean) {
		return nil, fmt.Errorf("add-dir: %q is inside fleet home — fleet's config dir cannot be a collection", clean)
	}
	for _, dir := range existing {
		if filepath.Clean(dir) == clean {
			return &AddResult{Dir: clean, Added: false}, nil
		}
	}
	// The transitional legacy repo list is a custom home too: a legacy repo's
	// collection, or a dir nesting with one, is already tracked and would
	// double-scan. Ticket 05 retires that list.
	legacy, err := CollectionDirs(p)
	if err != nil {
		return nil, err
	}
	for _, dir := range legacy {
		if filepath.Clean(dir) == clean {
			return nil, fmt.Errorf("add-dir: %q is already tracked as a legacy repo's collection", clean)
		}
	}
	tracked := append(append([]string(nil), existing...), legacy...)
	if err := checkNested(clean, tracked); err != nil {
		return nil, err
	}

	skills, err := scan.ScanStore(clean)
	if err != nil {
		return nil, fmt.Errorf("add-dir: cannot scan %q: %w", clean, err)
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("add-dir: %q holds no skills — a collection dir's immediate children must each hold a SKILL.md", clean)
	}
	if err := checkNameCollisions(p, clean, skills, tracked); err != nil {
		return nil, err
	}

	f.SetSkillsDirs(append(existing, clean))
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		return nil, err
	}
	return &AddResult{Dir: clean, Added: true}, nil
}

// resolveDirArg expands a leading ~ and absolutizes a relative path against
// the working directory, matching the config path rule, then cleans it. Add
// and Remove share it so both verbs address a directory the same way.
func resolveDirArg(arg string) (string, error) {
	expanded := config.ExpandPath(arg)
	if !filepath.IsAbs(expanded) {
		abs, err := filepath.Abs(expanded)
		if err != nil {
			return "", err
		}
		expanded = abs
	}
	return filepath.Clean(expanded), nil
}

// checkNested refuses a collection dir that sits inside a tracked dir or
// contains one, so precedence between overlapping collections is never
// ambiguous.
func checkNested(clean string, existing []string) error {
	for _, dir := range existing {
		d := filepath.Clean(dir)
		if isInside(clean, d) {
			return fmt.Errorf("add-dir: %q is inside tracked dir %q", clean, d)
		}
		if isInside(d, clean) {
			return fmt.Errorf("add-dir: %q contains tracked dir %q", clean, d)
		}
	}
	return nil
}

// checkNameCollisions refuses a skill name the incoming collection shares
// with another tracked home (an explicit dir or a transitional legacy
// collection) or the fallback, listing every collision and the home already
// holding it. A canonical-store collision is deliberately not checked:
// custom outranks canonical, and doctor reports the shadow.
func checkNameCollisions(p *paths.Paths, clean string, incoming []scan.Skill, existing []string) error {
	homes := append(append([]string(nil), existing...), p.FleetHomeSkills())
	owner := map[string]string{}
	for _, home := range homes {
		h := filepath.Clean(home)
		if h == clean {
			continue
		}
		skills, err := scan.ScanStore(h)
		if err != nil {
			return fmt.Errorf("add-dir: cannot scan tracked dir %q: %w", h, err)
		}
		for _, s := range skills {
			if _, ok := owner[s.Name]; !ok {
				owner[s.Name] = h
			}
		}
	}
	var collisions []string
	for _, s := range incoming {
		if home, ok := owner[s.Name]; ok {
			collisions = append(collisions, fmt.Sprintf("%s (in %s)", s.Name, home))
		}
	}
	if len(collisions) == 0 {
		return nil
	}
	return fmt.Errorf("add-dir: %q collides with already tracked skills: %s", clean, strings.Join(collisions, ", "))
}

// isInside reports whether path is a strict descendant of dir, lexically on
// cleaned paths.
func isInside(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

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
