// Package trackedset owns the tracked set of custom-skill homes: the
// explicit collection-dir list from the config's `skillsDirs` key, in
// order. The order is precedence. There is no env override and no
// convention-tracked checkout: every entry is an absolute directory the
// user registered. The unversioned fleet-home fallback is not part of the
// set; display and adopt layers add it where they need it.
//
// This is the only writer of the explicit list (Add/Remove) and the only
// reader of the ordering (List). Doctor calls in instead of re-deriving
// it.
package trackedset

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
// fallback, any path inside fleet home, a path already tracked, a path
// nested inside or containing a tracked dir, and a skill name that collides
// with another tracked home (an explicit dir or the fallback). Re-adding a
// path already in the explicit list is a no-op. A collision with the
// canonical store is allowed — custom outranks canonical and doctor reports
// the shadow.
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
	if err := checkNested(clean, existing); err != nil {
		return nil, err
	}

	skills, err := scan.ScanStore(clean)
	if err != nil {
		return nil, fmt.Errorf("add-dir: cannot scan %q: %w", clean, err)
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("add-dir: %q holds no skills — a collection dir's immediate children must each hold a SKILL.md", clean)
	}
	if err := checkNameCollisions(p, clean, skills, existing); err != nil {
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
		if paths.IsUnder(clean, d) {
			return fmt.Errorf("add-dir: %q is inside tracked dir %q", clean, d)
		}
		if paths.IsUnder(d, clean) {
			return fmt.Errorf("add-dir: %q contains tracked dir %q", clean, d)
		}
	}
	return nil
}

// checkNameCollisions refuses a skill name the incoming collection shares
// with another tracked home (an explicit dir or the fallback), listing every
// collision and the home already holding it. A canonical-store collision is
// deliberately not checked: custom outranks canonical, and doctor reports
// the shadow.
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

// List returns the explicit skillsDirs collection dirs in precedence
// order, cleaned. It is the one reader of the order. Duplicate or nested
// entries a hand-edited config carries are preserved here so doctor can
// report them; scanning callers dedupe.
func List(p *paths.Paths) ([]string, error) {
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return nil, err
	}
	dirs := f.SkillsDirs()
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		out = append(out, filepath.Clean(dir))
	}
	return out, nil
}
