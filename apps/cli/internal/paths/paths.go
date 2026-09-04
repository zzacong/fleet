// Package paths derives every filesystem location fleet reads from a
// single injected home root. Adapters and commands never call
// os.UserHomeDir() themselves; they receive a *Paths built with New
// (tests, sandboxes) or FromEnv (the real binary, honoring FLEET_HOME).
package paths

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zzacong/fleet/internal/config"
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
// otherwise the user's home directory. There is no DiscoverRepo walk-up on
// the read path.
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

// TrackedRepos resolves the ordered tracked set of versioned custom-skill
// homes (repo roots): the FLEET_REPO env override when set (prepended,
// highest precedence; kept working in code only, never presented in help or
// user docs), then the config's explicit non-fleet-home list in order, then
// every fleet-home checkout slot present on disk (immediate child
// directories of FleetReposDir, alphabetical by directory name). The retired
// single-pointer file key is never read: old-key-only configs resolve as if
// unset. Entries are deduped by cleaned path with the first occurrence
// winning. A missing checkout parent means no auto-tracked slots, not an
// error. The unversioned fleet-home fallback (FleetHomeSkills) is not part
// of the set; display and adopt layers add it where they need it.
func (p *Paths) TrackedRepos() ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(path string) {
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		if seen[clean] {
			return
		}
		seen[clean] = true
		out = append(out, clean)
	}
	if env := os.Getenv("FLEET_REPO"); env != "" {
		abs, err := filepath.Abs(env)
		if err != nil {
			return nil, err
		}
		add(abs)
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return nil, err
	}
	for _, repo := range f.SkillsRepos() {
		add(repo)
	}
	entries, err := os.ReadDir(p.FleetReposDir())
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		add(filepath.Join(p.FleetReposDir(), name))
	}
	return out, nil
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

// FleetReposDir is the fleet-home checkout parent: every immediate child
// directory is an auto-tracked customs checkout slot (presence on disk,
// never a config write). Clones landing inside the fleet home stay
// convention-tracked; clones outside are remembered in the config's
// explicit repo-root list.
func (p *Paths) FleetReposDir() string { return join(p, ".config", "fleet", "repos") }

// InsideFleetHome reports whether path sits inside the fleet home dir
// (~/.config/fleet). The check is lexical on the cleaned path; callers
// pass expanded absolute paths. Relative paths classify as outside.
func (p *Paths) InsideFleetHome(path string) bool {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return false
	}
	rel, err := filepath.Rel(p.FleetConfigDir(), clean)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
