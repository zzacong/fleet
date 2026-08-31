package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/zacong/fleet/internal/paths"
)

// Pi reads ~/.pi/agent/settings.json (strict JSON). pi discovers the
// canonical store natively; its config disables skills through force-exclude
// entries in the `skills` array, written in the exact form `pi config`
// produces: "-skills/<name>/SKILL.md" (exact path). The bare "!<glob>" form
// also works and is recognized on read. Plain path entries only add
// discovery sources; they never disable anything.
type PiAdapter struct {
	home *paths.Paths
}

// NewPi builds the pi adapter under an injected home root.
func NewPi(p *paths.Paths) *PiAdapter { return &PiAdapter{home: p} }

// Harness implements Adapter.
func (a *PiAdapter) Harness() Harness { return Pi }

// Installed implements Adapter.
func (a *PiAdapter) Installed() bool { return isDir(a.home.PiDir()) }

// Read implements Adapter.
func (a *PiAdapter) Read(names []string) (ReadResult, error) {
	res := ReadResult{States: onForAll(names)}

	body, err := os.ReadFile(a.home.PiSettings())
	if err != nil {
		if os.IsNotExist(err) {
			return res, nil
		}
		return ReadResult{}, err
	}

	var settings struct {
		Skills []string `json:"skills"`
	}
	if err := json.Unmarshal(body, &settings); err != nil {
		return ReadResult{}, fmt.Errorf("parse %s: %w", filepath.Base(a.home.PiSettings()), err)
	}

	exact, globs := parsePiExclusions(settings.Skills)
	for _, name := range names {
		if exact[name] {
			res.States[name] = StateOff
			continue
		}
		for _, glob := range globs {
			if matchPattern(glob, name) {
				res.States[name] = StateOff
				break
			}
		}
	}

	// Only the exact -skills/<name>/SKILL.md form is an entry fleet could
	// own; !glob exclusions are not.
	for name := range exact {
		res.Disables = append(res.Disables, name)
	}
	sort.Strings(res.Disables)
	return res, nil
}

// piExactExclusion matches the form pi config writes: -skills/<name>/SKILL.md,
// relative to ~/.agents.
var piExactExclusion = regexp.MustCompile(`^-skills/([^/]+)/SKILL\.md$`)

// parsePiExclusions splits pi's `skills` array entries into exact-name
// exclusions (the -skills/<name>/SKILL.md form) and !glob exclusions.
// Everything else is a discovery path, not an exclusion.
func parsePiExclusions(entries []string) (exact map[string]bool, globs []string) {
	exact = map[string]bool{}
	for _, entry := range entries {
		if m := piExactExclusion.FindStringSubmatch(entry); m != nil {
			exact[m[1]] = true
			continue
		}
		if len(entry) > 1 && entry[0] == '!' {
			globs = append(globs, entry[1:])
		}
	}
	return exact, globs
}
