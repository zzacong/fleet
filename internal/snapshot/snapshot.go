// Package snapshot builds the read-side picture of the world every fleet
// face renders: the canonical store's skills plus the fleet repo's custom
// skills, each with its per-harness enablement and update status. The ls
// verb and the TUI both render a snapshot; neither keeps its own copy of
// the scan, read, grouping, or badge logic. Building one writes only
// fleet's own update-check cache; nothing else is touched.
package snapshot

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/zacong/fleet/internal/harness"
	"github.com/zacong/fleet/internal/outdated"
	"github.com/zacong/fleet/internal/paths"
	"github.com/zacong/fleet/internal/scan"
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

// Build scans the canonical store and the fleet repo's custom skills,
// then every installed harness's own config, and classifies each installed
// skill's update state. Check failures come back as warnings, not errors.
func Build(ctx context.Context, p *paths.Paths, client outdated.TreeClient) (*Report, []string, error) {
	storeSkills, err := scan.ScanStore(p.SkillsStore())
	if err != nil {
		return nil, nil, fmt.Errorf("scan canonical store: %w", err)
	}
	// Repo customs join the list; custom means lives in the repo. A name
	// present in both places reports once, from the canonical store — the
	// double-visibility is drift for doctor to flag, not ls's job.
	repoNames := map[string]bool{}
	skills := storeSkills
	if p.Repo != "" {
		repoSkills, err := scan.ScanStore(p.RepoSkills())
		if err != nil {
			return nil, nil, fmt.Errorf("scan repo skills: %w", err)
		}
		seen := make(map[string]bool, len(storeSkills))
		for _, s := range storeSkills {
			seen[s.Name] = true
		}
		for _, s := range repoSkills {
			if seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			repoNames[s.Name] = true
			skills = append(skills, s)
		}
	}
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
	// stay the zero entry (unknown by definition, no API call). Repo
	// customs are skipped entirely — a lingering lock entry from a
	// pre-adoption install must not make fleet check (or misreport) a
	// skill that now lives in the repo.
	entries := make(map[string]outdated.Entry, len(skills))
	for _, s := range skills {
		if repoNames[s.Name] {
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
		if repoNames[s.Name] {
			// Custom by definition: it lives in the repo, unversioned by
			// the skills CLI. A lingering lock entry under the same
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
