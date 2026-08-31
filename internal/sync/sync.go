// Package sync makes each installed harness's config match the state
// file. It runs on every fleet command: read each config, diff against
// the state, repair what fleet owns, and flag what it doesn't recognize
// instead of touching it. Sync never edits the state file.
package sync

import (
	"fmt"

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
}

// Empty reports whether the report has nothing to tell the user.
func (r Report) Empty() bool { return len(r.Changed) == 0 && len(r.Flags) == 0 }

// Run projects the state file into every installed harness that can be
// written to. The state file is read fresh each time so commands that just
// updated it sync their own change.
func Run(p *paths.Paths) ([]Report, error) {
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		return nil, err
	}

	var reports []Report
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
			reports = append(reports, Report{
				Harness: string(a.Harness()),
				Changed: rep.Changed,
				Flags:   rep.Flags,
			})
		}
	}
	return reports, nil
}
