package harness

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/zacong/fleet/internal/paths"
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

	var cfg struct {
		Skills struct {
			Config []struct {
				Name    string `toml:"name"`
				Path    string `toml:"path"`
				Enabled *bool  `toml:"enabled"`
			} `toml:"config"`
		} `toml:"skills"`
	}
	if err := toml.Unmarshal(body, &cfg); err != nil {
		return ReadResult{}, fmt.Errorf("parse %s: %w", filepath.Base(a.home.CodexConfig()), err)
	}

	// Selections are only meaningful together with an enabled flag; entries
	// without enabled don't change anything.
	type selection struct {
		enabled bool
	}
	states := map[string]*selection{}
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
		enabled := *entry.Enabled
		states[name] = &selection{enabled: enabled} // later entries override earlier ones
	}

	for _, name := range names {
		if s, ok := states[name]; ok && !s.enabled {
			res.States[name] = StateOff
		}
	}
	return res, nil
}
