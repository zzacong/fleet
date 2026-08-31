// Package doctor is the read-only report of what's wrong across the home:
// redundant links, broken symlinks, unknown entries in harness skill dirs,
// missing directories, and manual config edits that disagree with the state
// file. Doctor reports; Sync fixes. The one exception is the manual-edit
// resolution the user is prompted for: keep adopts the edit into the state
// file, restore syncs the state back into the config. Nothing is ever
// overwritten or deleted silently.
package doctor

import (
	"fmt"
	"os"
	"sort"

	"github.com/zacong/fleet/internal/harness"
	"github.com/zacong/fleet/internal/paths"
	"github.com/zacong/fleet/internal/scan"
	"github.com/zacong/fleet/internal/state"
)

// Kind classifies one finding.
type Kind string

const (
	// KindRedundantLink: a per-agent link the canonical store already
	// covers; sync removes it on the next command.
	KindRedundantLink Kind = "redundant-link"
	// KindBrokenLink: a symlink whose target is missing or that loops.
	KindBrokenLink Kind = "broken-link"
	// KindUnknownEntry: an entry in a harness skills dir fleet doesn't
	// own — reported, never touched.
	KindUnknownEntry Kind = "unknown-entry"
	// KindManualEdit: a config disable fleet can't represent or manage
	// (a pattern or blanket rule). Reported, never prompted.
	KindManualEdit Kind = "manual-edit"
	// KindDrift: the state and the config disagree in a way sync will
	// resolve on the next command.
	KindDrift Kind = "drift"
	// KindMissingDir: a directory the workflow depends on is absent.
	KindMissingDir Kind = "missing-dir"
	// KindBrokenConfig: a config file that doesn't parse; sync will fail
	// on it until it's fixed.
	KindBrokenConfig Kind = "broken-config"
)

// Finding is one observed problem that has no interactive resolution: it
// either needs fleet's machinery (sync), the user's hands, or nothing.
type Finding struct {
	Kind Kind
	// Harness scopes the finding; empty when it concerns the home itself
	// (the canonical store).
	Harness string
	// Skill names the skill involved; empty when none.
	Skill   string
	Message string
}

// Conflict is a manual edit that disagrees with the state file, offered to
// the user as keep-my-change vs restore.
type Conflict struct {
	Harness string
	Skill   string
	// ConfigDisables selects the disagreement's direction:
	//
	//   true  — the config disables the skill, the state doesn't. Keep
	//           records the disable in the state; restore removes it from
	//           the config.
	//   false — the state disables the skill, the config doesn't (the
	//           entry was deleted by hand, or the skills CLI cleaned it).
	//           Keep records the skill as enabled; restore re-disables it.
	ConfigDisables bool
	// Message states the disagreement.
	Message string
}

// Report is everything Analyze found.
type Report struct {
	Findings  []Finding
	Conflicts []Conflict
}

// Empty reports whether the home is clean.
func (r Report) Empty() bool { return len(r.Findings) == 0 && len(r.Conflicts) == 0 }

// Analyze inspects the home and reports every problem it finds. It writes
// nothing.
func Analyze(p *paths.Paths) (Report, error) {
	rep := Report{}

	if _, err := os.Stat(p.SkillsStore()); os.IsNotExist(err) {
		rep.Findings = append(rep.Findings, Finding{
			Kind: KindMissingDir,
			Message: fmt.Sprintf("the canonical store %s does not exist — no skills are installed",
				p.SkillsStore()),
		})
	}

	// Links: what's in each installed harness's skills dir.
	for _, d := range harness.SkillDirs(p) {
		entries, err := harness.ScanSkillDir(d, p.SkillsStore())
		if err != nil {
			return Report{}, fmt.Errorf("scan %s skills dir: %w", d.Harness, err)
		}
		if len(entries) == 0 && !d.NativeScan {
			if _, err := os.Stat(d.Path); os.IsNotExist(err) {
				rep.Findings = append(rep.Findings, Finding{
					Kind:    KindMissingDir,
					Harness: string(d.Harness),
					Message: fmt.Sprintf("%s discovers skills only through links in %s, which does not exist — it sees no linked skills",
						d.Harness, d.Path),
				})
			}
		}
		for _, e := range entries {
			f, ok := linkFinding(e)
			if ok {
				rep.Findings = append(rep.Findings, f)
			}
		}
	}

	conflicts, findings, err := analyzeConfigs(p)
	if err != nil {
		return Report{}, err
	}
	rep.Findings = append(rep.Findings, findings...)
	rep.Conflicts = conflicts

	return rep, nil
}

// linkFinding turns a classified skills-dir entry into a finding. Entries
// that are exactly as they should be (claude's live store links) produce
// none.
func linkFinding(e harness.Entry) (Finding, bool) {
	f := Finding{Harness: string(e.Harness), Skill: e.Name}
	target := ""
	if e.Target != "" {
		target = fmt.Sprintf(" → %s", e.Target)
	}
	switch e.Class {
	case harness.EntryRedundant:
		f.Kind = KindRedundantLink
		f.Message = fmt.Sprintf("link %q%s — %s; sync removes it", e.Name, target, e.Reason)
	case harness.EntryBroken:
		f.Kind = KindBrokenLink
		f.Message = fmt.Sprintf("link %q%s — %s", e.Name, target, e.Reason)
	case harness.EntryForeign, harness.EntryUntracked:
		f.Kind = KindUnknownEntry
		f.Message = fmt.Sprintf("%q — %s", e.Name, e.Reason)
	default:
		return Finding{}, false
	}
	return f, true
}

// analyzeConfigs compares each writable harness's own config against the
// state file. Configs that don't parse become findings instead of failing
// the whole checkup.
func analyzeConfigs(p *paths.Paths) ([]Conflict, []Finding, error) {
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		return nil, nil, err
	}

	skills, err := scan.ScanStore(p.SkillsStore())
	if err != nil {
		return nil, nil, fmt.Errorf("scan canonical store: %w", err)
	}

	// The universe of skill names the configs can meaningfully talk about:
	// everything installed, everything the state knows, and — added per
	// adapter from the read itself — everything a config disables.
	storeNames := make([]string, 0, len(skills))
	for _, s := range skills {
		storeNames = append(storeNames, s.Name)
	}
	universe := map[string]bool{}
	for _, name := range st.Universe(storeNames) {
		universe[name] = true
	}

	var conflicts []Conflict
	var findings []Finding
	for _, a := range harness.Installed(p) {
		if !a.CanProject() {
			continue // no config lever: nothing to disagree with
		}
		h := string(a.Harness())

		read, err := a.Read(sortedNames(universe))
		if err != nil {
			findings = append(findings, brokenConfigFinding(h, err))
			continue
		}

		// Config disable entries fleet never sees in the store or state —
		// stale disables for uninstalled skills, say — join the universe.
		// Their effective state comes from a second read over the grown
		// universe; the enumeration itself doesn't depend on the names.
		grown := len(universe)
		for _, name := range read.Disables {
			universe[name] = true
		}
		if len(universe) > grown {
			read, err = a.Read(sortedNames(universe))
			if err != nil {
				findings = append(findings, brokenConfigFinding(h, err))
				continue
			}
		}

		exact := map[string]bool{}
		for _, name := range read.Disables {
			exact[name] = true
		}
		for _, name := range sortedNames(universe) {
			stateOff := st.IsDisabled(name, h)
			cfgState := read.States[name]
			switch {
			case !stateOff && exact[name] && cfgState != harness.StateOn:
				// An exact disable entry the state doesn't track: live
				// (the harness reports off), or stale-but-present (claude's
				// override for a skill it can no longer discover). The user
				// decides.
				conflicts = append(conflicts, Conflict{
					Harness:        h,
					Skill:          name,
					ConfigDisables: true,
					Message: fmt.Sprintf("%q is disabled in the %s config, but fleet's state has it enabled",
						name, h),
				})
			case cfgState == harness.StateOff && !stateOff:
				// Disabled by something fleet can't represent or own.
				findings = append(findings, Finding{
					Kind:    KindManualEdit,
					Harness: h,
					Skill:   name,
					Message: fmt.Sprintf("%q is disabled by an entry fleet doesn't manage (a pattern or blanket rule) — edit the %s config by hand if that's wrong",
						name, h),
				})
			case stateOff && cfgState == harness.StateOn:
				conflicts = append(conflicts, Conflict{
					Harness:        h,
					Skill:          name,
					ConfigDisables: false,
					Message: fmt.Sprintf("%q is disabled in fleet's state, but the %s config no longer disables it — sync re-disables it on the next command",
						name, h),
				})
			case stateOff && cfgState == harness.StateAbsent:
				// Claude discovers through links; without one there is
				// nothing for sync to write, so this is a report, not a
				// prompt.
				findings = append(findings, Finding{
					Kind:    KindDrift,
					Harness: h,
					Skill:   name,
					Message: fmt.Sprintf("%q is disabled in fleet's state, but %s cannot discover it (no link in %s) — the disable is moot until the link returns",
						name, h, skillsDirFor(p, a.Harness())),
				})
			}
		}
	}
	return conflicts, findings, nil
}

// Resolve applies the chosen resolution for one conflict. keep records the
// manual edit as the new intent in the state file; restore re-projects the
// recorded intent into the harness config. Restore returns the write
// report so the caller can show what changed — including flags for
// interference fleet can't remove.
func Resolve(p *paths.Paths, c Conflict, keep bool) (harness.WriteReport, error) {
	if !keep {
		return restoreConflict(p, c)
	}

	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		return harness.WriteReport{}, err
	}
	if c.ConfigDisables {
		st.SetDisabled(c.Skill, c.Harness)
	} else {
		st.SetEnabled(c.Skill, c.Harness)
	}
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		return harness.WriteReport{}, fmt.Errorf("save state: %w", err)
	}
	return harness.WriteReport{}, nil
}

// brokenConfigFinding is the finding for a harness config fleet can't
// read at all — one per harness, wherever the read happens.
func brokenConfigFinding(harnessName string, err error) Finding {
	return Finding{
		Kind:    KindBrokenConfig,
		Harness: harnessName,
		Message: fmt.Sprintf("the %s config can't be read (%v) — sync will fail until it's fixed", harnessName, err),
	}
}

// restoreConflict projects the state's intent back into the harness config.
func restoreConflict(p *paths.Paths, c Conflict) (harness.WriteReport, error) {
	var adapter harness.Adapter
	for _, a := range harness.All(p) {
		if string(a.Harness()) == c.Harness {
			adapter = a
			break
		}
	}
	if adapter == nil || !adapter.CanProject() {
		return harness.WriteReport{}, fmt.Errorf("%s has no config fleet can write", c.Harness)
	}

	desired := harness.StateOn
	if !c.ConfigDisables {
		desired = harness.StateOff
	}
	rep, err := adapter.Project([]harness.SkillWrite{{Name: c.Skill, State: desired}})
	if err != nil {
		return harness.WriteReport{}, fmt.Errorf("restore %q in %s: %w", c.Skill, c.Harness, err)
	}
	return rep, nil
}

// skillsDirFor names the skills dir a harness discovers links through (for
// messages only).
func skillsDirFor(p *paths.Paths, h harness.Harness) string {
	for _, d := range harness.SkillDirs(p) {
		if d.Harness == h {
			return d.Path
		}
	}
	return string(h) + " skills dir"
}

func sortedNames(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
