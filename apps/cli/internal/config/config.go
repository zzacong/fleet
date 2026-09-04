// Package config owns fleet's machine-local config file
// (~/.config/fleet/config.json): the persistent pointers to versioned
// skills homes. The file is JSON, FLEET_HOME-aware, atomic via temp+rename,
// unknown fields preserved verbatim, canonical formatting. Missing file
// means nothing is set. The explicit repo-root list (skillsRepos) and the
// adopt-target collection dir (adoptTarget) are the tracked-set model. The
// old single repo-root file key (skillsRepo) is superseded: it is preserved
// verbatim as an unknown field and never interpreted, so old-key-only
// configs behave as unset. There is no automatic migration in this layer.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// File is the parsed config file.
type File struct {
	skillsRepos []string
	adoptTarget string
	unknown     map[string]json.RawMessage
}

// Load reads the config file. A missing file is an empty config, not an
// error. Malformed JSON, a non-string adoptTarget, or a non-array-of-strings
// skillsRepos is an error. The superseded single-pointer key (skillsRepo),
// when present, is preserved verbatim as an unknown field and never
// interpreted: old-key-only configs behave as unset, with no migration.
func Load(path string) (*File, error) {
	f := &File{
		unknown: map[string]json.RawMessage{},
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, err
	}
	if len(bytesTrimSpace(body)) == 0 {
		return f, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if listRaw, ok := raw["skillsRepos"]; ok {
		var list []string
		if err := json.Unmarshal(listRaw, &list); err != nil {
			return nil, fmt.Errorf("%s: \"skillsRepos\" must be an array of strings: %w", path, err)
		}
		f.skillsRepos = list
	}
	if targetRaw, ok := raw["adoptTarget"]; ok {
		var target string
		if err := json.Unmarshal(targetRaw, &target); err != nil {
			return nil, fmt.Errorf("%s: \"adoptTarget\" must be a string: %w", path, err)
		}
		f.adoptTarget = target
	}
	for k, v := range raw {
		if k == "skillsRepos" || k == "adoptTarget" {
			continue
		}
		f.unknown[k] = v
	}
	return f, nil
}

func bytesTrimSpace(b []byte) []byte {
	return bytes.TrimSpace(b)
}

// Save writes the config file, creating its directory when needed. The
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

// SkillsRepos returns a copy of the explicit non-fleet-home repo-root list,
// in precedence order. Empty means none.
func (f *File) SkillsRepos() []string {
	out := make([]string, len(f.skillsRepos))
	copy(out, f.skillsRepos)
	return out
}

// SetSkillsRepos replaces the explicit repo-root list (order significant,
// caller validates each entry).
func (f *File) SetSkillsRepos(list []string) {
	f.skillsRepos = append([]string(nil), list...)
}

// UnsetSkillsRepos clears the explicit repo-root list.
func (f *File) UnsetSkillsRepos() { f.skillsRepos = nil }

// AdoptTarget returns the configured adopt-target collection dir, or empty
// when unset (callers fall back to the fleet-home skills dir).
func (f *File) AdoptTarget() string { return f.adoptTarget }

// SetAdoptTarget records the adopt-target collection dir (absolute, caller
// validates).
func (f *File) SetAdoptTarget(p string) { f.adoptTarget = p }

// UnsetAdoptTarget clears the adopt target.
func (f *File) UnsetAdoptTarget() { f.adoptTarget = "" }

// Unknown returns a copy of unknown fields (for testing).
func (f *File) Unknown() map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(f.unknown))
	for k, v := range f.unknown {
		out[k] = v
	}
	return out
}

// NormalizeKey maps a user-supplied config key to its canonical kebab
// form, accepting the existing kebab/camel pair convention. It returns ""
// for unknown keys. Known pairs: skills-repos/skillsRepos,
// adopt-target/adoptTarget. The superseded single-pointer key
// (skills-repo/skillsRepo) is unknown: it fails like any other unknown key.
func NormalizeKey(k string) string {
	switch k {
	case "skills-repos", "skillsRepos":
		return "skills-repos"
	case "adopt-target", "adoptTarget":
		return "adopt-target"
	default:
		return ""
	}
}

// ExpandPath expands a leading "~" or "~/" to the user's home directory,
// mirroring the fleet config set behavior. Anything else is unchanged.
func ExpandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	return p
}

// AbsolutePath expands a leading "~" and requires the result to be
// absolute, returning the cleaned path. Both config values (the
// explicit repo-root list and the adopt-target collection dir) honor the
// same home-expansion and absolute-path rule.
func AbsolutePath(input string) (string, error) {
	expanded := ExpandPath(input)
	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("path must be absolute: %q", input)
	}
	return filepath.Clean(expanded), nil
}

// render produces canonical file text: skillsRepos, then adoptTarget when
// set, then unknown fields in sorted order (including the superseded
// single-pointer key when some old file still carries it — preserved, never
// migrated), two-space indent.
func (f *File) render() ([]byte, error) {
	var b strings.Builder
	b.WriteString("{\n")
	first := true
	// sep starts a new key line. After "{" the buffer already ends with
	// a newline (first key); after a value it needs ",\n".
	sep := func() {
		if !first {
			b.WriteString(",")
			b.WriteString("\n")
		}
		b.WriteString("  ")
		first = false
	}
	if len(f.skillsRepos) > 0 {
		sep()
		b.WriteString("\"skillsRepos\": [")
		for i, repo := range f.skillsRepos {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("\n    ")
			vb, err := json.Marshal(repo)
			if err != nil {
				return nil, err
			}
			b.Write(vb)
		}
		b.WriteString("\n  ]")
	}
	if f.adoptTarget != "" {
		sep()
		b.WriteString("\"adoptTarget\": ")
		vb, err := json.Marshal(f.adoptTarget)
		if err != nil {
			return nil, err
		}
		b.Write(vb)
	}
	for _, k := range sortedKeys(f.unknown) {
		sep()
		if err := writeKV(&b, k, f.unknown[k]); err != nil {
			return nil, err
		}
	}
	if !first {
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return []byte(b.String()), nil
}

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
