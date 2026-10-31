// Archiving a custom skill: the one verb that moves a custom skill's files
// out of its collection dir. The copy goes into an archived/ folder beside
// its collection dir, then every managed link and state entry for it goes.
package customs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/skillindex"
	"github.com/zzacong/fleet/internal/state"
)

// Archival is what one archive run did. To is the skill's new location;
// Unlinked lists the managed links removed this run.
type Archival struct {
	To       string
	Unlinked []harness.UnlinkResult
}

// CustomCopies returns every custom-home copy of the skill directory name,
// precedence-first. The canonical store never counts as a custom copy, so
// an installed skill yields none. It fails when any home cannot be scanned,
// because a missing copy could then be hiding among the unreadable ones.
func CustomCopies(p *paths.Paths, name string) ([]skillindex.Hit, error) {
	idx, err := loadScanned(p)
	if err != nil {
		return nil, err
	}
	var out []skillindex.Hit
	for _, h := range idx.Ordered(name) {
		if h.Home != idx.Store() {
			out = append(out, h)
		}
	}
	return out, nil
}

// CanonicalMatch returns the canonical store skill that any of names matches
// by directory or frontmatter name, and false when the store holds none. It
// fails when any home cannot be scanned, since a missing match could then be
// hiding among the unreadable ones.
func CanonicalMatch(p *paths.Paths, names ...string) (skillindex.Hit, bool, error) {
	idx, err := loadScanned(p)
	if err != nil {
		return skillindex.Hit{}, false, err
	}
	for _, name := range names {
		for _, h := range idx.Lookup(name) {
			if h.Home == idx.Store() {
				return h, true, nil
			}
		}
	}
	return skillindex.Hit{}, false, nil
}

// ShadowError is the refusal for archiving a custom copy named name whose
// name matches the canonical skill canon: archiving would expose canon in
// every harness.
func ShadowError(name string, canon skillindex.Hit) error {
	return fmt.Errorf("archive: %q shadows canonical skill %q at %s; archiving it would expose the installed copy in every harness", name, canon.Skill.Dir, filepath.Join(canon.Home, canon.Skill.Dir))
}

// loadScanned loads the skill index and fails when any home cannot be
// scanned, so callers never answer from a partial view.
func loadScanned(p *paths.Paths) (*skillindex.Index, error) {
	idx, errs, err := skillindex.Load(p)
	if err != nil {
		return nil, err
	}
	for _, home := range idx.Homes() {
		if err, ok := errs[home]; ok {
			return nil, fmt.Errorf("scan %s: %w", home, err)
		}
	}
	return idx, nil
}

// Archive moves one custom skill copy into the archived/ folder beside its
// collection dir (the fleet-home fallback's goes to the fleet home's
// archived/), unlinks its managed link from every installed harness, and
// drops its state entries. It refuses a copy whose name shadows a canonical
// skill or whose target already exists in archived/, before anything
// changes. The state is loaded before anything changes and the move runs
// before any unlink, so a failed move leaves links and state as they were.
// Sync is the caller's to run. The archive is one-way.
func Archive(p *paths.Paths, hit skillindex.Hit) (*Archival, error) {
	name := hit.Skill.Dir
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		return nil, err
	}

	canon, shadowed, err := CanonicalMatch(p, name, hit.Skill.Name)
	if err != nil {
		return nil, err
	}
	if shadowed {
		return nil, ShadowError(name, canon)
	}

	archivedDir := filepath.Join(filepath.Dir(hit.Home), "archived")
	to := filepath.Join(archivedDir, name)
	if _, err := os.Lstat(to); err == nil {
		return nil, fmt.Errorf("archive: %q already exists at %s; rename or remove the existing archived folder first", name, to)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		return nil, err
	}
	if err := moveDir(filepath.Join(hit.Home, name), to); err != nil {
		return nil, fmt.Errorf("archive %s: %w", name, err)
	}

	unlinked, err := unlinkSkill(p, name, hit.Home)
	if err != nil {
		return nil, err
	}

	st.RemoveSkill(name)
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		return nil, err
	}
	return &Archival{To: to, Unlinked: unlinked}, nil
}

// unlinkSkill removes name's managed link from every installed harness that
// links skills. Only links that resolve into home are touched, the same
// rule remove-dir applies, so links to other copies or the store stay.
func unlinkSkill(p *paths.Paths, name, home string) ([]harness.UnlinkResult, error) {
	var out []harness.UnlinkResult
	for _, a := range harness.Installed(p) {
		res, removed, err := harness.RemoveCustomSkillLink(p, a, name, []string{home})
		if err != nil {
			return nil, err
		}
		if removed {
			out = append(out, res)
		}
	}
	return out, nil
}
