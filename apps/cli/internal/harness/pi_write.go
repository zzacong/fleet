// The pi write side: force-exclude entries in the `skills` array of
// ~/.pi/agent/settings.json, in the exact form `pi config` produces. The
// file is strict JSON: the read-modify-write parses strictly, preserves
// unknown keys and formatting through the JSONC editor, and never emits
// comments.

package harness

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zzacong/fleet/internal/jsonc"
)

// piExactEntry is the exclusion form fleet writes, matching `pi config`:
// a path relative to ~/.agents.
func piExactEntry(name string) string {
	return "-skills/" + name + "/SKILL.md"
}

// CanProject implements Adapter: pi's settings file is a config lever.
func (a *PiAdapter) CanProject() bool { return true }

// Project implements Adapter.
func (a *PiAdapter) Project(writes []SkillWrite) (WriteReport, error) {
	rep := WriteReport{}
	if err := validateNames(writes); err != nil {
		return rep, err
	}

	read, err := a.Read(writeNames(writes))
	if err != nil {
		return WriteReport{}, err
	}

	src, err := readConfig(a.home.PiSettings())
	if err != nil {
		return WriteReport{}, err
	}
	doc, err := parseStrictJSONConfig(src)
	if err != nil {
		return WriteReport{}, fmt.Errorf("parse %s: %w", filepath.Base(a.home.PiSettings()), err)
	}
	root := doc.RootObject()
	if root == nil {
		return WriteReport{}, fmt.Errorf("%s must contain a JSON object", filepath.Base(a.home.PiSettings()))
	}

	skills := root.Array("skills")
	if root.Has("skills") && skills == nil {
		return WriteReport{}, fmt.Errorf("%s: \"skills\" must be an array", filepath.Base(a.home.PiSettings()))
	}

	// Entries to write when the skills array has to be created fresh.
	var fresh []string

	for _, w := range writes {
		before := read.States[w.Name]
		switch w.State {
		case StateOff:
			if before == StateOff {
				// Already excluded. Quiet when fleet's own exact entry is
				// what does it; a !glob exclusion isn't fleet's to touch.
				if skills == nil || !piHasExactEntry(skills, w.Name) {
					rep.Flags = append(rep.Flags, Flag{Skill: w.Name, Message: "still excluded by a !glob entry — left alone"})
				}
				continue
			}
			if skills == nil {
				fresh = append(fresh, w.Name)
				continue
			}
			skills.Append(jsonc.Quote(piExactEntry(w.Name)))
			rep.Changed = append(rep.Changed, Change{Skill: w.Name, From: before, To: StateOff})
		case StateOn:
			if before != StateOff || skills == nil {
				continue
			}
			removed := piRemoveExactEntries(skills, w.Name)
			if piGlobExcludes(skills, w.Name) {
				rep.Flags = append(rep.Flags, Flag{Skill: w.Name, Message: "still excluded by a !glob entry — left alone"})
				continue
			}
			if removed {
				rep.Changed = append(rep.Changed, Change{Skill: w.Name, From: StateOff, To: StateOn})
			}
		}
	}

	if skills == nil && len(fresh) > 0 {
		var b strings.Builder
		b.WriteString("[")
		for i, name := range fresh {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(jsonc.Quote(piExactEntry(name)))
		}
		b.WriteString("]")
		root.Set("skills", b.String())
		for _, name := range fresh {
			rep.Changed = append(rep.Changed, Change{Skill: name, From: read.States[name], To: StateOff})
		}
	}

	flagUnmanagedPi(skills, writes, &rep)

	if err := writeIfConfigChanged(a.home.PiSettings(), src, renderConfig(src, doc)); err != nil {
		return WriteReport{}, err
	}
	return rep, nil
}

// flagUnmanagedPi reports exclusion entries no write covers: exact
// exclusions for skills fleet isn't tracking and !glob entries fleet can
// never own. Written skills are handled by their own write, and globs
// whose effect on a written skill that write already reported are skipped.
func flagUnmanagedPi(skills *jsonc.Array, writes []SkillWrite, rep *WriteReport) {
	if skills == nil {
		return
	}
	written := writeNameSet(writes)
	for i := 0; i < skills.Len(); i++ {
		s, ok := skills.StringItem(i)
		if !ok {
			continue
		}
		if len(s) > 1 && s[0] == '!' {
			if patternCoversWrite(s[1:], writes) {
				continue // the write for that skill already reported it
			}
			flagForeign(rep, fmt.Sprintf("skills entry %q is not fleet's", s))
			continue
		}
		if m := piExactExclusion.FindStringSubmatch(s); m != nil && !written[m[1]] {
			flagNotTracked(rep, m[1])
		}
	}
}

// piHasExactEntry reports whether the array holds fleet's exact exclusion
// form for name.
func piHasExactEntry(skills *jsonc.Array, name string) bool {
	for i := 0; i < skills.Len(); i++ {
		if s, ok := skills.StringItem(i); ok && s == piExactEntry(name) {
			return true
		}
	}
	return false
}

// piRemoveExactEntries deletes every exact exclusion for name; it reports
// whether any were found.
func piRemoveExactEntries(skills *jsonc.Array, name string) bool {
	removed := false
	for i := skills.Len() - 1; i >= 0; i-- {
		if s, ok := skills.StringItem(i); ok && s == piExactEntry(name) {
			skills.Delete(i)
			removed = true
		}
	}
	return removed
}

// piGlobExcludes reports whether any !glob entry still excludes name — a
// form fleet doesn't write, so it stays.
func piGlobExcludes(skills *jsonc.Array, name string) bool {
	for i := 0; i < skills.Len(); i++ {
		s, ok := skills.StringItem(i)
		if !ok || len(s) < 2 || s[0] != '!' {
			continue
		}
		if matchPattern(s[1:], name) {
			return true
		}
	}
	return false
}
