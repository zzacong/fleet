// The claude code write side: skillOverrides entries in
// ~/.claude/settings.json (strict JSON). Disabling writes
// "<name>: "off""; enabling removes fleet's entry, dropping the whole
// skillOverrides object when nothing is left in it. Link presence decides
// whether claude can even see a skill; links are never written here —
// an absent skill has nothing to disable, so it is left alone.

package harness

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zzacong/fleet/internal/jsonc"
)

// CanProject implements Adapter: skillOverrides is a config lever.
func (a *ClaudeAdapter) CanProject() bool { return true }

// Project implements Adapter.
func (a *ClaudeAdapter) Project(writes []SkillWrite) (WriteReport, error) {
	rep := WriteReport{}
	if err := validateNames(writes); err != nil {
		return rep, err
	}

	read, err := a.Read(writeNames(writes))
	if err != nil {
		return WriteReport{}, err
	}

	src, err := readConfig(a.home.ClaudeSettings())
	if err != nil {
		return WriteReport{}, err
	}
	doc, err := parseStrictJSONConfig(src)
	if err != nil {
		return WriteReport{}, fmt.Errorf("parse %s: %w", filepath.Base(a.home.ClaudeSettings()), err)
	}
	root := doc.RootObject()
	if root == nil {
		return WriteReport{}, fmt.Errorf("%s must contain a JSON object", filepath.Base(a.home.ClaudeSettings()))
	}

	overrides := root.Obj("skillOverrides")
	if root.Has("skillOverrides") && overrides == nil {
		return WriteReport{}, fmt.Errorf("%s: \"skillOverrides\" must be an object", filepath.Base(a.home.ClaudeSettings()))
	}

	// Names to mark "off" when the skillOverrides object has to be created
	// fresh.
	var fresh []string

	for _, w := range writes {
		before := read.States[w.Name]
		switch w.State {
		case StateOff:
			switch before {
			case StateOff:
				// Already off via an exact "off" entry — fleet's own shape
				// or one that reads identically. Quiet.
			case StateAbsent:
				// Without a link claude cannot discover the skill, so
				// there is nothing to disable. Doctor explains missing
				// links in a later ticket.
			default:
				if overrides == nil {
					fresh = append(fresh, w.Name)
					continue
				}
				overrides.Set(w.Name, `"off"`)
				rep.Changed = append(rep.Changed, Change{Skill: w.Name, From: before, To: StateOff})
			}
		case StateOn:
			// The entry here is fleet's marker, an identical manual one,
			// or a stale "off" for a skill claude can no longer discover
			// (no link). Removing it is what the state's "on" asks for in
			// every case: nothing else in the settings disables the skill.
			// Other values (claude's own, e.g. "user-invocable-only") are
			// not disables and stay.
			if overrides == nil {
				continue
			}
			var value string
			var ok bool
			for _, kv := range overrides.KVs() {
				if kv.Key == w.Name {
					value, ok = jsonString(kv.RawValue)
					break
				}
			}
			stale := ok && value == "off" &&
				(before == StateOff || before == StateAbsent)
			if !stale {
				continue
			}
			overrides.Delete(w.Name)
			if len(overrides.Keys()) == 0 {
				root.Delete("skillOverrides")
			}
			rep.Changed = append(rep.Changed, Change{Skill: w.Name, From: before, To: StateOn})
		}
	}

	if overrides == nil && len(fresh) > 0 {
		var b strings.Builder
		b.WriteString("{")
		for i, name := range fresh {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s: \"off\"", jsonc.Quote(name))
		}
		b.WriteString("}")
		root.Set("skillOverrides", b.String())
		for _, name := range fresh {
			rep.Changed = append(rep.Changed, Change{Skill: name, From: read.States[name], To: StateOff})
		}
	}

	flagUnmanagedClaude(overrides, writes, &rep)

	if err := writeIfConfigChanged(a.home.ClaudeSettings(), src, renderConfig(src, doc)); err != nil {
		return WriteReport{}, err
	}
	return rep, nil
}

// flagUnmanagedClaude reports "off" overrides for skills no write covers.
// Other values (claude's own, e.g. "user-invocable-only") don't disable a
// skill and pass silently.
func flagUnmanagedClaude(overrides *jsonc.Object, writes []SkillWrite, rep *WriteReport) {
	if overrides == nil {
		return
	}
	written := writeNameSet(writes)
	for _, kv := range overrides.KVs() {
		if value, ok := jsonString(kv.RawValue); !ok || value != "off" || written[kv.Key] {
			continue
		}
		flagNotTracked(rep, kv.Key)
	}
}
