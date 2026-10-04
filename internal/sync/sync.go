// Package sync makes each installed harness's config match the state
// file. It runs on every fleet command: read each config, diff against
// the state, repair what fleet owns, and flag what it doesn't recognize
// instead of touching it. It makes every custom home visible (linking every
// skill it holds into every installed harness) so customs stay
// discoverable without an adopt or pull run. It also removes redundant
// per-agent links — symlinks into the canonical store in harnesses that
// scan the store natively — so the skills CLI's link spam stops
// accumulating. Sync never edits the state file.
package sync

import (
	"fmt"
	"os"

	"github.com/zzacong/fleet/internal/customs"
	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/skillindex"
	"github.com/zzacong/fleet/internal/state"
)

// Report is one harness's sync outcome: what changed and what was left
// alone. Harnesses with nothing to say are omitted.
type Report struct {
	Harness string
	Changed []harness.Change
	Flags   []harness.Flag
	// Linked lists the managed custom-skill links created or repointed
	// this run, one entry per harness per custom skill.
	Linked []harness.LinkResult
	// Unlinked lists managed custom-skill links removed this run because
	// the state disables the skill on a link-toggleable harness (Bob,
	// Cursor), where the link is the only disable lever.
	Unlinked []harness.UnlinkResult
	// Removed lists the redundant links deleted from this harness's
	// skills dir. Nothing else in the dir is ever touched.
	Removed []harness.Entry
	// Cleaned lists legacy fleet-owned config entries the one-time
	// migration removed this run: old collection paths and custom
	// off-entries from the pre-link model.
	Cleaned []harness.LegacyRemoval
}

// Empty reports whether the report has nothing to tell the user.
func (r Report) Empty() bool {
	return len(r.Changed) == 0 && len(r.Flags) == 0 &&
		len(r.Linked) == 0 && len(r.Unlinked) == 0 &&
		len(r.Removed) == 0 && len(r.Cleaned) == 0
}

// Run projects the state file into every installed harness that can be
// written to, and cleans up redundant links. The state file is read fresh
// each time so commands that just updated it sync their own change.
func Run(p *paths.Paths) ([]Report, error) {
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		return nil, err
	}
	idx, _, err := skillindex.Load(p)
	if err != nil {
		return nil, fmt.Errorf("resolve tracked repos: %w", err)
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

	// One-time legacy cleanup: remove the collection paths and custom
	// off-entries the old wiring model left behind, before projection
	// consults the configs. The scope is the current custom homes and
	// names, so anything fleet did not write is left alone. Idempotent: a
	// second sync finds nothing to remove.
	cleaned, err := harness.CleanLegacy(p, harness.LegacyScope{
		Homes:    idx.CustomHomes(),
		IsCustom: idx.IsCustom,
	})
	if err != nil {
		return nil, fmt.Errorf("clean legacy entries: %w", err)
	}
	for _, rem := range cleaned {
		r := reportFor(rem.Harness)
		r.Cleaned = append(r.Cleaned, rem)
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

	// Custom visibility: every scanned custom home — each tracked
	// collection plus the fleet-home fallback — is linked into every
	// installed harness, so customs stay discoverable without an adopt or
	// pull run. Idempotent: a home that is already visible reports nothing.
	vis, err := customs.EnsureVisible(p)
	if err != nil {
		return nil, fmt.Errorf("make customs visible: %w", err)
	}
	for _, l := range vis.Linked {
		r := reportFor(l.Harness)
		r.Linked = append(r.Linked, l)
	}

	// A link-toggleable harness (Bob, Cursor) reaches a custom skill only
	// through its managed link, so a recorded disable is projected by
	// removing that link. MakeVisible already skipped re-creating it; this
	// cleans up a link that predates the disable.
	unlinked, err := customs.PruneHiddenLinks(p)
	if err != nil {
		return nil, fmt.Errorf("hide disabled customs: %w", err)
	}
	for _, u := range unlinked {
		r := reportFor(u.Harness)
		r.Unlinked = append(r.Unlinked, u)
	}

	for _, a := range harness.Installed(p) {
		if !a.CanProject() {
			continue
		}
		// The state schema is sparse: every entry is a disable. On a
		// link-toggleable harness a custom skill's lever is its managed
		// link, so its disable never becomes a config off-entry; canonical
		// names still project.
		linkCustom := harness.LinkToggleable(a)
		var writes []harness.SkillWrite
		for _, name := range st.Disabled(string(a.Harness())) {
			if linkCustom && idx.IsCustom(name) {
				continue
			}
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
