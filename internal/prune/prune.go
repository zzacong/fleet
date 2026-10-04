// Package prune owns the pure `skill prune` domain operation: find the
// stale disable rules and state entries for skills installed nowhere, and
// remove them through the same write paths the enable verbs use. A config
// rule is removed only when it is fleet's own exact disable shape — the
// adapter's StateOn projection strips it and leaves every pattern, blanket,
// extra-key, or foreign shape in place. A state entry is removed through
// the state enable path, which drops the skill entry once nothing remains.
// The operation refuses to act when the skill scan is incomplete, naming
// the homes that blocked it. Sync runs at the CLI layer, not here.
package prune

import (
	"fmt"
	"os"
	"slices"
	"sort"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/skillindex"
	"github.com/zzacong/fleet/internal/state"
)

// Options scopes one prune run. Harnesses is the already-validated
// --harness filter (empty means every harness); ConfigOnly and StateOnly
// select one axis; Apply removes instead of only reporting.
type Options struct {
	Harnesses  []string
	ConfigOnly bool
	StateOnly  bool
	Apply      bool
}

// ConfigRemoval is one stale fleet-owned disable rule removed from a
// harness config.
type ConfigRemoval struct {
	Harness string
	Skill   string
}

// StateRemoval is one stale state disable removed.
type StateRemoval struct {
	Harness string
	Skill   string
}

// Skip is a stale disable prune left alone because it is not fleet's own
// exact shape.
type Skip struct {
	Harness string
	Skill   string
	Reason  string
}

// Report is what one prune run found and did.
type Report struct {
	Config  []ConfigRemoval
	State   []StateRemoval
	Skipped []Skip
	// Blocked names the homes that made the skill scan incomplete. When
	// non-empty, nothing was removed.
	Blocked []string
}

// Empty reports whether there is nothing to remove. A run that only
// skipped foreign shapes is empty too.
func (r Report) Empty() bool {
	return len(r.Config) == 0 && len(r.State) == 0
}

// Run finds the stale config rules and state entries for skills installed
// nowhere and, when opts.Apply, removes them. An incomplete scan removes
// nothing and returns the blocking homes in Report.Blocked, so a name that
// looks uninstalled can never be pruned while an unscanned home might still
// hold it.
func Run(p *paths.Paths, opts Options) (Report, error) {
	idx, _, err := skillindex.Load(p)
	if err != nil {
		return Report{}, err
	}
	if !idx.Complete() {
		return Report{Blocked: blockedHomes(idx)}, nil
	}

	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		return Report{}, err
	}

	filter := harnessFilter(opts.Harnesses)
	rep := Report{}
	if !opts.StateOnly {
		if err := pruneConfig(p, st, idx, filter, opts.Apply, &rep); err != nil {
			return Report{}, err
		}
	}
	if !opts.ConfigOnly {
		if err := pruneState(p, st, idx, filter, opts.Apply, &rep); err != nil {
			return Report{}, err
		}
	}
	return rep, nil
}

// pruneConfig removes, for every projectable installed harness in scope,
// the stale rules for skills the state disables and the index cannot find.
// The removal runs through the adapter's StateOn projection, so only
// fleet's own exact shapes go; a pattern, blanket, or foreign shape is
// reported as skipped. A dry run previews the same report without writing.
func pruneConfig(p *paths.Paths, st *state.File, idx *skillindex.Index, filter map[string]bool, apply bool, rep *Report) error {
	for _, a := range harness.Installed(p) {
		h := string(a.Harness())
		if !a.CanProject() || !inScope(filter, h) {
			continue
		}
		var names []string
		for _, name := range st.Disabled(h) {
			if !idx.IsInstalled(name) {
				names = append(names, name)
			}
		}
		if len(names) == 0 {
			continue
		}
		writes := make([]harness.SkillWrite, len(names))
		for i, name := range names {
			writes[i] = harness.SkillWrite{Name: name, State: harness.StateOn, DryRun: !apply}
		}
		wr, err := a.Project(writes)
		if err != nil {
			return fmt.Errorf("prune %s: %w", h, err)
		}
		for _, c := range wr.Changed {
			rep.Config = append(rep.Config, ConfigRemoval{Harness: h, Skill: c.Skill})
		}
		for _, f := range wr.Flags {
			if !slices.Contains(names, f.Skill) {
				continue // ambient flag about a skill prune isn't touching
			}
			rep.Skipped = append(rep.Skipped, Skip{Harness: h, Skill: f.Skill, Reason: f.Message})
		}
	}
	return nil
}

// pruneState removes every in-scope state disable for a skill the index
// cannot find, link-toggleable harnesses included. Each removal runs
// through the state enable path, so the skill entry disappears once nothing
// remains and a harness value fleet doesn't recognize is left in place. A
// dry run reports without saving.
func pruneState(p *paths.Paths, st *state.File, idx *skillindex.Index, filter map[string]bool, apply bool, rep *Report) error {
	type pair struct{ harness, skill string }
	var pairs []pair
	for _, a := range harness.All(p) {
		h := string(a.Harness())
		if !inScope(filter, h) {
			continue
		}
		for _, name := range st.Disabled(h) {
			if idx.IsInstalled(name) {
				continue
			}
			pairs = append(pairs, pair{harness: h, skill: name})
		}
	}
	if apply && len(pairs) > 0 {
		for _, pr := range pairs {
			st.SetEnabled(pr.skill, pr.harness)
		}
		if err := state.Save(p.FleetStateFile(), st); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}
	for _, pr := range pairs {
		rep.State = append(rep.State, StateRemoval{Harness: pr.harness, Skill: pr.skill})
	}
	return nil
}

// blockedHomes names every home that made the scan incomplete. The
// canonical store's absence is not part of the index's BlockedHomes —
// doctor reports it as a missing directory — but prune must still fail
// closed on it, so it is added here.
func blockedHomes(idx *skillindex.Index) []string {
	blocked := idx.BlockedHomes()
	if !dirExists(idx.Store()) {
		blocked = append(blocked, idx.Store())
		sort.Strings(blocked)
	}
	return blocked
}

// harnessFilter turns the --harness list into a lookup; empty means no
// filter, every harness in scope.
func harnessFilter(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	filter := make(map[string]bool, len(names))
	for _, name := range names {
		filter[name] = true
	}
	return filter
}

// inScope reports whether a harness passes the filter.
func inScope(filter map[string]bool, harnessName string) bool {
	return filter == nil || filter[harnessName]
}

// dirExists reports whether path is an existing directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
