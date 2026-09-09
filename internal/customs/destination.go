// Adopt destination resolution for the multi-repo model.
//
// Resolution order (decided by the CLI layer): an explicit --into collection
// dir for the run, else the configured adopt target, else the prompt/default
// rules over the skill index's custom homes. This file owns the
// explicit-target adopt flow that moves into any collection dir — tracked
// or unscanned — without touching config.
package customs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/skillindex"
)

// AdoptTo migrates one skill into the explicit target collection dir,
// creating it on demand. A canonical-store hit moves atomically into the
// target; a hit in exactly one custom home moves nothing but re-ensures the
// wiring and links for the skill's actual home. A name present in more than
// one source is a double-presence error to resolve by hand. An unscanned
// target still proceeds (doctor warns elsewhere). No config file is written.
func AdoptTo(p *paths.Paths, name, target string) (*Report, error) {
	idx, errs, err := skillindex.Load(p)
	if err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("scan custom skills: %s", firstErr(idx, errs))
	}
	return adoptInto(p, idx, name, filepath.Clean(target))
}

// firstErr reports one per-home scan failure for the adopt error line.
func firstErr(idx *skillindex.Index, errs map[string]error) error {
	for _, home := range idx.Homes() {
		if err, ok := errs[home]; ok {
			return err
		}
	}
	return nil
}

// adoptInto is the shared adopt flow parameterized by destination target.
// Hits come from the skill index: the store copy (if any) moves, a lone
// custom copy is re-ensured in place.
func adoptInto(p *paths.Paths, idx *skillindex.Index, name, target string) (*Report, error) {
	var storeHit *skillindex.Hit
	var customHits []skillindex.Hit
	for _, h := range idx.Lookup(name) {
		h := h
		if h.Home == idx.Store() {
			if storeHit == nil {
				storeHit = &h
			}
			continue
		}
		customHits = append(customHits, h)
	}

	switch {
	case storeHit != nil && len(customHits) > 0:
		return nil, fmt.Errorf("skill %q exists in both %s and %s — resolve by hand before adopting", name, p.SkillsStore(), customHits[0].Home)
	case len(customHits) > 1:
		return nil, fmt.Errorf("skill %q exists in both %s and %s — resolve by hand before adopting", name, customHits[0].Home, customHits[1].Home)
	case storeHit == nil && len(customHits) == 0:
		return nil, fmt.Errorf("skill %q not found in %s", name, p.SkillsStore())
	}

	rep := &Report{}
	var wireTarget string
	switch {
	case storeHit != nil:
		rep.Skill = storeHit.Skill.Dir
		rep.From = filepath.Join(p.SkillsStore(), storeHit.Skill.Dir)
		rep.To = filepath.Join(target, storeHit.Skill.Dir)
		wireTarget = target
		rep.Moved = true
		if err := os.MkdirAll(wireTarget, 0o755); err != nil {
			return nil, err
		}
		if err := moveDir(rep.From, rep.To); err != nil {
			return nil, fmt.Errorf("move %s into %s: %w", storeHit.Skill.Dir, target, err)
		}
	default:
		rep.Skill = customHits[0].Skill.Dir
		actualHome := customHits[0].Home
		rep.To = filepath.Join(actualHome, customHits[0].Skill.Dir)
		wireTarget = actualHome
		if err := os.MkdirAll(wireTarget, 0o755); err != nil {
			return nil, err
		}
	}

	wired, err := harness.WireSkillSource(p, wireTarget)
	if err != nil {
		return nil, err
	}
	linked, err := harness.LinkCustomSkill(p, rep.Skill, rep.To)
	if err != nil {
		return nil, err
	}
	rep.Wired, rep.Linked = wired, linked
	return rep, nil
}
