// Adopt destination resolution for the multi-repo model.
//
// Resolution order (decided by the CLI layer): an explicit --into collection
// dir for the run, else the configured adopt target, else the prompt/default
// rules over AdoptCandidates. This file owns the two pieces the CLI builds
// on: the ordered candidate list (every tracked collection plus the
// always-offered fleet-home fallback) and the explicit-target adopt flow
// that moves into any collection dir — tracked or unscanned — without
// touching config.
package customs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/scan"
	"github.com/zzacong/fleet/internal/trackedset"
)

// AdoptCandidates returns the ordered adopt destination candidates
// (collection dirs): each tracked repo root's skills/ subdir in tracked
// order, then the unversioned fleet-home fallback. The fallback is always
// present, so zero tracked collections yields exactly one candidate and no
// prompt is needed. Entries are deduped by cleaned path.
func AdoptCandidates(p *paths.Paths) ([]string, error) {
	collections, err := trackedset.CollectionDirs(p)
	if err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	add := func(dir string) {
		clean := filepath.Clean(dir)
		if seen[clean] {
			return
		}
		seen[clean] = true
		out = append(out, clean)
	}
	for _, collection := range collections {
		add(collection)
	}
	add(p.FleetHomeSkills())
	return out, nil
}

// AdoptTo migrates one skill into the explicit target collection dir,
// creating it on demand. A canonical-store hit moves atomically into the
// target; a hit in exactly one custom home moves nothing but re-ensures the
// wiring and links for the skill's actual home. A name present in more than
// one source is a double-presence error to resolve by hand. An unscanned
// target still proceeds (doctor warns elsewhere). No config file is written.
func AdoptTo(p *paths.Paths, name, target string) (*Report, error) {
	homes, err := AdoptCandidates(p)
	if err != nil {
		return nil, err
	}
	return adoptInto(p, name, filepath.Clean(target), homes)
}

// adoptInto is the shared adopt flow parameterized by destination target
// and the custom homes to scan for double presence.
func adoptInto(p *paths.Paths, name, target string, homes []string) (*Report, error) {
	storeSkills, err := scan.ScanStore(p.SkillsStore())
	if err != nil {
		return nil, fmt.Errorf("scan canonical store: %w", err)
	}
	type hit struct {
		skill *scan.Skill
		home  string
	}
	storeHit := findSkill(storeSkills, name)
	var customHits []hit
	for _, home := range homes {
		skills, err := scan.ScanStore(home)
		if err != nil {
			return nil, fmt.Errorf("scan custom skills: %w", err)
		}
		if s := findSkill(skills, name); s != nil {
			customHits = append(customHits, hit{skill: s, home: home})
		}
	}

	switch {
	case storeHit != nil && len(customHits) > 0:
		return nil, fmt.Errorf("skill %q exists in both %s and %s — resolve by hand before adopting", name, p.SkillsStore(), customHits[0].home)
	case len(customHits) > 1:
		return nil, fmt.Errorf("skill %q exists in both %s and %s — resolve by hand before adopting", name, customHits[0].home, customHits[1].home)
	case storeHit == nil && len(customHits) == 0:
		return nil, fmt.Errorf("skill %q not found in %s", name, p.SkillsStore())
	}

	rep := &Report{}
	var wireTarget string
	switch {
	case storeHit != nil:
		rep.Skill = storeHit.Dir
		rep.From = filepath.Join(p.SkillsStore(), storeHit.Dir)
		rep.To = filepath.Join(target, storeHit.Dir)
		wireTarget = target
		rep.Moved = true
		if err := os.MkdirAll(wireTarget, 0o755); err != nil {
			return nil, err
		}
		if err := moveDir(rep.From, rep.To); err != nil {
			return nil, fmt.Errorf("move %s into %s: %w", storeHit.Dir, target, err)
		}
	default:
		rep.Skill = customHits[0].skill.Dir
		actualHome := customHits[0].home
		rep.To = filepath.Join(actualHome, customHits[0].skill.Dir)
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
