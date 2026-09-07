// Package state owns fleet's state file (~/.config/fleet/state.json): the
// versioned single source of truth for per-harness skill enablement that
// sync projects into each harness's native config.
//
// The schema is sparse: a skill entry lists only the harnesses it is
// disabled for; an absent entry means on everywhere. Unknown fields — top
// level, per skill, and per harness value — are preserved verbatim on
// round-trips so newer or foreign writers lose nothing.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Version is the state file schema version fleet reads and writes.
const Version = 1

// File is the parsed state file.
type File struct {
	version int
	skills  map[string]*skill
	unknown map[string]json.RawMessage // top-level fields fleet doesn't know
}

// skill is one skill's per-harness enablement plus unknown fields.
type skill struct {
	harnesses map[string]json.RawMessage // value is "off" or a future form
	unknown   map[string]json.RawMessage
}

// Load reads the state file. A missing file is an empty state, not an
// error. A file without a version, with an unsupported version, or with
// unusable shapes is an error — fleet never guesses at foreign content.
func Load(path string) (*File, error) {
	f := &File{
		version: Version,
		skills:  map[string]*skill{},
		unknown: map[string]json.RawMessage{},
	}

	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}

	ver, ok := raw["version"]
	if !ok {
		return nil, fmt.Errorf("%s is missing a \"version\" field; not a fleet state file", path)
	}
	var version int
	if err := json.Unmarshal(ver, &version); err != nil {
		return nil, fmt.Errorf("%s: \"version\" must be a number: %w", path, err)
	}
	if version != Version {
		rel := "newer"
		if version < Version {
			rel = "older"
		}
		return nil, fmt.Errorf("%s: state file version %d is %s than fleet's (want %d)", path, version, rel, Version)
	}
	f.version = version

	if skillsRaw, ok := raw["skills"]; ok {
		var skills map[string]json.RawMessage
		if err := json.Unmarshal(skillsRaw, &skills); err != nil {
			return nil, fmt.Errorf("%s: \"skills\" must be an object: %w", path, err)
		}
		for name, entryRaw := range skills {
			s, err := parseSkill(entryRaw)
			if err != nil {
				return nil, fmt.Errorf("%s: skill %q: %w", path, name, err)
			}
			f.skills[name] = s
		}
	}
	for k, v := range raw {
		if k == "version" || k == "skills" {
			continue
		}
		f.unknown[k] = v
	}
	return f, nil
}

func parseSkill(raw json.RawMessage) (*skill, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("must be an object: %w", err)
	}
	s := &skill{harnesses: map[string]json.RawMessage{}, unknown: map[string]json.RawMessage{}}
	if h, ok := fields["harnesses"]; ok {
		if err := json.Unmarshal(h, &s.harnesses); err != nil {
			return nil, fmt.Errorf("\"harnesses\" must be an object: %w", err)
		}
	}
	for k, v := range fields {
		if k == "harnesses" {
			continue
		}
		s.unknown[k] = v
	}
	return s, nil
}

// Save writes the state file, creating its directory when needed. The
// write is atomic (temp file + rename) so a crash cannot truncate it.
func Save(path string, f *File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := f.render()
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// render produces the canonical file text: fixed key order for fleet's own
// fields, unknown fields after them in sorted order, two-space indent.
func (f *File) render() ([]byte, error) {
	var b strings.Builder
	b.WriteString("{\n  \"version\": ")
	vb, err := json.Marshal(f.version)
	if err != nil {
		return nil, err
	}
	b.Write(vb)

	b.WriteString(",\n  \"skills\": ")
	if err := writeSkills(&b, f.skills); err != nil {
		return nil, err
	}

	for _, k := range sortedKeys(f.unknown) {
		b.WriteString(",\n  ")
		if err := writeKV(&b, k, f.unknown[k]); err != nil {
			return nil, err
		}
	}
	b.WriteString("\n}\n")
	return []byte(b.String()), nil
}

func writeSkills(b *strings.Builder, skills map[string]*skill) error {
	if len(skills) == 0 {
		b.WriteString("{}")
		return nil
	}
	b.WriteString("{")
	for i, name := range sortedKeys(skills) {
		if i > 0 {
			b.WriteString(",")
		}
		nameJSON, err := json.Marshal(name)
		if err != nil {
			return err
		}
		b.WriteString("\n    ")
		b.Write(nameJSON)
		b.WriteString(": ")
		if err := writeSkill(b, skills[name]); err != nil {
			return err
		}
	}
	b.WriteString("\n  }")
	return nil
}

func writeSkill(b *strings.Builder, s *skill) error {
	b.WriteString("{\n      \"harnesses\": ")
	if len(s.harnesses) == 0 {
		b.WriteString("{}")
	} else {
		b.WriteString("{")
		for i, h := range sortedKeys(s.harnesses) {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("\n        ")
			if err := writeKV(b, h, s.harnesses[h]); err != nil {
				return err
			}
		}
		b.WriteString("\n      }")
	}
	for _, k := range sortedKeys(s.unknown) {
		b.WriteString(",\n      ")
		if err := writeKV(b, k, s.unknown[k]); err != nil {
			return err
		}
	}
	b.WriteString("\n    }")
	return nil
}

// writeKV writes "key": raw with a single separating space.
func writeKV(b *strings.Builder, key string, raw json.RawMessage) error {
	keyJSON, err := json.Marshal(key)
	if err != nil {
		return err
	}
	b.Write(keyJSON)
	b.WriteString(": ")
	b.Write(raw)
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Disabled returns the skill names disabled for harness, sorted.
func (f *File) Disabled(harness string) []string {
	var names []string
	for name, s := range f.skills {
		if string(s.harnesses[harness]) == `"off"` {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// Names returns every skill the state file mentions, sorted — the universe
// doctor compares configs against.
func (f *File) Names() []string {
	return sortedKeys(f.skills)
}

// Universe merges storeNames with the state's own names into one deduped
// list, store order first: everything harness configs can meaningfully
// talk about — a disable may outlive the skill it was recorded for.
func (f *File) Universe(storeNames []string) []string {
	seen := make(map[string]bool, len(storeNames)+len(f.skills))
	out := make([]string, 0, len(storeNames)+len(f.skills))
	for _, name := range storeNames {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	for _, name := range f.Names() {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// IsDisabled reports whether skill is disabled for harness.
func (f *File) IsDisabled(name, harness string) bool {
	s, ok := f.skills[name]
	return ok && string(s.harnesses[harness]) == `"off"`
}

// SetDisabled records that skill is disabled for harness. Idempotent.
func (f *File) SetDisabled(name, harness string) {
	s, ok := f.skills[name]
	if !ok {
		s = &skill{harnesses: map[string]json.RawMessage{}, unknown: map[string]json.RawMessage{}}
		f.skills[name] = s
	}
	s.harnesses[harness] = json.RawMessage(`"off"`)
}

// SetEnabled records that skill is enabled for harness: the "off" entry is
// removed and the skill entry with it once nothing is left. Values fleet
// doesn't recognize (written by a newer fleet) are left in place.
func (f *File) SetEnabled(name, harness string) {
	s, ok := f.skills[name]
	if !ok {
		return
	}
	if string(s.harnesses[harness]) == `"off"` {
		delete(s.harnesses, harness)
	}
	if len(s.harnesses) == 0 && len(s.unknown) == 0 {
		delete(f.skills, name)
	}
}
