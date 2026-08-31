// Package sync makes each installed harness's config match the state
// file. It runs on every fleet command: read each config, diff against
// the state, repair what fleet owns, and flag what it doesn't recognize
// instead of touching it. It also removes redundant per-agent links —
// symlinks into the canonical store in harnesses that scan the store
// natively — so the skills CLI's link spam stops accumulating. Sync never
// edits the state file.
package sync

import (
	"fmt"
	"os"

	"github.com/zacong/fleet/internal/harness"
	"github.com/zacong/fleet/internal/paths"
	"github.com/zacong/fleet/internal/state"
)

// Report is one harness's sync outcome: what changed and what was left
// alone. Harnesses with nothing to say are omitted.
type Report struct {
	Harness string
	Changed []harness.Change
	Flags   []harness.Flag
	// Removed lists the redundant links deleted from this harness's
	// skills dir. Nothing else in the dir is ever touched.
	Removed []harness.Entry
}

// Empty reports whether the report has nothing to tell the user.
func (r Report) Empty() bool {
	return len(r.Changed) == 0 && len(r.Flags) == 0 && len(r.Removed) == 0
}

// Run projects the state file into every installed harness that can be
// written to, and cleans up redundant links. The state file is read fresh
// each time so commands that just updated it sync their own change.
func Run(p *paths.Paths) ([]Report, error) {
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		return nil, err
	}

	reports := map[string]*Report{}
	var order []string
	reportFor := func(h harness.Harness) *Report {
		r := reports[string(h)]
		if r == nil {
			r = &Report{Harness: string(h)}
			reports[string(h)] = r
			order = append(order, string(h))
		}
		return r
	}

	// Redundant links first: they are pure filesystem hygiene, independent
	// of what the configs say. Only links provably resolving into the
	// canonical store in a native scanner are removed — a link pointing
	// anywhere else (a repo custom skill, the user's own link) survives
	// untouched, as does every non-symlink entry.
	// Claude is not a native scanner: its links are its only discovery
	// path, so none of them are redundant by construction.
	for _, d := range harness.SkillDirs(p) {
		if !d.NativeScan {
			continue
		}
		entries, err := harness.ScanSkillDir(d, p.SkillsStore())
		if err != nil {
			return nil, fmt.Errorf("scan %s skills dir: %w", d.Harness, err)
		}
		for _, e := range entries {
			if e.Class != harness.EntryRedundant {
				continue // foreign, untracked, or broken: never fleet's to delete
			}
			if err := os.Remove(e.Path); err != nil {
				return nil, fmt.Errorf("remove redundant link %s: %w", e.Path, err)
			}
			r := reportFor(d.Harness)
			r.Removed = append(r.Removed, e)
		}
	}

	for _, a := range harness.All(p) {
		if !a.Installed() || !a.CanProject() {
			continue
		}
		// The state schema is sparse: every entry is a disable.
		var writes []harness.SkillWrite
		for _, name := range st.Disabled(string(a.Harness())) {
			writes = append(writes, harness.SkillWrite{Name: name, State: harness.StateOff})
		}
		rep, err := a.Project(writes)
		if err != nil {
			return nil, fmt.Errorf("sync %s: %w", a.Harness(), err)
		}
		if !rep.Empty() {
			r := reportFor(a.Harness())
			r.Changed = append(r.Changed, rep.Changed...)
			r.Flags = append(r.Flags, rep.Flags...)
		}
	}

	out := make([]Report, 0, len(order))
	for _, h := range order {
		out = append(out, *reports[h])
	}
	return out, nil
}
