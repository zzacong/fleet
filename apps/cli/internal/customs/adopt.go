// Package customs owns the custom-skills homes: the designated skills repo's
// skills/ when a skills repo is set, otherwise the fleet-home fallback
// ~/.config/fleet/skills. Adopt moves a skill from the canonical store into
// the resolved home, wires that home into the harnesses that take extra
// discovery paths (opencode, pi), and manages every link-based harness's
// symlink to it (codex, claude code, Cursor, Bob).
//
// Adoption records nothing but the location itself: custom means lives in
// the resolved home, so the state file is untouched. Disables recorded
// before adoption keep applying (their config rules target the skill's
// name, wherever it lives), and moving the directory back by hand undoes
// the adoption cleanly.
package customs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/scan"
)

// Report is what one adopt run did. Empty slices mean nothing to do.
type Report struct {
	// Skill is the skill's directory name — the discovery name every
	// harness and link uses.
	Skill string
	// From and To are the directory's old and new locations. From is empty
	// when nothing moved (the skill was already adopted: Moved is false
	// and the wiring and links were still ensured).
	From, To string
	// Moved reports whether the directory actually moved this run.
	Moved bool
	// Wired lists the config-path harnesses the resolved home was wired into
	// this run.
	Wired []harness.WireResult
	// Linked lists the managed links created or repointed this run.
	Linked []harness.LinkResult
}

// Adopt migrates one custom skill into the resolved custom home. The skill
// is looked up by directory or frontmatter name across the canonical store
// and both custom homes. A name present in more than one source (canonical vs
// either custom, or both customs) is a double-presence error to resolve by
// hand. On a canonical-store hit the directory is moved atomically into the
// resolved target; on a custom-home hit nothing moves but the wiring and
// links for the resolved home are re-ensured so a partially failed earlier
// run heals (Report.Moved is false in that case). No state file is written.
func Adopt(p *paths.Paths, name string) (*Report, error) {
	target := p.RepoSkills()
	if target == "" {
		target = p.FleetHomeSkills()
	}

	storeSkills, err := scan.ScanStore(p.SkillsStore())
	if err != nil {
		return nil, fmt.Errorf("scan canonical store: %w", err)
	}
	fleetSkills, err := scan.ScanStore(p.FleetHomeSkills())
	if err != nil {
		return nil, fmt.Errorf("scan fleet-home skills: %w", err)
	}
	var repoSkills []scan.Skill
	if p.RepoSkills() != "" {
		rs, err := scan.ScanStore(p.RepoSkills())
		if err != nil {
			return nil, fmt.Errorf("scan repo skills: %w", err)
		}
		repoSkills = rs
	}

	storeHit := findSkill(storeSkills, name)
	fleetHit := findSkill(fleetSkills, name)
	var repoHit *scan.Skill
	if p.RepoSkills() != "" {
		repoHit = findSkill(repoSkills, name)
	}

	switch {
	case storeHit != nil && fleetHit != nil:
		return nil, fmt.Errorf("skill %q exists in both %s and %s — resolve by hand before adopting", name, p.SkillsStore(), p.FleetHomeSkills())
	case storeHit != nil && repoHit != nil:
		return nil, fmt.Errorf("skill %q exists in both %s and %s — resolve by hand before adopting", name, p.SkillsStore(), p.RepoSkills())
	case fleetHit != nil && repoHit != nil:
		return nil, fmt.Errorf("skill %q exists in both %s and %s — resolve by hand before adopting", name, p.FleetHomeSkills(), p.RepoSkills())
	case storeHit == nil && fleetHit == nil && repoHit == nil:
		return nil, fmt.Errorf("skill %q not found in %s", name, p.SkillsStore())
	}

	rep := &Report{}
	var wireTarget string
	switch {
	case storeHit != nil:
		// Move first: the wired paths and links must point at a directory
		// that exists.
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
	case fleetHit != nil:
		rep.Skill = fleetHit.Dir
		actualHome := p.FleetHomeSkills()
		rep.To = filepath.Join(actualHome, fleetHit.Dir)
		wireTarget = actualHome
		if err := os.MkdirAll(wireTarget, 0o755); err != nil {
			return nil, err
		}
	case repoHit != nil:
		rep.Skill = repoHit.Dir
		actualHome := p.RepoSkills()
		rep.To = filepath.Join(actualHome, repoHit.Dir)
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

// findSkill matches by directory name first, then by frontmatter name.
func findSkill(skills []scan.Skill, name string) *scan.Skill {
	for i := range skills {
		if skills[i].Dir == name {
			return &skills[i]
		}
	}
	for i := range skills {
		if skills[i].Name == name {
			return &skills[i]
		}
	}
	return nil
}
