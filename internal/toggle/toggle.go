// Package toggle holds the one write path every fleet face shares: record
// per-skill, per-harness enablement in the state file, then project it
// into the harnesses' own configs. The CLI's on/off verbs and the TUI's
// staged apply both call Apply — neither keeps its own copy of the
// sequence.
//
// Apply does not validate skill names: enabling is deliberately lenient
// (it also cleans up entries for uninstalled skills), and callers that
// need strictness check the canonical store themselves before disabling.
package toggle

import (
	"fmt"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/skillindex"
	"github.com/zzacong/fleet/internal/state"
	fleetsync "github.com/zzacong/fleet/internal/sync"
)

// Toggle is one desired enablement: skill Name on or off for Harness.
type Toggle struct {
	Name    string
	Harness string
	On      bool
}

// Projected is what writing one harness's "on" toggles changed.
type Projected struct {
	Harness harness.Harness
	Report  harness.WriteReport
}

// Apply records the toggles in the state file — the single source of
// truth — and projects them:
//
//   - "on" writes strip fleet's own markers directly, before ambient sync
//     runs: the state entry is already gone by then, so sync would
//     otherwise flag the still-present markers as untracked.
//   - "off" writes need no direct step — sync projects them.
//   - sync then runs anyway, repairing any other drift it finds.
//
// A toggle naming a harness with no config lever is still recorded when
// the harness reaches the skill through a managed link (Bob or Cursor on a
// custom skill): there the link's presence is the enablement, and sync
// projects the state by creating or removing it. Every other no-lever
// toggle is skipped: there is nothing to record or write, and callers
// surface that limitation themselves. It returns how many toggles were
// recorded, what the direct "on" writes changed, and sync's reports.
func Apply(p *paths.Paths, toggles []Toggle) (int, []Projected, []fleetsync.Report, error) {
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		return 0, nil, nil, err
	}
	idx, _, err := skillindex.Load(p)
	if err != nil {
		return 0, nil, nil, err
	}

	applied := 0
	onWrites := map[string][]harness.SkillWrite{}
	for _, t := range toggles {
		a := adapterFor(p, t.Harness)
		switch {
		case a == nil:
			continue
		case a.CanProject():
			if t.On {
				st.SetEnabled(t.Name, t.Harness)
				onWrites[t.Harness] = append(onWrites[t.Harness], harness.SkillWrite{Name: t.Name, State: harness.StateOn})
			} else {
				st.SetDisabled(t.Name, t.Harness)
			}
			applied++
		case harness.LinkToggleable(a) && idx.IsCustom(t.Name):
			// The managed link is this harness's only lever over a custom
			// skill; sync creates it for "on" and removes it for "off".
			if t.On {
				st.SetEnabled(t.Name, t.Harness)
			} else {
				st.SetDisabled(t.Name, t.Harness)
			}
			applied++
		}
	}
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		return 0, nil, nil, fmt.Errorf("save state: %w", err)
	}

	var projected []Projected
	for _, a := range harness.Installed(p) {
		writes := onWrites[string(a.Harness())]
		if len(writes) == 0 {
			continue
		}
		rep, err := a.Project(writes)
		if err != nil {
			return applied, nil, nil, fmt.Errorf("enable in %s: %w", a.Harness(), err)
		}
		projected = append(projected, Projected{Harness: a.Harness(), Report: rep})
	}

	reports, err := fleetsync.Run(p)
	if err != nil {
		return applied, projected, nil, err
	}
	return applied, projected, reports, nil
}

// adapterFor returns the named harness's adapter, or nil for an unknown
// name.
func adapterFor(p *paths.Paths, name string) harness.Adapter {
	for _, a := range harness.All(p) {
		if string(a.Harness()) == name {
			return a
		}
	}
	return nil
}
