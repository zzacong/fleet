// Package paths derives every filesystem location fleet reads from a single
// injected home root. Adapters and commands never call os.UserHomeDir()
// themselves; they receive a *Paths built with New (tests, sandboxes) or
// FromEnv (the real binary, honoring FLEET_HOME).
package paths

import (
	"os"
	"path/filepath"
)

// Paths holds the home root every fleet path derives from.
type Paths struct {
	// Home is the injected home root. FLEET_HOME overrides it for the real
	// binary; tests pass a fake home so nothing outside the project (or a
	// t.TempDir) is ever touched.
	Home string
}

// New builds Paths under an explicit home root.
func New(home string) *Paths {
	return &Paths{Home: home}
}

// FromEnv resolves the home root for the real binary: FLEET_HOME when set,
// otherwise the user's home directory.
func FromEnv() (*Paths, error) {
	if root := os.Getenv("FLEET_HOME"); root != "" {
		return New(root), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return New(home), nil
}

func join(p *Paths, elems ...string) string {
	return filepath.Join(append([]string{p.Home}, elems...)...)
}

// Canonical store and skills CLI lockfile.

func (p *Paths) AgentsDir() string   { return join(p, ".agents") }
func (p *Paths) SkillsStore() string { return join(p, ".agents", "skills") }
func (p *Paths) SkillLock() string   { return join(p, ".agents", ".skill-lock.json") }

// opencode (JSONC config, V1 and V2 dialects).

func (p *Paths) OpenCodeDir() string    { return join(p, ".config", "opencode") }
func (p *Paths) OpenCodeConfig() string { return join(p, ".config", "opencode", "opencode.jsonc") }

// pi (strict JSON settings).

func (p *Paths) PiDir() string      { return join(p, ".pi") }
func (p *Paths) PiSettings() string { return join(p, ".pi", "agent", "settings.json") }

// codex (TOML config).

func (p *Paths) CodexDir() string    { return join(p, ".codex") }
func (p *Paths) CodexConfig() string { return join(p, ".codex", "config.toml") }

// claude code (strict JSON settings; skills arrive through links).

func (p *Paths) ClaudeDir() string      { return join(p, ".claude") }
func (p *Paths) ClaudeSkills() string   { return join(p, ".claude", "skills") }
func (p *Paths) ClaudeSettings() string { return join(p, ".claude", "settings.json") }

// Cursor (native canonical-store reader, no per-skill config).

func (p *Paths) CursorDir() string    { return join(p, ".cursor") }
func (p *Paths) CursorSkills() string { return join(p, ".cursor", "skills") }

// IBM Bob (skills arrive through links into ~/.bob/skills).

func (p *Paths) BobDir() string    { return join(p, ".bob") }
func (p *Paths) BobSkills() string { return join(p, ".bob", "skills") }

// Fleet's own config home (state file).

func (p *Paths) FleetConfigDir() string { return join(p, ".config", "fleet") }
func (p *Paths) FleetStateFile() string { return join(p, ".config", "fleet", "state.json") }
