package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/zzacong/fleet/internal/paths"
)

// Codex reads ~/.codex/config.toml. Codex discovers the canonical store
// natively; its config disables skills through [[skills.config]] entries
// with a name (or path) selector and an enabled flag. Later entries override
// earlier ones; unknown keys and comments are ignored by codex and by us.
type CodexAdapter struct {
	home *paths.Paths
}

// NewCodex builds the codex adapter under an injected home root.
func NewCodex(p *paths.Paths) *CodexAdapter { return &CodexAdapter{home: p} }

// Harness implements Adapter.
func (a *CodexAdapter) Harness() Harness { return Codex }

// Installed implements Adapter.
func (a *CodexAdapter) Installed() bool { return isDir(a.home.CodexDir()) }

// Dir implements Adapter.
func (a *CodexAdapter) Dir() string { return a.home.CodexDir() }

// Read implements Adapter.
func (a *CodexAdapter) Read(names []string) (ReadResult, error) {
	res := ReadResult{States: onForAll(names)}

	body, err := os.ReadFile(a.home.CodexConfig())
	if err != nil {
		if os.IsNotExist(err) {
			return res, nil
		}
		return ReadResult{}, err
	}

	disabled, err := codexDisabledEntries(body)
	if err != nil {
		return ReadResult{}, fmt.Errorf("parse %s: %w", filepath.Base(a.home.CodexConfig()), err)
	}
	for _, name := range names {
		if disabled[name] {
			res.States[name] = StateOff
		}
	}

	// Every name the config currently disables, however it selects the
	// skill (name or path selector): it is a disable entry, and doctor
	// must see it even when fleet wouldn't write that shape.
	for name, off := range disabled {
		if off {
			res.Disables = append(res.Disables, name)
		}
	}
	sort.Strings(res.Disables)
	return res, nil
}

// codexConfig mirrors the subset of ~/.codex/config.toml fleet models.
type codexConfig struct {
	Skills struct {
		Config []codexEntry `toml:"config"`
	} `toml:"skills"`
}

type codexEntry struct {
	Name    string `toml:"name"`
	Path    string `toml:"path"`
	Enabled *bool  `toml:"enabled"`
}

// codexDisabledEntries returns the skill names the config's
// [[skills.config]] entries disable. Selections are only meaningful
// together with an enabled flag; entries without one change nothing. Later
// entries override earlier ones.
func codexDisabledEntries(body []byte) (map[string]bool, error) {
	var cfg codexConfig
	if err := toml.Unmarshal(body, &cfg); err != nil {
		return nil, err
	}

	disabled := map[string]bool{}
	for _, entry := range cfg.Skills.Config {
		name := entry.Name
		if name == "" && entry.Path != "" {
			// path selectors point at .../skills/<name>/SKILL.md; codex
			// canonicalizes them, we recover the skill from the path.
			name = filepath.Base(filepath.Dir(entry.Path))
		}
		if name == "" || entry.Enabled == nil {
			continue
		}
		disabled[name] = !*entry.Enabled // later entries override earlier ones
	}
	return disabled, nil
}
