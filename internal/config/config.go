// Package config owns fleet's machine-local config file
// (~/.config/fleet/config.json): the persistent pointer to the versioned
// skills repo. The file is JSON, FLEET_HOME-aware, atomic via temp+rename,
// unknown fields preserved verbatim, canonical formatting. Missing file
// means no skills repo is set. FLEET_REPO env overrides the file.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zzacong/fleet/internal/paths"
)

// File is the parsed config file.
type File struct {
	skillsRepo string
	unknown    map[string]json.RawMessage
}

// Load reads the config file. A missing file is an empty config, not an
// error. Malformed JSON or a non-string skillsRepo is an error.
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
	if repoRaw, ok := raw["skillsRepo"]; ok {
		var repo string
		if err := json.Unmarshal(repoRaw, &repo); err != nil {
			return nil, fmt.Errorf("%s: \"skillsRepo\" must be a string: %w", path, err)
		}
		f.skillsRepo = repo
	}
	for k, v := range raw {
		if k == "skillsRepo" {
			continue
		}
		f.unknown[k] = v
	}
	return f, nil
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
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

// SkillsRepo returns the configured skills repo path, or empty when unset.
func (f *File) SkillsRepo() string { return f.skillsRepo }

// SetSkillsRepo records the skills repo path (absolute, caller validates).
func (f *File) SetSkillsRepo(p string) { f.skillsRepo = p }

// UnsetSkillsRepo clears the skills repo pointer.
func (f *File) UnsetSkillsRepo() { f.skillsRepo = "" }

// Unknown returns a copy of unknown fields (for testing).
func (f *File) Unknown() map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(f.unknown))
	for k, v := range f.unknown {
		out[k] = v
	}
	return out
}

// EffectiveRepo resolves the skills repo honoring FLEET_REPO env override.
// Env > file > "".
func EffectiveRepo(p *paths.Paths) (string, error) {
	if env := os.Getenv("FLEET_REPO"); env != "" {
		abs, err := filepath.Abs(env)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	f, err := Load(p.FleetConfigFile())
	if err != nil {
		return "", err
	}
	return f.SkillsRepo(), nil
}

// render produces canonical file text: skillsRepo first when set, then
// unknown fields in sorted order, two-space indent.
func (f *File) render() ([]byte, error) {
	var b strings.Builder
	b.WriteString("{\n")
	first := true
	if f.skillsRepo != "" {
		b.WriteString("  \"skillsRepo\": ")
		vb, err := json.Marshal(f.skillsRepo)
		if err != nil {
			return nil, err
		}
		b.Write(vb)
		first = false
	}
	for _, k := range sortedKeys(f.unknown) {
		if !first {
			b.WriteString(",")
		}
		b.WriteString("\n  ")
		if err := writeKV(&b, k, f.unknown[k]); err != nil {
			return nil, err
		}
		first = false
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
