// Package paths derives every filesystem location fleet reads from a
// single injected home root, plus the fleet repo root that owns custom
// skills. Adapters and commands never call os.UserHomeDir() themselves;
// they receive a *Paths built with New (tests, sandboxes) or FromEnv (the
// real binary, honoring FLEET_HOME).
package paths

import (
	"os"
	"path/filepath"

	"github.com/zzacong/fleet/internal/config"
)

// Paths holds the home root every fleet path derives from.
type Paths struct {
	// Home is the injected home root. FLEET_HOME overrides it for the real
	// binary; tests pass a fake home so nothing outside the project (or a
	// t.TempDir) is ever touched.
	Home string
	// Repo is the fleet repo root whose skills/ directory holds custom
	// skills. FLEET_REPO overrides it for the real binary; empty when none
	// was found — commands that need the repo say so instead of guessing.
	Repo string
}

// New builds Paths under an explicit home root.
func New(home string) *Paths {
	return &Paths{Home: home}
}

// WithRepo builds Paths under an explicit home and repo root (tests and
// callers that already know where the repo is).
func WithRepo(home, repo string) *Paths {
	return &Paths{Home: home, Repo: repo}
}

// FromEnv resolves the home root for the real binary: FLEET_HOME when set,
// otherwise the user's home directory. The repo root is resolved as
// FLEET_REPO env > ~/.config/fleet/config.json: skillsRepo > "" (no
// value). There is no DiscoverRepo walk-up on the read path.
func FromEnv() (*Paths, error) {
	var p *Paths
	if root := os.Getenv("FLEET_HOME"); root != "" {
		p = New(root)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		p = New(home)
	}

	if root := os.Getenv("FLEET_REPO"); root != "" {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		p.Repo = abs
		return p, nil
	}
	repo, err := loadSkillsRepo(p.FleetConfigFile())
	if err != nil {
		return nil, err
	}
	p.Repo = repo
	return p, nil
}

func loadSkillsRepo(path string) (string, error) {
	f, err := config.Load(path)
	if err != nil {
		return "", err
	}
	return f.SkillsRepo(), nil
}

// DiscoverRepo walks up from startDir to the filesystem root and returns
// the first directory containing .git — a directory in a normal checkout,
// a file in a git worktree. Empty when none is found. It is retained only
// for fleet config suggestion text and is never called implicitly on the
// read path.
func DiscoverRepo(startDir string) string {
	dir := startDir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
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
func (p *Paths) OpenCodeSkills() string { return join(p, ".config", "opencode", "skills") }

// pi (strict JSON settings).

func (p *Paths) PiDir() string      { return join(p, ".pi") }
func (p *Paths) PiSettings() string { return join(p, ".pi", "agent", "settings.json") }
func (p *Paths) PiSkills() string   { return join(p, ".pi", "agent", "skills") }

// codex (TOML config; skills arrive natively or through links).

func (p *Paths) CodexDir() string    { return join(p, ".codex") }
func (p *Paths) CodexConfig() string { return join(p, ".codex", "config.toml") }
func (p *Paths) CodexSkills() string { return join(p, ".codex", "skills") }

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

// Fleet's own config home (state file, config file, tree cache, fallback skills).

func (p *Paths) FleetConfigDir() string     { return join(p, ".config", "fleet") }
func (p *Paths) FleetStateFile() string     { return join(p, ".config", "fleet", "state.json") }
func (p *Paths) FleetConfigFile() string    { return join(p, ".config", "fleet", "config.json") }
func (p *Paths) FleetTreeCacheFile() string { return join(p, ".config", "fleet", "tree-cache.json") }
func (p *Paths) FleetHomeSkills() string    { return join(p, ".config", "fleet", "skills") }

// The fleet repo (custom skills).

// RepoSkills is the repo's skills/ directory, where custom skills live and
// stay versioned. Empty when no repo is bound; callers must check and say
// so rather than deriving paths from an empty root.
func (p *Paths) RepoSkills() string {
	if p.Repo == "" {
		return ""
	}
	return filepath.Join(p.Repo, "skills")
}
