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

// ArchiveResult is what one archive run did. To is the skill's new location;
// Unlinked lists the managed links removed this run.
type ArchiveResult struct {
	To       string
	Unlinked []harness.UnlinkResult
}

// CustomCopies returns every custom-home copy that name matches by directory
// or frontmatter name, precedence-first, so the first copy is the one the
// snapshot shows for that name. The canonical store never counts as a custom
// copy, so an installed skill yields none. It fails when any home cannot be
// scanned, because a missing copy could then be hiding among the unreadable
// ones.
func CustomCopies(p *paths.Paths, name string) ([]skillindex.Hit, error) {
	idx, err := loadScanned(p)
	if err != nil {
		return nil, err
	}
	var out []skillindex.Hit
	for _, home := range idx.CustomHomes() {
		for _, s := range idx.Skills(home) {
			if s.Dir == name || s.Name == name {
				out = append(out, skillindex.Hit{Skill: s, Home: home})
			}
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
	return fmt.Errorf("archive: %q shadows canonical skill %q at %s; archiving it would expose the installed copy in every harness", name, canon.Skill.Dir, canon.Path())
}

// InstalledError is the refusal for archiving a skill the skills CLI owns:
// it names the canonical skill so the user knows which one to remove.
func InstalledError(name string, canon skillindex.Hit) error {
	return fmt.Errorf("archive: %q is installed as canonical skill %q at %s; archive moves only custom skills, so remove it with the skills CLI", name, canon.Skill.Dir, canon.Path())
}

// LiveCopy returns the custom copy a row for name displays: the highest-
// precedence custom copy matched by directory or frontmatter name. false
// when no custom copy matches. The TUI uses this so A archives the copy on
// screen; the CLI resolves through CustomCopies instead so it can refuse an
// ambiguous name.
func LiveCopy(p *paths.Paths, name string) (skillindex.Hit, bool, error) {
	copies, err := CustomCopies(p, name)
	if err != nil {
		return skillindex.Hit{}, false, err
	}
	if len(copies) == 0 {
		return skillindex.Hit{}, false, nil
	}
	return copies[0], true, nil
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
// archived/), unlinks its managed link from every installed harness, drops
// its state entries, and clears Claude's "off" skillOverrides entry for it.
// It refuses a copy whose name shadows a canonical skill or whose target
// already exists in archived/, before anything changes. Every step that can
// fail on existing data — the state load and the Claude settings parse —
// runs before the move, and a failed move removes any archived/ folder this
// run created, so a refusal or a failed move leaves links and state as they
// were. Sync is the caller's to run. The archive is one-way.
func Archive(p *paths.Paths, hit skillindex.Hit) (*ArchiveResult, error) {
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

	claude := harness.NewClaude(p)
	if err := claude.CheckOverrideConfig(); err != nil {
		return nil, err
	}

	archivedDir := filepath.Join(filepath.Dir(hit.Home), "archived")
	to := filepath.Join(archivedDir, name)
	if _, err := os.Lstat(to); err == nil {
		return nil, fmt.Errorf("archive: %q already exists at %s; rename or remove the existing archived folder first", name, to)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	_, statErr := os.Stat(archivedDir)
	createdArchived := os.IsNotExist(statErr)

	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		return nil, err
	}
	if err := moveDir(hit.Path(), to); err != nil {
		if createdArchived {
			_ = os.Remove(archivedDir) // empty unless this run's MkdirAll was the only one
		}
		return nil, fmt.Errorf("archive %s: %w", name, err)
	}

	unlinked, err := unlinkSkill(p, name, hit.Home)
	if err != nil {
		return nil, err
	}
	if err := claude.RemoveOff(name); err != nil {
		return nil, err
	}

	st.RemoveSkill(name)
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		return nil, err
	}
	return &ArchiveResult{To: to, Unlinked: unlinked}, nil
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
