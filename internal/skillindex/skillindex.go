// Package skillindex owns the skill index: the precedence-ordered union
// of every skill source — the canonical store, every tracked collection
// in tracked-set order, then the fleet-home fallback. It is the sole
// scanner of skill homes: snapshot, doctor, adopt, and update read
// through it instead of scanning homes themselves. Each reader keeps its
// own collision policy (display picks the precedence winner, adopt
// errors, doctor reports); the index only reports every copy in
// precedence order, keyed by skill directory with lookup by frontmatter
// name.
package skillindex

import (
	"path/filepath"

	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/scan"
	"github.com/zzacong/fleet/internal/trackedset"
)

// Hit is one copy of a skill: the scanned skill plus the collection dir
// holding it.
type Hit struct {
	Skill scan.Skill
	Home  string
}

// Index is the scanned union of every skill source. Homes are
// precedence-first: tracked collections in tracked-set order, then the
// fleet-home fallback, then the canonical store.
type Index struct {
	homes    []string
	customs  []string
	store    string
	fallback string
	byHome   map[string][]scan.Skill
	byDir    map[string][]Hit
}

// CustomHomes returns the adopt-destination candidates without scanning:
// every tracked collection in tracked-set order, then the always-offered
// fleet-home fallback. Zero tracked collections yields exactly the
// fallback, so no prompt is needed. Entries are deduped by cleaned path.
func CustomHomes(p *paths.Paths) ([]string, error) {
	collections, err := trackedset.CollectionDirs(p)
	if err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	add := func(dir string) {
		clean := filepath.Clean(dir)
		if clean == "" || seen[clean] {
			return
		}
		seen[clean] = true
		out = append(out, clean)
	}
	for _, c := range collections {
		add(c)
	}
	add(p.FleetHomeSkills())
	return out, nil
}

// Load scans every skill source once. The tracked set itself unreadable
// is a hard error; per-home scan failures come back in the error map so
// each caller applies its own rule (snapshot and adopt fail, doctor
// skips). A missing home scans empty, never an error.
func Load(p *paths.Paths) (*Index, map[string]error, error) {
	customs, err := CustomHomes(p)
	if err != nil {
		return nil, nil, err
	}
	x := &Index{
		store:    filepath.Clean(p.SkillsStore()),
		fallback: filepath.Clean(p.FleetHomeSkills()),
		byHome:   map[string][]scan.Skill{},
		byDir:    map[string][]Hit{},
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
		}
	}
	return x, errs, nil
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
