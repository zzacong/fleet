// Custom-skill visibility: the one fan-out that makes a collection dir
// discoverable in every installed harness. Every harness reaches a custom
// skill through a managed symlink per skill the collection holds. Adopt,
// pull, and re-ensure all call in here instead of repeating the scan +
// link tail.
package customs

import (
	"os"
	"path/filepath"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/scan"
	"github.com/zzacong/fleet/internal/skillindex"
	"github.com/zzacong/fleet/internal/state"
)

// Visibility is what one make-visible run did. Empty slices mean nothing
// to do (already visible: the link primitive is idempotent).
type Visibility struct {
	// Linked lists the managed links created or repointed this run, one
	// entry per harness per skill in the collection.
	Linked []harness.LinkResult
}

// Withdrawal is what one withdraw run undid: the inverse of Visibility.
type Withdrawal struct {
	// Unlinked lists the managed links removed this run.
	Unlinked []harness.UnlinkResult
}

// MakeVisible links every skill collectionDir holds for every installed
// harness, so customs are discoverable immediately. A skill the state
// disables on a link-toggleable harness is left unlinked: for those the
// link is the only disable lever, so its absence is the disable.
// It fails fast on the first harness error, matching the tails it replaces.
func MakeVisible(p *paths.Paths, collectionDir string) (*Visibility, error) {
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		return nil, err
	}
	toggleable := linkToggleableSet(p)
	skills, err := scan.ScanStore(collectionDir)
	if err != nil {
		return nil, err
	}
	vis := &Visibility{}
	for _, s := range skills {
		keep := func(h harness.Harness) bool {
			return !toggleable[h] || !st.IsDisabled(s.Name, string(h))
		}
		linked, err := harness.LinkCustomSkill(p, s.Dir, filepath.Join(collectionDir, s.Dir), keep)
		if err != nil {
			return nil, err
		}
		vis.Linked = append(vis.Linked, linked...)
	}
	return vis, nil
}

// linkToggleableSet names the installed harnesses whose only lever over a
// custom skill is its managed link.
func linkToggleableSet(p *paths.Paths) map[harness.Harness]bool {
	set := map[harness.Harness]bool{}
	for _, a := range harness.All(p) {
		if harness.LinkToggleable(a) {
			set[a.Harness()] = true
		}
	}
	return set
}

// PruneHiddenLinks removes the managed custom link for every custom skill
// the state disables on a link-toggleable harness. It is the removal half
// of a link toggle's projection: MakeVisible only skips creating the link,
// so a disable recorded after the link existed is cleaned up here. Foreign
// links and links outside every custom home are never touched.
func PruneHiddenLinks(p *paths.Paths) ([]harness.UnlinkResult, error) {
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		return nil, err
	}
	homes, err := skillindex.CustomHomes(p)
	if err != nil {
		return nil, err
	}
	var out []harness.UnlinkResult
	for _, a := range harness.Installed(p) {
		if !harness.LinkToggleable(a) {
			continue
		}
		for _, name := range st.Disabled(string(a.Harness())) {
			res, removed, err := harness.RemoveCustomSkillLink(p, a, name, homes)
			if err != nil {
				return nil, err
			}
			if removed {
				out = append(out, res)
			}
		}
	}
	return out, nil
}

// EnsureVisible makes every scanned custom home discoverable: it is
// MakeVisible applied to the skill index's custom homes (each tracked
// collection plus the fleet-home fallback, which is where a configured
// adopt target lands when it is tracked). Sync calls it so customs stay
// visible without an adopt or pull run. It is idempotent like the
// primitive underneath, and homes that do not exist on disk are skipped:
// linking into a phantom home would make every later run report a change.
func EnsureVisible(p *paths.Paths) (*Visibility, error) {
	homes, err := skillindex.CustomHomes(p)
	if err != nil {
		return nil, err
	}
	vis := &Visibility{}
	for _, home := range homes {
		info, err := os.Stat(home)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		one, err := MakeVisible(p, home)
		if err != nil {
			return nil, err
		}
		vis.Linked = append(vis.Linked, one.Linked...)
	}
	return vis, nil
}

// Withdraw unlinks collectionDir's managed links: the inverse of
// MakeVisible, run on drop. It fails fast on the first harness error.
func Withdraw(p *paths.Paths, collectionDir string) (*Withdrawal, error) {
	unlinked, err := harness.RemoveCustomLinks(p, collectionDir)
	if err != nil {
		return nil, err
	}
	return &Withdrawal{Unlinked: unlinked}, nil
}
