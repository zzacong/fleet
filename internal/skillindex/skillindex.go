// Package skillindex owns the skill index: the precedence-ordered union
// of every skill source — every explicit `skillsDirs` collection (scanned
// directly, no `skills/` derivation), then the fleet-home fallback, then
// the canonical store. It is the sole scanner of skill homes: snapshot,
// doctor, adopt, and update read through it instead of scanning homes
// themselves. Each reader keeps its own collision policy (display picks
// the precedence winner, adopt errors, doctor reports); the index only
// reports every copy in precedence order, keyed by skill directory with
// lookup by frontmatter name. It also answers the shared "is this skill
// installed" question by directory or frontmatter name, and carries the
// completeness signal that says whether a negative answer can be trusted.
package skillindex

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/scan"
)

// Hit is one copy of a skill: the scanned skill plus the collection dir
// holding it.
type Hit struct {
	Skill scan.Skill
	Home  string
}

// Index is the scanned union of every skill source. Homes are
// precedence-first: every explicit skillsDirs collection, then the
// fleet-home fallback, then the canonical store.
type Index struct {
	homes     []string
	customs   []string
	store     string
	fallback  string
	byHome    map[string][]scan.Skill
	byDir     map[string][]Hit
	installed map[string]bool
	complete  bool
	blocked   []string
}

// CustomHomes returns the adopt-destination candidates without scanning:
// every explicit collection dir in precedence order, then the
// always-offered fleet-home fallback. Zero tracked collections yields
// exactly the fallback, so no prompt is needed. Entries are deduped by
// cleaned path.
func CustomHomes(p *paths.Paths) ([]string, error) {
	homes, err := customHomes(p)
	return homes, err
}

// customHomes resolves the custom collection dirs: every explicit
// skillsDirs entry in order, then the fleet-home fallback. Entries are
// deduped by cleaned collection path.
func customHomes(p *paths.Paths) ([]string, error) {
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var homes []string
	add := func(dir string) {
		clean := filepath.Clean(dir)
		if clean == "" || seen[clean] {
			return
		}
		seen[clean] = true
		homes = append(homes, clean)
	}
	// Explicit collection dirs are scanned directly: the tracked path is
	// the collection itself, with no `skills/` derivation. They are the
	// highest-precedence custom source.
	for _, dir := range f.SkillsDirs() {
		add(dir)
	}
	add(p.FleetHomeSkills())
	return homes, nil
}

// Load scans every skill source once. The tracked set itself unreadable
// is a hard error; per-home scan failures come back in the error map so
// each caller applies its own rule (snapshot and adopt fail, doctor
// skips). A missing home scans empty, never an error. The result also
// carries the completeness signal: see Complete.
func Load(p *paths.Paths) (*Index, map[string]error, error) {
	customs, err := customHomes(p)
	if err != nil {
		return nil, nil, err
	}
	x := &Index{
		store:     filepath.Clean(p.SkillsStore()),
		fallback:  filepath.Clean(p.FleetHomeSkills()),
		byHome:    map[string][]scan.Skill{},
		byDir:     map[string][]Hit{},
		installed: map[string]bool{},
	}
	seen := map[string]bool{}
	add := func(dir string) (string, bool) {
		clean := filepath.Clean(dir)
		if clean == "" || seen[clean] {
			return clean, false
		}
		seen[clean] = true
		x.homes = append(x.homes, clean)
		return clean, true
	}
	for _, c := range customs {
		if _, ok := add(c); ok {
			x.customs = append(x.customs, filepath.Clean(c))
		}
	}
	add(x.store)

	errs := map[string]error{}
	for _, home := range x.homes {
		skills, serr := scan.ScanStore(home)
		if serr != nil {
			errs[home] = serr
			skills = nil
		}
		x.byHome[home] = skills
		for _, s := range skills {
			x.byDir[s.Dir] = append(x.byDir[s.Dir], Hit{Skill: s, Home: home})
			x.installed[s.Dir] = true
			x.installed[s.Name] = true
		}
	}
	// Complete requires the store to exist and scan and no home to fail
	// scanning. The fleet-home fallback and every explicit skillsDirs
	// entry are optional: a listed dir missing from disk scans empty
	// rather than blocking, and doctor reports it separately.
	x.complete = dirExists(x.store) && len(errs) == 0
	x.blocked = blockedHomes(x.homes, errs)
	return x, errs, nil
}

// dirExists reports whether path is an existing directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// blockedHomes names the homes that made a scan incomplete: any home that
// failed to scan. The canonical store's absence is deliberately not listed
// here — doctor reports that as a missing directory; a store that exists
// but fails to scan is a scan error and does appear. Sorted and deduped.
func blockedHomes(homes []string, errs map[string]error) []string {
	seen := map[string]bool{}
	var out []string
	add := func(path string) {
		clean := filepath.Clean(path)
		if clean == "" || seen[clean] {
			return
		}
		seen[clean] = true
		out = append(out, clean)
	}
	for _, home := range homes {
		if _, ok := errs[home]; ok {
			add(home)
		}
	}
	sort.Strings(out)
	return out
}

// Homes returns every source scanned, precedence-first.
func (x *Index) Homes() []string {
	return append([]string(nil), x.homes...)
}

// CustomHomes returns the versioned customs plus the fallback — every
// tracked collection in order, then the fleet-home fallback. The
// canonical store is not included.
func (x *Index) CustomHomes() []string {
	return append([]string(nil), x.customs...)
}

// Store returns the canonical store dir.
func (x *Index) Store() string {
	return x.store
}

// Fallback returns the fleet-home fallback dir.
func (x *Index) Fallback() string {
	return x.fallback
}

// Skills returns the skills scanned in one home, in store order. An
// unscanned home yields nil.
func (x *Index) Skills(home string) []scan.Skill {
	return x.byHome[filepath.Clean(home)]
}

// Ordered returns every copy of a skill directory, precedence-first.
// Empty when the directory lives in no scanned home.
func (x *Index) Ordered(dir string) []Hit {
	return append([]Hit(nil), x.byDir[dir]...)
}

// Lookup finds copies by directory name first, then by frontmatter
// name — the same match order adopt always used. Every hit carries its
// home so callers partition store from customs themselves.
func (x *Index) Lookup(name string) []Hit {
	var out []Hit
	seen := map[string]bool{}
	for _, h := range x.byDir[name] {
		out = append(out, h)
		seen[h.Skill.Dir] = true
	}
	for _, home := range x.homes {
		for _, s := range x.byHome[home] {
			if s.Name == name && !seen[s.Dir] {
				seen[s.Dir] = true
				out = append(out, Hit{Skill: s, Home: home})
			}
		}
	}
	return out
}

// IsCustom reports whether any custom home holds a skill with the given
// name (by directory or frontmatter name). The canonical store is not a
// custom home: a store-only hit is installed, not custom.
func (x *Index) IsCustom(name string) bool {
	for _, h := range x.Lookup(name) {
		if h.Home != x.store {
			return true
		}
	}
	return false
}

// Complete reports whether the "installed" answer can be trusted: the
// canonical store exists and scans and no home failed to scan. A caller
// may assert a skill is uninstalled only when Complete is true.
func (x *Index) Complete() bool {
	return x.complete
}

// BlockedHomes returns the homes that made the scan incomplete: any home
// that failed to scan. Sorted and deduped. Empty when Complete is true, or
// when the only reason is the canonical store's absence, which callers
// report separately.
func (x *Index) BlockedHomes() []string {
	return append([]string(nil), x.blocked...)
}

// IsInstalled reports whether a skill lives in the canonical store or any
// custom home, matched by directory name or frontmatter name.
func (x *Index) IsInstalled(name string) bool {
	return x.installed[name]
}

// InstalledNames returns the sorted, deduped set of names the index can
// match as installed: every scanned skill's directory name and frontmatter
// name across the canonical store and every custom home.
func (x *Index) InstalledNames() []string {
	out := make([]string, 0, len(x.installed))
	for name := range x.installed {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
