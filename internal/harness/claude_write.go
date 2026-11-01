// The claude code write side: skillOverrides entries in
// ~/.claude/settings.json (strict JSON). Disabling writes
// "<name>: "off""; enabling removes fleet's entry, dropping the whole
// skillOverrides object when nothing is left in it. RemoveOff drops fleet's
// entry for a skill that has left state, which Project never clears. Link
// presence decides whether claude can even see a skill; links are never
// written here — an absent skill has nothing to disable, so it is left alone.

package harness

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zzacong/fleet/internal/jsonc"
)

// CanProject implements Adapter: skillOverrides is a config lever.
func (a *ClaudeAdapter) CanProject() bool { return true }

// claudeOverrides is one parsed settings.json: the source text, its document
// (for rendering an edit back), the root object, and the skillOverrides
// object, which is nil when the key is absent. Project and RemoveOff share
// this loader so both reject the same malformed shapes.
type claudeOverrides struct {
	src  string
	doc  *jsonc.Document
	root *jsonc.Object
	obj  *jsonc.Object
}

// loadClaudeOverrides reads and parses settings.json, requiring a JSON
// object at the root and one under skillOverrides when that key is present.
func loadClaudeOverrides(path string) (*claudeOverrides, error) {
	src, err := readConfig(path)
	if err != nil {
		return nil, err
	}
	doc, err := parseStrictJSONConfig(src)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	root := doc.RootObject()
	if root == nil {
		return nil, fmt.Errorf("%s must contain a JSON object", filepath.Base(path))
	}
	obj := root.Obj("skillOverrides")
	if root.Has("skillOverrides") && obj == nil {
		return nil, fmt.Errorf("%s: \"skillOverrides\" must be an object", filepath.Base(path))
	}
	return &claudeOverrides{src: src, doc: doc, root: root, obj: obj}, nil
}

// CheckOverrideConfig parses the Claude settings without changing them, so
// archive can refuse a malformed file before it moves any skill.
func (a *ClaudeAdapter) CheckOverrideConfig() error {
	_, err := loadClaudeOverrides(a.home.ClaudeSettings())
	return err
}

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

	cfg, err := loadClaudeOverrides(a.home.ClaudeSettings())
	if err != nil {
		return WriteReport{}, err
	}
	src, root, overrides := cfg.src, cfg.root, cfg.obj

	// Names to mark "off" when the skillOverrides object has to be created
	// fresh.
	var fresh []string

	for _, w := range writes {
		if w.Dormant {
			continue // installed nowhere: leave any existing override, write none
		}
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

	if !allPreview(writes) {
		if err := writeIfConfigChanged(a.home.ClaudeSettings(), src, renderConfig(src, cfg.doc)); err != nil {
			return WriteReport{}, err
		}
	}
	return rep, nil
}

// RemoveOff deletes name's skillOverrides entry when its value is fleet's
// "off" marker, dropping the skillOverrides object once nothing is left in
// it. Any other value, and any other entry, stays. Project only writes
// "off" for names still in state, so a skill being archived needs this.
// Call CheckOverrideConfig first when a move precedes this write: a
// malformed file then fails before anything moves.
func (a *ClaudeAdapter) RemoveOff(name string) error {
	cfg, err := loadClaudeOverrides(a.home.ClaudeSettings())
	if err != nil {
		return err
	}
	if cfg.obj == nil {
		return nil
	}
	if value, ok := cfg.obj.String(name); !ok || value != "off" {
		return nil
	}

	cfg.obj.Delete(name)
	if len(cfg.obj.Keys()) == 0 {
		cfg.root.Delete("skillOverrides")
	}
	return writeIfConfigChanged(a.home.ClaudeSettings(), cfg.src, renderConfig(cfg.src, cfg.doc))
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
