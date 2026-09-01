// Package customs owns the fleet repo's skills/ directory: the home of
// custom skills. Adopt moves a skill from the canonical
// store into the repo, wires the repo path into the harnesses that take
// extra discovery paths (opencode, pi), and manages every link-based
// harness's symlink to it (codex, claude code, Cursor, Bob).
//
// Adoption records nothing but the location itself: custom means lives in
// the repo, so the state file is untouched. Disables recorded before
// adoption keep applying (their config rules target the skill's name,
// wherever it lives), and moving the directory back by hand undoes the
// adoption cleanly.
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
	// Wired lists the config-path harnesses the repo dir was wired into
	// this run.
	Wired []harness.WireResult
	// Linked lists the managed links created or repointed this run.
	Linked []harness.LinkResult
}

// Adopt migrates one custom skill into the repo's skills/ directory. The
// skill is looked up by directory or frontmatter name. Adopting an
// already-adopted skill is not an error: nothing moves, but the wiring and
// links are re-ensured, so a partially failed earlier run heals on the
// next adopt (Report.Moved is false in that case).
func Adopt(p *paths.Paths, name string) (*Report, error) {
	if p.RepoSkills() == "" {
		return nil, fmt.Errorf("no fleet repo found — run inside the repo or set FLEET_REPO")
	}

	storeSkills, err := scan.ScanStore(p.SkillsStore())
	if err != nil {
		return nil, fmt.Errorf("scan canonical store: %w", err)
	}
	repoSkills, err := scan.ScanStore(p.RepoSkills())
	if err != nil {
		return nil, fmt.Errorf("scan repo skills: %w", err)
	}

	storeHit := findSkill(storeSkills, name)
	repoHit := findSkill(repoSkills, name)
	switch {
	case storeHit != nil && repoHit != nil:
		return nil, fmt.Errorf("skill %q exists in both %s and %s — resolve by hand before adopting", name, p.SkillsStore(), p.RepoSkills())
	case storeHit == nil && repoHit == nil:
		return nil, fmt.Errorf("skill %q not found in %s", name, p.SkillsStore())
	}

	rep := &Report{}
	switch {
	case storeHit != nil:
		// Move first: the wired paths and links must point at a directory
		// that exists.
		rep.Skill = storeHit.Dir
		rep.From = filepath.Join(p.SkillsStore(), storeHit.Dir)
		rep.To = filepath.Join(p.RepoSkills(), storeHit.Dir)
		rep.Moved = true
		if err := os.MkdirAll(p.RepoSkills(), 0o755); err != nil {
			return nil, err
		}
		if err := moveDir(rep.From, rep.To); err != nil {
			return nil, fmt.Errorf("move %s into the repo: %w", storeHit.Dir, err)
		}
	case repoHit != nil:
		rep.Skill = repoHit.Dir
		rep.To = filepath.Join(p.RepoSkills(), repoHit.Dir)
	}

	wired, err := harness.WireSkillSource(p, p.RepoSkills())
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
