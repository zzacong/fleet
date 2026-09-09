// Package snapshot builds the read-side picture of the world every fleet
// face renders: the canonical store's skills plus custom skills from the
// tracked set and the fleet-home fallback, each with its per-harness
// enablement and update status. The ls
// verb and the TUI both render a snapshot; neither keeps its own copy of
// the scan, read, grouping, or badge logic. Building one writes only
// fleet's own update-check cache; nothing else is touched.
package snapshot

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/outdated"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/scan"
	"github.com/zzacong/fleet/internal/skillindex"
)

// SkillRow is one skill as a snapshot reports it: identity, origin,
// description, per-harness enablement, and the update badge. The same
// shape backs the ls table, the ls --json output, and the TUI matrix.
type SkillRow struct {
	Name        string            `json:"name"`
	Custom      bool              `json:"custom"`
	Source      string            `json:"source,omitempty"`
	SourceType  string            `json:"sourceType,omitempty"`
	Hash        string            `json:"hash,omitempty"`
	InstalledAt string            `json:"installedAt,omitempty"`
	UpdatedAt   string            `json:"updatedAt,omitempty"`
	Description string            `json:"description,omitempty"`
	States      map[string]string `json:"states"`
	// Outdated is the tri-state update badge: true = update available,
	// false = current, null = unknown (custom skills, non-GitHub sources,
	// and failed checks are unknown — fleet never guesses). Always present
	// in the JSON so scripts can rely on the key.
	Outdated *bool `json:"outdated"`
}

// Report is the full picture: the installed harnesses found on this
// machine (in harness.All's column order), and every skill.
type Report struct {
	Harnesses []string   `json:"harnesses"`
	Skills    []SkillRow `json:"skills"`
}

// DefaultTreeClient builds the update-check client: the real GitHub API
// behind the persistent TTL cache in fleet's config dir. Callers keep a
// variable so tests can swap in a scripted stub.
func DefaultTreeClient(p *paths.Paths) outdated.TreeClient {
	return outdated.NewCachingClient(outdated.NewHTTPClient(""), filepath.Join(p.FleetConfigDir(), "tree-cache.json"))
}

// Build scans the canonical store, every tracked custom home (the explicit
// repo list in order, then the auto-tracked fleet-home checkouts
// alphabetically, via the tracked set), and the unversioned fleet-home
// fallback, then every installed harness's own config, and classifies each
// installed skill's update state. Check failures come back as warnings, not
// errors. Display precedence is explicit-list order, then fleet-home
// checkouts alphabetically, then the unversioned fallback, then the
// canonical store. A name present in more than one source appears once, from
// the highest-precedence source. The other copy is not shown — its existence
// is doctor drift.
func Build(ctx context.Context, p *paths.Paths, client outdated.TreeClient) (*Report, []string, error) {
	// Every skill source scanned once through the skill index (the
	// canonical store, every tracked collection in precedence order,
	// then the fleet-home fallback). Per-home scan failures fail the
	// build, as before.
	idx, errs, err := skillindex.Load(p)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve tracked repos: %w", err)
	}
	if serr, ok := errs[idx.Store()]; ok {
		return nil, nil, fmt.Errorf("scan canonical store: %w", serr)
	}
	if serr, ok := errs[idx.Fallback()]; ok {
		return nil, nil, fmt.Errorf("scan fleet home: %w", serr)
	}
	for _, home := range idx.CustomHomes() {
		if home == idx.Fallback() {
			continue
		}
		if serr, ok := errs[home]; ok {
			return nil, nil, fmt.Errorf("scan tracked repo %s: %w", filepath.Dir(home), serr)
		}
	}
	// Union with precedence tracked order > fleet-home > canonical: apply
	// lowest first so the highest-precedence source wins.
	skillsByName := make(map[string]scan.Skill)
	customHomeNames := map[string]bool{}
	homes := idx.Homes()
	for i := len(homes) - 1; i >= 0; i-- {
		home := homes[i]
		for _, s := range idx.Skills(home) {
			skillsByName[s.Name] = s
			if home != idx.Store() {
				customHomeNames[s.Name] = true
			}
		}
	}
	skills := make([]scan.Skill, 0, len(skillsByName))
	for _, s := range skillsByName {
		skills = append(skills, s)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Dir < skills[j].Dir })
	lock, err := scan.ReadLockfile(p.SkillLock())
	if err != nil {
		return nil, nil, fmt.Errorf("read skills lockfile: %w", err)
	}

	installed := harness.Installed(p)

	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.Name
	}
	reads := make([]harness.ReadResult, len(installed))
	for i, a := range installed {
		read, err := a.Read(names)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s config: %w", a.Harness(), err)
		}
		reads[i] = read
	}

	harnessNames := make([]string, len(installed))
	for i, a := range installed {
		harnessNames[i] = string(a.Harness())
	}

	// One update check for every skill in the store, grouped inside the
	// checker: installed skills carry their lockfile provenance, customs
	// stay the zero entry (unknown by definition, no API call). Tracked-set
	// and fleet-home customs are zero entries — a lingering lock entry from a
	// pre-adoption install must not make fleet check (or misreport) a
	// skill that now lives in a custom home.
	entries := make(map[string]outdated.Entry, len(skills))
	for _, s := range skills {
		if customHomeNames[s.Name] {
			entries[s.Dir] = outdated.Entry{}
			continue
		}
		if prov, ok := lock[s.Dir]; ok {
			entries[s.Dir] = outdated.Entry{
				Source:     prov.Source,
				SourceType: prov.SourceType,
				SkillPath:  prov.SkillPath,
				Ref:        prov.Ref,
				Hash:       prov.Hash,
			}
		} else {
			entries[s.Dir] = outdated.Entry{}
		}
	}
	check := outdated.Check(ctx, client, entries)

	rows := make([]SkillRow, len(skills))
	for i, s := range skills {
		states := make(map[string]string, len(installed))
		for j := range installed {
			state := reads[j].States[s.Name]
			if state == "" {
				state = harness.StateOn // adapters report every name; be defensive anyway
			}
			states[harnessNames[j]] = string(state)
		}

		row := SkillRow{
			Name:        s.Name,
			Description: s.Description,
			States:      states,
			Outdated:    outdatedPtr(check.Statuses[s.Dir]),
		}
		if customHomeNames[s.Name] {
			// Custom by definition: it lives in a custom home, unversioned
			// by the skills CLI. A lingering lock entry under the same
			// directory name is stale provenance, not provenance.
			row.Custom = true
		} else if prov, ok := lock[s.Dir]; ok {
			row.Source = prov.Source
			row.SourceType = prov.SourceType
			row.Hash = prov.Hash
			row.InstalledAt = prov.InstalledAt
			row.UpdatedAt = prov.UpdatedAt
		} else {
			row.Custom = true
		}
		rows[i] = row
	}

	// Custom first, then installed grouped by source repo, alphabetical
	// inside each group.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Custom != rows[j].Custom {
			return rows[i].Custom
		}
		if rows[i].Source != rows[j].Source {
			return rows[i].Source < rows[j].Source
		}
		return rows[i].Name < rows[j].Name
	})

	return &Report{Harnesses: harnessNames, Skills: rows}, check.Warnings, nil
}

// outdatedPtr renders the tri-state for JSON: outdated true, current
// false, unknown null.
func outdatedPtr(s outdated.Status) *bool {
	var v bool
	switch s {
	case outdated.StatusOutdated:
		v = true
	case outdated.StatusCurrent:
		v = false
	default:
		return nil
	}
	return &v
}
