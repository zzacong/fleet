package harness

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/zzacong/fleet/internal/paths"
)

// Bob reads ~/.agents/skills natively (verified empirically on this
// machine; the docs only mention ~/.bob/skills) and has no documented
// per-skill disable mechanism, so like Cursor the read side reports every
// skill on. It still reads link presence in ~/.bob/skills: the links are
// how Bob reaches custom skills from the fleet repo (a later ticket) and
// what doctor checks for redundancy.
type BobAdapter struct {
	home *paths.Paths
}

// NewBob builds the Bob adapter under an injected home root.
func NewBob(p *paths.Paths) *BobAdapter { return &BobAdapter{home: p} }

// Harness implements Adapter.
func (a *BobAdapter) Harness() Harness { return Bob }

// Installed implements Adapter.
func (a *BobAdapter) Installed() bool { return isDir(a.home.BobDir()) }

// Read implements Adapter.
func (a *BobAdapter) Read(names []string) (ReadResult, error) {
	res := ReadResult{States: onForAll(names)}

	for _, name := range names {
		if linkPresent(filepath.Join(a.home.BobSkills(), name)) {
			res.Linked = append(res.Linked, name)
		}
	}
	sort.Strings(res.Linked)
	return res, nil
}

// CanProject implements Adapter: Bob's disable path needs live
// verification (it empirically reads the canonical store), so fleet
// writes nothing.
func (a *BobAdapter) CanProject() bool { return false }

// Project implements Adapter; Bob has no write side, so callers must check
// CanProject first.
func (a *BobAdapter) Project(writes []SkillWrite) (WriteReport, error) {
	return WriteReport{}, fmt.Errorf("bob has no verified per-skill disable mechanism")
}
