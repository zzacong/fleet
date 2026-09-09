// Custom-skill visibility: the one fan-out that makes a collection dir
// discoverable in every installed harness. Config-path harnesses (opencode,
// pi) get the dir wired as an extra skill-discovery source; link-based
// harnesses (codex, claude code, Cursor, Bob) get a managed symlink per
// skill the collection holds. Adopt, pull, and re-ensure all call in here
// instead of repeating the wire + scan + link tail.
package customs

import (
	"path/filepath"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/scan"
)

// Visibility is what one make-visible run did. Empty slices mean nothing
// to do (already visible: both primitives are idempotent).
type Visibility struct {
	// Wired lists the config-path harnesses the collection dir was wired
	// into this run.
	Wired []harness.WireResult
	// Linked lists the managed links created or repointed this run, one
	// entry per harness per skill in the collection.
	Linked []harness.LinkResult
}

// Withdrawal is what one withdraw run undid: the inverse of Visibility.
type Withdrawal struct {
	// Unwired lists the config-path harnesses the collection dir was
	// unwired from this run.
	Unwired []harness.UnwireResult
	// Unlinked lists the managed links removed this run.
	Unlinked []harness.UnlinkResult
}

// MakeVisible wires collectionDir into the config-path harnesses and links
// every skill it holds for the link-based harnesses, so customs are
// discoverable immediately. It fails fast on the first harness error,
// matching the tails it replaces.
func MakeVisible(p *paths.Paths, collectionDir string) (*Visibility, error) {
	wired, err := harness.WireSkillSource(p, collectionDir)
	if err != nil {
		return nil, err
	}
	skills, err := scan.ScanStore(collectionDir)
	if err != nil {
		return nil, err
	}
	vis := &Visibility{Wired: wired}
	for _, s := range skills {
		linked, err := harness.LinkCustomSkill(p, s.Dir, filepath.Join(collectionDir, s.Dir))
		if err != nil {
			return nil, err
		}
		vis.Linked = append(vis.Linked, linked...)
	}
	return vis, nil
}

// Withdraw unwires collectionDir from the config-path harnesses and
// unlinks its managed links: the inverse of MakeVisible, run on drop.
// It fails fast on the first harness error.
func Withdraw(p *paths.Paths, collectionDir string) (*Withdrawal, error) {
	unwired, err := harness.UnwireSkillSource(p, collectionDir)
	if err != nil {
		return nil, err
	}
	unlinked, err := harness.RemoveCustomLinks(p, collectionDir)
	if err != nil {
		return nil, err
	}
	return &Withdrawal{Unwired: unwired, Unlinked: unlinked}, nil
}
