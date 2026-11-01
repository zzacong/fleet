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
	"maps"
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

// Report is the full picture: the managed harnesses found on this
// machine (in harness.All's column order), and every skill.
type Report struct {
	Harnesses []string   `json:"harnesses"`
	Skills    []SkillRow `json:"skills"`
}

// DefaultTreeClient builds the update-check client: the real GitHub API
// behind the persistent TTL cache in fleet's config dir. Callers keep a
// variable so tests can swap in a scripted stub.
func DefaultTreeClient(p *paths.Paths) outdated.CachedTreeClient {
	return outdated.NewCachingClient(outdated.NewHTTPClient(""), filepath.Join(p.FleetConfigDir(), "tree-cache.json"))
}

// draft is a snapshot whose update badges are not yet filled in. entries is
// the update-check input keyed by skill directory; dirOf maps each row's
// name back to its directory, since a badge is decided per directory.
type draft struct {
	report  *Report
	entries map[string]outdated.Entry
	dirOf   map[string]string
}

// setBadges fills every row's update badge from statuses, keyed by skill
// directory. A directory with no status stays unknown.
func (d *draft) setBadges(statuses map[string]outdated.Status) {
	for i := range d.report.Skills {
		d.report.Skills[i].Outdated = outdatedPtr(statuses[d.dirOf[d.report.Skills[i].Name]])
	}
}

// Build is the synchronous snapshot: every update check finishes before it
// returns, so the badges are final. The ls verb uses it. Check failures come
// back as warnings, not errors.
func Build(ctx context.Context, p *paths.Paths, client outdated.TreeClient) (*Report, []string, error) {
	d, err := readDraft(p)
	if err != nil {
		return nil, nil, err
	}
	check := outdated.Check(ctx, client, d.entries)
	d.setBadges(check.Statuses)
	return d.report, check.Warnings, nil
}

// Loaded is a snapshot whose update badges may still be pending. Report
// holds every badge the cache could answer; Checks holds the repo trees it
// could not. Warnings are update problems known without a network call.
type Loaded struct {
	Report   *Report
	Checks   *Checks
	Warnings []string
}

// Load builds the snapshot without waiting on GitHub. Badges the cache can
// answer are filled in right away; every other repo tree is left in Checks
// for the caller to run in the background. force skips the cache, so every
// repo is pending: the manual refresh.
func Load(p *paths.Paths, client outdated.CachedTreeClient, force bool) (*Loaded, error) {
	d, err := readDraft(p)
	if err != nil {
		return nil, err
	}
	nameByDir := make(map[string]string, len(d.dirOf))
	for name, dir := range d.dirOf {
		nameByDir[dir] = name
	}
	checks := &Checks{client: client, entries: d.entries, nameByDir: nameByDir, force: force}

	groups, warnings := outdated.Plan(d.entries)
	statuses := map[string]outdated.Status{}
	for _, g := range groups {
		if !force {
			if resp, ok := client.Fresh(g.Owner, g.Repo, g.Ref); ok {
				got, w := outdated.Classify(g, d.entries, resp)
				maps.Copy(statuses, got)
				warnings = append(warnings, w...)
				continue
			}
		}
		checks.groups = append(checks.groups, g)
		for _, key := range g.Keys {
			checks.pending = append(checks.pending, nameByDir[key])
		}
	}
	d.setBadges(statuses)
	sort.Strings(checks.pending)
	sort.Strings(warnings)
	return &Loaded{Report: d.report, Checks: checks, Warnings: warnings}, nil
}

// Checks is the update work a Load left behind: one repo tree per group the
// cache could not answer. Run each group in turn; the caller owns the order
// and decides when to stop.
type Checks struct {
	client    outdated.CachedTreeClient
	entries   map[string]outdated.Entry
	nameByDir map[string]string // skill directory -> row name
	groups    []outdated.Group
	pending   []string // row names still waiting on a Run, sorted
	force     bool     // re-fetch every tree, ignoring the cache's TTL
}

// Len is the number of repo trees left to fetch.
func (c *Checks) Len() int { return len(c.groups) }

// Pending lists the skill names whose badge is waiting on a Run, sorted.
func (c *Checks) Pending() []string { return c.pending }

// Result is one repo tree's badges. Outdated has an entry for every skill
// the group decides, nil for the ones that stayed unknown.
type Result struct {
	Outdated map[string]*bool
	Warnings []string
}

// Run fetches group i and classifies its members. It is safe to call from a
// background command: the cache serializes its own state, and the checks
// read only what Load set.
func (c *Checks) Run(ctx context.Context, i int) Result {
	g := c.groups[i]
	fetch := func(ctx context.Context, owner, repo, ref string) (outdated.TreeResponse, error) {
		if c.force {
			return c.client.Revalidate(ctx, owner, repo, ref)
		}
		return c.client.FetchTree(ctx, owner, repo, ref, "")
	}
	statuses, warnings := outdated.CheckGroup(ctx, fetch, g, c.entries)
	res := Result{Outdated: make(map[string]*bool, len(g.Keys)), Warnings: warnings}
	for _, key := range g.Keys {
		res.Outdated[c.nameByDir[key]] = outdatedPtr(statuses[key])
	}
	return res
}

// readDraft reads the canonical store, every tracked custom home, the fleet-home
// fallback, and every managed harness's config. It returns the rows in
// display order, without update badges. Display precedence is explicit-list
// order, then the unversioned fallback, then the canonical store. A name
// present in more than one source appears once, from the highest-precedence
// source. The other copy is not shown — its existence is doctor drift.
func readDraft(p *paths.Paths) (*draft, error) {
	// Every skill source scanned once through the skill index (the
	// canonical store, every tracked collection in precedence order,
	// then the fleet-home fallback). Per-home scan failures fail the
	// build, as before.
	idx, errs, err := skillindex.Load(p)
	if err != nil {
		return nil, fmt.Errorf("resolve tracked dirs: %w", err)
	}
	if serr, ok := errs[idx.Store()]; ok {
		return nil, fmt.Errorf("scan canonical store: %w", serr)
	}
	if serr, ok := errs[idx.Fallback()]; ok {
		return nil, fmt.Errorf("scan fleet home: %w", serr)
	}
	for _, home := range idx.CustomHomes() {
		if home == idx.Fallback() {
			continue
		}
		if serr, ok := errs[home]; ok {
			return nil, fmt.Errorf("scan tracked dir %s: %w", home, serr)
		}
	}
	// Union with precedence tracked order > fleet-home > canonical: apply
	// lowest first so the highest-precedence source wins.
	skillsByName := make(map[string]scan.Skill)
	customHomeNames := map[string]bool{}
	homes := idx.Homes()
	for i := len(homes) - 1; i >= 0; i-- {
		home := homes[i]
		// Within one home the first directory wins a frontmatter-name
		// collision (the scan is sorted by directory); the home then
		// overwrites the lower-precedence homes applied before it.
		seenInHome := map[string]bool{}
		for _, s := range idx.Skills(home) {
			if seenInHome[s.Name] {
				continue
			}
			seenInHome[s.Name] = true
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
		return nil, fmt.Errorf("read skills lockfile: %w", err)
	}

	managed, err := harness.Managed(p)
	if err != nil {
		return nil, err
	}

	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.Name
	}
	reads := make([]harness.ReadResult, len(managed))
	linkToggle := make([]bool, len(managed))
	linkedNames := make([]map[string]bool, len(managed))
	for i, a := range managed {
		read, err := a.Read(names)
		if err != nil {
			return nil, fmt.Errorf("read %s config: %w", a.Harness(), err)
		}
		reads[i] = read
		// A link-toggleable harness reaches a custom skill only through
		// its managed link, so for customs the link's presence is the
		// enablement. Canonical skills stay on: it reads the store natively.
		if harness.LinkToggleable(a) {
			linkToggle[i] = true
			set := make(map[string]bool, len(read.Linked))
			for _, n := range read.Linked {
				set[n] = true
			}
			linkedNames[i] = set
		}
	}

	harnessNames := make([]string, len(managed))
	for i, a := range managed {
		harnessNames[i] = string(a.Harness())
	}

	// Checkable managed skills carry their lockfile provenance; customs
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

	dirOf := make(map[string]string, len(skills))
	rows := make([]SkillRow, len(skills))
	for i, s := range skills {
		dirOf[s.Name] = s.Dir
		states := make(map[string]string, len(managed))
		for j := range managed {
			state := reads[j].States[s.Name]
			if customHomeNames[s.Name] && linkToggle[j] {
				if linkedNames[j][s.Name] {
					state = harness.StateOn
				} else {
					state = harness.StateAbsent
				}
			}
			if state == "" {
				state = harness.StateOn // adapters report every name; be defensive anyway
			}
			states[harnessNames[j]] = string(state)
		}

		row := SkillRow{
			Name:        s.Name,
			Description: s.Description,
			States:      states,
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

	return &draft{
		report:  &Report{Harnesses: harnessNames, Skills: rows},
		entries: entries,
		dirOf:   dirOf,
	}, nil
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
