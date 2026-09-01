package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/zzacong/fleet/internal/paths"
)

// Claude reads ~/.claude/settings.json (strict JSON) and checks link
// presence in ~/.claude/skills. Claude is not a native canonical-store
// reader: a skill only reaches it through a link (or directory) named after
// the skill, so without one the state is absent. skillOverrides maps a
// skill name to "off" (hidden) or other values such as
// "user-invocable-only" (still loads on explicit invocation, so still on).
type ClaudeAdapter struct {
	home *paths.Paths
}

// NewClaude builds the claude code adapter under an injected home root.
func NewClaude(p *paths.Paths) *ClaudeAdapter { return &ClaudeAdapter{home: p} }

// Harness implements Adapter.
func (a *ClaudeAdapter) Harness() Harness { return Claude }

// Installed implements Adapter.
func (a *ClaudeAdapter) Installed() bool { return isDir(a.home.ClaudeDir()) }

// Read implements Adapter.
func (a *ClaudeAdapter) Read(names []string) (ReadResult, error) {
	res := ReadResult{States: make(map[string]State, len(names))}

	var overrides map[string]string
	body, err := os.ReadFile(a.home.ClaudeSettings())
	if err != nil {
		if !os.IsNotExist(err) {
			return ReadResult{}, err
		}
	} else {
		var settings struct {
			SkillOverrides map[string]string `json:"skillOverrides"`
		}
		if err := json.Unmarshal(body, &settings); err != nil {
			return ReadResult{}, fmt.Errorf("parse %s: %w", filepath.Base(a.home.ClaudeSettings()), err)
		}
		overrides = settings.SkillOverrides
	}

	for _, name := range names {
		if !linkPresent(filepath.Join(a.home.ClaudeSkills(), name)) {
			res.States[name] = StateAbsent
			continue
		}
		res.Linked = append(res.Linked, name)
		if overrides[name] == "off" {
			res.States[name] = StateOff
			continue
		}
		res.States[name] = StateOn
	}
	sort.Strings(res.Linked)

	// The exact "off" entries, independent of link presence: an override
	// for an unlinked skill is stale, but it is still a disable entry in
	// the config that doctor must see.
	for name, value := range overrides {
		if value == "off" {
			res.Disables = append(res.Disables, name)
		}
	}
	sort.Strings(res.Disables)
	return res, nil
}
