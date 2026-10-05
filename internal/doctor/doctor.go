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
	"path/filepath"
	"sort"
	"strings"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/scan"
	"github.com/zzacong/fleet/internal/skillindex"
	"github.com/zzacong/fleet/internal/state"
	"github.com/zzacong/fleet/internal/trackedset"
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
	// KindDoublePresence: a skill name that exists in more than one of the
	// skill homes (canonical store, fleet-home fallback, tracked dirs) —
	// the harnesses that read more than one would see it twice. Only the
	// user's hands can remove a copy.
	KindDoublePresence Kind = "double-presence"
	// KindUnscannedAdoptTarget: the configured adopt-target collection dir
	// is not one of the scanned custom homes, so skills adopted there
	// won't appear in ls. Adopt still proceeds; the footgun stays visible.
	KindUnscannedAdoptTarget Kind = "unscanned-adopt-target"
	// KindStaleLock: a skills CLI lockfile entry for a skill that now
	// lives in a custom home (fleet-home fallback or tracked collection) —
	// the skills CLI would keep trying to update it. Fleet reads the
	// lockfile only; it never writes it.
	KindStaleLock Kind = "stale-lock"
	// KindStaleConfig: a fleet-owned disable rule in a harness config for
	// a skill installed nowhere — a leftover prune removes.
	KindStaleConfig Kind = "stale-config"
	// KindStaleState: a state entry for a skill installed nowhere —
	// dormant intent prune removes, losing the disable-on-reinstall
	// behavior.
	KindStaleState Kind = "stale-state"
	// KindIncompleteScan: a skill home is missing or unreadable, so the
	// "installed nowhere" answer can't be trusted and stale findings are
	// suppressed. Names the home that blocked the scan.
	KindIncompleteScan Kind = "incomplete-scan"
	// KindTrackedDirMissing: an explicit skillsDirs entry that is missing
	// from disk or is not a directory — a registration the scan can't turn
	// into skills. Only the user can fix the config or the disk.
	KindTrackedDirMissing Kind = "tracked-dir-missing"
	// KindTrackedDirEmpty: an explicit skillsDirs entry that is a directory
	// but holds no skills — a registration that wires nothing into any
	// harness.
	KindTrackedDirEmpty Kind = "tracked-dir-empty"
	// KindTrackedSetOverlap: a hand-edited skillsDirs list that repeats an
	// entry or nests one tracked dir inside another, making precedence
	// ambiguous. Add refuses both; only a manual edit can produce them.
	KindTrackedSetOverlap Kind = "tracked-set-overlap"
)

// DriftReason names why a KindDrift finding exists, so the report can group
// findings that share one cause and one fix instead of repeating a full
// sentence per skill.
type DriftReason string

const (
	// DriftEnabledUnlinked: the state leaves a custom skill enabled but the
	// link-toggleable harness has no managed link; sync links it.
	DriftEnabledUnlinked DriftReason = "enabled-unlinked"
	// DriftDisabledLinked: the state disables a custom skill but its managed
	// link is still present; sync removes the link.
	DriftDisabledLinked DriftReason = "disabled-linked"
	// DriftDisabledUnlinked: the state disables a skill the harness cannot
	// discover, so the disable is moot until the link returns.
	DriftDisabledUnlinked DriftReason = "disabled-unlinked"
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
	// Path is the filesystem path for link findings; empty otherwise.
	Path string
	// Reason identifies the drift direction when Kind is KindDrift; empty
	// otherwise. The CLI groups drift findings by harness and reason so the
	// shared cause and fix print once per group.
	Reason DriftReason
	// Dir is the harness skills directory a drift finding concerns (the link
	// that is missing or stale); empty otherwise.
	Dir string
	// Homes lists the scanned home (collection dir) holding each copy of a
	// double-presence finding, in source order. The CLI groups collisions
	// that share the same homes and prints the shared explanation once.
	// Empty otherwise.
	Homes []string
	// Harnesses lists the installed native-scanning harnesses that would see
	// a double-present name twice, in column order. The CLI expands a finding
	// under each of them so the section groups by harness. Empty otherwise.
	Harnesses []string
	// Cause is the groupable explanation for findings whose section prints
	// one line per distinct cause and then the skill names (redundant links,
	// broken symlinks, unknown entries, manual edits). It never repeats the
	// harness (the column carries it) or the skill name (the name list
	// carries it). Empty otherwise.
	Cause string
	// Home is the scanned custom home (collection dir) a stale-lock finding
	// concerns, for grouping the section by home. Empty otherwise.
	Home string
	// HomeLabel describes that home in prose (e.g. "the explicit repo") in
	// the stale-lock section. Empty otherwise.
	HomeLabel string
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
	// Custom skills are recognized across every custom home (fallback and
	// tracked dirs) so managed links into any of them are filtered from
	// unknown entries.
	customHomes, customByDir, customByName := customLinkIndex(p)
	present := map[harness.Harness]map[string]bool{}
	managed := map[harness.Harness]map[string]bool{}
	for _, d := range harness.SkillDirs(p) {
		entries, err := harness.ScanSkillDir(d, p.SkillsStore())
		if err != nil {
			return Report{}, fmt.Errorf("scan %s skills dir: %w", d.Harness, err)
		}
		if present[d.Harness] == nil {
			present[d.Harness] = map[string]bool{}
			managed[d.Harness] = map[string]bool{}
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
			present[d.Harness][e.Name] = true
			if isManagedCustomLink(e, customHomes, customByDir, customByName) {
				managed[d.Harness][e.Name] = true
				continue
			}
			f, ok := linkFinding(e)
			if ok {
				rep.Findings = append(rep.Findings, f)
			}
		}
	}

	toggleFindings, err := analyzeLinkToggles(p, customByDir, present, managed)
	if err != nil {
		return Report{}, err
	}
	rep.Findings = append(rep.Findings, toggleFindings...)

	conflicts, findings, err := analyzeConfigs(p, customByName)
	if err != nil {
		return Report{}, err
	}
	rep.Findings = append(rep.Findings, findings...)
	rep.Conflicts = conflicts

	// Customs: each tracked dir against the canonical store and the
	// skills CLI lockfile.
	repoFindings, err := analyzeRepoSkills(p)
	if err != nil {
		return Report{}, err
	}
	rep.Findings = append(rep.Findings, repoFindings...)

	// The tracked set itself: entries a hand-edited config can leave
	// missing, empty, or overlapping.
	trackedFindings, err := analyzeTrackedSet(p)
	if err != nil {
		return Report{}, err
	}
	rep.Findings = append(rep.Findings, trackedFindings...)

	return rep, nil
}

// linkFinding turns a classified skills-dir entry into a finding. Entries
// that are exactly as they should be (claude's live store links) produce
// none.
func linkFinding(e harness.Entry) (Finding, bool) {
	f := Finding{Harness: string(e.Harness), Skill: e.Name, Path: e.Path, Cause: e.Cause}
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

// customLinkIndex maps every custom home — the fleet-home fallback and
// each tracked dir — for managed-link filtering: the home
// dirs, skills keyed by directory, and frontmatter names. Unreadable
// homes and sets only narrow the filter, mirroring the old best-effort
// scan.
func customLinkIndex(p *paths.Paths) ([]string, map[string]string, map[string]bool) {
	byDir := map[string]string{}
	byName := map[string]bool{}
	idx, _, err := skillindex.Load(p)
	if err != nil {
		// Tracked set unreadable: the fallback alone, scanned
		// best-effort.
		home := filepath.Clean(p.FleetHomeSkills())
		if skills, serr := scan.ScanStore(home); serr == nil {
			for _, s := range skills {
				byDir[s.Dir] = s.Name
				byName[s.Name] = true
			}
		}
		return []string{home}, byDir, byName
	}
	homes := idx.CustomHomes()
	for _, home := range homes {
		for _, s := range idx.Skills(home) {
			if _, ok := byDir[s.Dir]; !ok {
				byDir[s.Dir] = s.Name
			}
			byName[s.Name] = true
		}
	}
	return homes, byDir, byName
}

// isManagedCustomLink reports whether the entry is a managed custom-skill
// link pointing into any custom home (fallback or tracked dir). Such links
// are fleet's own discovery path for adopted skills, not unknown entries.
func isManagedCustomLink(e harness.Entry, homes []string, byDir map[string]string, byName map[string]bool) bool {
	if e.Class != harness.EntryForeign {
		return false
	}
	if e.Target == "" {
		return false
	}
	inside := false
	for _, home := range homes {
		if home == "" {
			continue
		}
		if pathInside(e.Target, home) || e.Target == home {
			inside = true
			break
		}
	}
	if !inside {
		return false
	}
	// The link name should match a custom skill (by dir or frontmatter name) in any home.
	if _, ok := byDir[e.Name]; ok {
		return true
	}
	if byName[e.Name] {
		return true
	}
	// Also handle the case where target's leaf matches the link name.
	if filepath.Base(e.Target) == e.Name {
		if _, err := os.Stat(e.Target); err == nil {
			return true
		}
	}
	return false
}

// pathInside reports whether path is inside dir. Local copy to avoid
// importing harness's unexported helper; lexical plus symlink-evaluated
// check mirrors harness.pathInside.
func pathInside(path, dir string) bool {
	if paths.IsUnder(path, dir) {
		return true
	}
	evaled, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	evalDir := dir
	if evaledDir, err := filepath.EvalSymlinks(dir); err == nil {
		evalDir = evaledDir
	}
	return paths.IsUnder(evaled, evalDir)
}

// RemoveBroken removes the broken symlink at the finding's path.
func RemoveBroken(f Finding) error {
	if f.Path == "" {
		return fmt.Errorf("broken link has no path")
	}
	if err := os.Remove(f.Path); err != nil {
		return fmt.Errorf("remove broken link %s: %w", f.Path, err)
	}
	return nil
}

// analyzeConfigs compares each writable harness's own config against the
// state file. Configs that don't parse become findings instead of failing
// the whole checkup. A custom skill on a link-toggleable harness is
// skipped: its lever is the managed link, so a config entry for it is not
// a disagreement the state can resolve (link drift is analyzeLinkToggles'
// business, and the legacy-entry migration removes stale entries).
func analyzeConfigs(p *paths.Paths, customByName map[string]bool) ([]Conflict, []Finding, error) {
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		return nil, nil, err
	}

	// The single scanner answers "is this skill installed" and whether that
	// answer can be trusted. Only a complete scan may call a skill
	// uninstalled; otherwise a name that looks gone might live in a home
	// that failed to scan.
	idx, _, err := skillindex.Load(p)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve skill homes: %w", err)
	}
	complete := idx.Complete()

	// The universe of skill names the configs can meaningfully talk about:
	// everything installed, everything the state knows, and — added per
	// adapter from the read itself — everything a config disables.
	universe := map[string]bool{}
	for _, name := range st.Universe(idx.InstalledNames()) {
		universe[name] = true
	}

	var conflicts []Conflict
	var findings []Finding
	if blocked := idx.BlockedHomes(); len(blocked) > 0 {
		findings = append(findings, Finding{
			Kind: KindIncompleteScan,
			Message: fmt.Sprintf("the skill scan is incomplete (%s), so stale disables are not reported — a skill that looks uninstalled might live in an unscanned home",
				strings.Join(blocked, ", ")),
		})
	}
	for _, a := range harness.Installed(p) {
		if !a.CanProject() {
			continue // no config lever: nothing to disagree with
		}
		h := string(a.Harness())
		linkCustom := harness.LinkToggleable(a)

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

		// exact is the fleet-owned removable subset, not every disable the
		// config carries: a shape the write side can't remove (a codex path
		// selector, an opencode V2 rule with extra keys) is a manual edit,
		// not a leftover prune can clear.
		exact := map[string]bool{}
		for _, name := range read.ExactDisables {
			exact[name] = true
		}
		for _, name := range sortedNames(universe) {
			if linkCustom && customByName[name] {
				continue // custom skills on a native scanner toggle by link
			}
			stateOff := st.IsDisabled(name, h)
			cfgState := read.States[name]
			if complete && !idx.IsInstalled(name) && stateOff {
				// Installed nowhere with a dormant state disable: a missing
				// config rule is expected, not drift, and a fleet-owned rule
				// still present is the stale-config leftover prune removes.
				// A config rule the state does not track is a manual edit,
				// not a stale leftover — prune's config axis only covers
				// state-disabled names — so it falls through to the existing
				// comparison below.
				if exact[name] && cfgState != harness.StateOn {
					findings = append(findings, Finding{
						Kind:    KindStaleConfig,
						Harness: h,
						Skill:   name,
						Message: fmt.Sprintf("%q is disabled in the %s config, but it is installed nowhere — run `fleet skill prune` to remove the leftover rule",
							name, h),
					})
				}
				continue
			}
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
					Cause:   "disabled by an entry fleet doesn't manage (a pattern or blanket rule) — edit the config by hand if that's wrong",
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
					Reason:  DriftDisabledUnlinked,
					Dir:     skillsDirFor(p, a.Harness()),
					Message: fmt.Sprintf("%q is disabled in fleet's state, but %s cannot discover it (no link in %s) — the disable is moot until the link returns",
						name, h, skillsDirFor(p, a.Harness())),
				})
			}
		}
	}
	findings = append(findings, staleStateFindings(p, st, idx, complete)...)
	return conflicts, findings, nil
}

// staleStateFindings reports every harness a state-disabled skill is
// installed nowhere for: dormant intent no config can act on. Only a
// complete scan may assert "installed nowhere", so an incomplete scan
// yields none. Each finding names the skill and harness and points at
// prune, warning that removing the entry loses the disable-on-reinstall
// behavior.
func staleStateFindings(p *paths.Paths, st *state.File, idx *skillindex.Index, complete bool) []Finding {
	if !complete {
		return nil
	}
	var findings []Finding
	for _, name := range st.Names() {
		if idx.IsInstalled(name) {
			continue
		}
		for _, a := range harness.All(p) {
			if !st.IsDisabled(name, string(a.Harness())) {
				continue
			}
			findings = append(findings, Finding{
				Kind:    KindStaleState,
				Harness: string(a.Harness()),
				Skill:   name,
				Message: fmt.Sprintf("%q is disabled for %s in fleet's state, but it is installed nowhere — run `fleet skill prune` to remove the state entry; pruning loses the disable-on-reinstall behavior",
					name, a.Harness()),
			})
		}
	}
	return findings
}

// analyzeLinkToggles compares the state with the managed custom links of
// the link-toggleable harnesses. For a custom skill the link
// is that harness's only path to it and therefore its enablement: a missing
// link for a skill the state leaves enabled (sync links it), or a managed
// link for a skill the state disables (sync removes it), is drift. Stored
// skills are exempt — those harnesses read the canonical store natively, so
// their link presence says nothing.
func analyzeLinkToggles(p *paths.Paths, customByDir map[string]string, present, managed map[harness.Harness]map[string]bool) ([]Finding, error) {
	if len(customByDir) == 0 {
		return nil, nil
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		return nil, err
	}

	toggle := map[harness.Harness]bool{}
	for _, a := range harness.Installed(p) {
		if harness.LinkToggleable(a) {
			toggle[a.Harness()] = true
		}
	}

	dirs := make([]string, 0, len(customByDir))
	for dir := range customByDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	var findings []Finding
	for _, d := range harness.SkillDirs(p) {
		if !toggle[d.Harness] {
			continue
		}
		h := string(d.Harness)
		for _, dir := range dirs {
			name := customByDir[dir]
			stateOff := st.IsDisabled(name, h)
			switch {
			case !stateOff && !present[d.Harness][dir]:
				findings = append(findings, Finding{
					Kind:    KindDrift,
					Harness: h,
					Skill:   name,
					Reason:  DriftEnabledUnlinked,
					Dir:     d.Path,
					Message: fmt.Sprintf("%q is enabled in fleet's state, but %s cannot discover it (no link in %s) — sync links it on the next command",
						name, h, d.Path),
				})
			case stateOff && managed[d.Harness][dir]:
				findings = append(findings, Finding{
					Kind:    KindDrift,
					Harness: h,
					Skill:   name,
					Path:    filepath.Join(d.Path, dir),
					Reason:  DriftDisabledLinked,
					Dir:     d.Path,
					Message: fmt.Sprintf("%q is disabled in fleet's state, but %s still has a link for it in %s — sync removes it on the next command",
						name, h, d.Path),
				})
			}
		}
	}
	return findings, nil
}

// joinWithAnd joins values into a readable list: "a", "a and b", or
// "a, b, and c". Empty for no values.
func joinWithAnd(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	case 2:
		return values[0] + " and " + values[1]
	}
	return strings.Join(values[:len(values)-1], ", ") + ", and " + values[len(values)-1]
}

// analyzeRepoSkills cross-checks every skill home — the canonical store
// (~/.agents/skills), each tracked collection dir (explicit list order), the
// unversioned fleet-home fallback — plus the skills CLI lockfile
// and the machine-local config. A name present in more than one home is
// double visibility: the installed native-scanning harnesses read the
// canonical store and the linked custom homes, so they would see the skill
// twice. A lock entry for a custom
// skill is stale provenance from before its adoption: the skills CLI keys
// updates by it and would keep touching a skill that moved. An adopt target
// outside the scanned homes is a warning: a visible footgun, never a silent
// failure. All of them need the user's hands; fleet never deletes a copy or
// edits the lockfile.
func analyzeRepoSkills(p *paths.Paths) ([]Finding, error) {
	type source struct {
		// key groups copies of one name; one finding lists every key.
		key string
		// label describes the home in messages, e.g. "the tracked dir".
		label string
		// skillsDir is the collection dir scanned.
		skillsDir string
		skills    []scan.Skill
	}
	var sources []source
	// Every home scanned once through the skill index; the tracked-set
	// call below stays for source keys and message labels only.
	idx, errs, err := skillindex.Load(p)
	if err != nil {
		return nil, fmt.Errorf("resolve tracked dirs: %w", err)
	}
	addSource := func(key, label, dir string) error {
		if serr, ok := errs[filepath.Clean(dir)]; ok {
			return fmt.Errorf("scan %s: %w", label, serr)
		}
		sources = append(sources, source{key: key, label: label, skillsDir: dir, skills: idx.Skills(dir)})
		return nil
	}
	if err := addSource("canonical", "the canonical store", p.SkillsStore()); err != nil {
		return nil, err
	}

	// Reserve the fixed homes so a tracked dir that overlaps one is not
	// counted twice, and skip duplicate skillsDirs entries a hand-edited
	// config carries (ticket 06 reports those separately).
	reserved := map[string]bool{
		filepath.Clean(p.SkillsStore()):     true,
		filepath.Clean(p.FleetHomeSkills()): true,
	}
	tracked, err := trackedset.List(p)
	if err != nil {
		return nil, fmt.Errorf("resolve tracked dirs: %w", err)
	}
	for _, dir := range tracked {
		if reserved[dir] {
			continue
		}
		reserved[dir] = true
		if _, ok := errs[dir]; ok {
			// An unscannable tracked dir (a file, an unreadable path) is
			// reported by analyzeTrackedSet and named by the incomplete-scan
			// finding; skip it as a double-presence source rather than
			// failing the whole checkup.
			continue
		}
		if err := addSource("tracked:"+dir, "the tracked dir", dir); err != nil {
			return nil, err
		}
	}
	if err := addSource("fleet-home", "the fleet home", p.FleetHomeSkills()); err != nil {
		return nil, err
	}

	lock, err := scan.ReadLockfile(p.SkillLock())
	if err != nil {
		return nil, fmt.Errorf("read skills lockfile: %w", err)
	}

	// Names are the identity the harnesses see (ls dedupes on them too),
	// so a double presence is a name match, wherever each copy's
	// directory sits. Track every home a name appears in so any pair or
	// N-way collision is flagged as one finding grouped by name.
	byName := map[string]map[string]string{}
	for _, src := range sources {
		for _, s := range src.skills {
			if _, ok := byName[s.Name]; !ok {
				byName[s.Name] = map[string]string{}
			}
			if _, exists := byName[s.Name][src.key]; !exists {
				byName[s.Name][src.key] = filepath.Join(src.skillsDir, s.Dir)
			}
		}
	}
	labels := map[string]string{}
	for _, src := range sources {
		labels[src.key] = src.label
	}

	var names []string
	for name, homes := range byName {
		if len(homes) > 1 {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	// The installed harnesses that read the canonical store natively would
	// see a double-present name twice. Claude is link-only and does not scan
	// the store, so it is excluded. Empty when no such harness is installed;
	// the CLI then reports the collision without a harness column.
	var scanners []string
	for _, d := range harness.SkillDirs(p) {
		if d.NativeScan {
			scanners = append(scanners, string(d.Harness))
		}
	}

	var findings []Finding
	for _, name := range names {
		homes := byName[name]
		var parts []string
		var copyPaths []string
		var collisionHomes []string
		for _, src := range sources {
			copyPath, ok := homes[src.key]
			if !ok {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s (%s)", labels[src.key], copyPath))
			copyPaths = append(copyPaths, copyPath)
			collisionHomes = append(collisionHomes, src.skillsDir)
		}
		// Keep message stable for two-way and N-way cases while
		// guaranteeing it mentions every conflicting path and the manual
		// resolution the ticket requires. Two-way keeps the historical
		// "both" phrasing so existing CLI contract tests stay green.
		joined := joinWithAnd(parts)
		prefix := "exists in "
		if len(parts) == 2 {
			prefix = "exists in both "
		}
		who := "a native-scanning harness"
		if len(scanners) > 0 {
			who = joinWithAnd(scanners)
		}
		findings = append(findings, Finding{
			Kind:      KindDoublePresence,
			Skill:     name,
			Homes:     collisionHomes,
			Harnesses: scanners,
			Message: fmt.Sprintf("%q %s%s — %s would see it twice, and one copy's rules may shadow the other — resolve by hand (remove one of the copies: %s)",
				name, prefix, joined, who, strings.Join(copyPaths, ", ")),
		})
	}
	// Stale lock entries for every custom home (union, deduped by Dir).
	// The message names the home the copy lives in so multi-dir setups
	// say which collection holds it.
	type customCopy struct {
		name     string
		label    string
		home     string
		copyPath string
	}
	customByDir := map[string]customCopy{}
	for _, src := range sources {
		if src.key == "canonical" {
			continue
		}
		for _, s := range src.skills {
			if _, ok := customByDir[s.Dir]; !ok {
				customByDir[s.Dir] = customCopy{
					name:     s.Name,
					label:    labels[src.key],
					home:     src.skillsDir,
					copyPath: filepath.Join(src.skillsDir, s.Dir),
				}
			}
		}
	}
	var staleDirs []string
	for dir := range customByDir {
		if _, ok := lock[dir]; ok {
			staleDirs = append(staleDirs, dir)
		}
	}
	sort.Strings(staleDirs)
	for _, dir := range staleDirs {
		c := customByDir[dir]
		findings = append(findings, Finding{
			Kind:      KindStaleLock,
			Skill:     c.name,
			Home:      c.home,
			HomeLabel: c.label,
			Message: fmt.Sprintf("%q lives in %s (%s), but the skills lockfile (%s) still carries its install entry — the entry's source and hash describe a skill that moved out of the canonical store, so the skills CLI will keep trying to update it — remove the entry by hand; fleet never writes the lockfile",
				c.name, c.label, c.copyPath, p.SkillLock()),
		})
	}

	cfg, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return nil, err
	}
	// An adopt target outside the scanned custom homes still proceeds at
	// adopt time, but the footgun stays visible here.
	if target := cfg.AdoptTarget(); target != "" {
		clean := filepath.Clean(target)
		scanned := false
		for _, src := range sources {
			if src.key == "canonical" {
				continue
			}
			if filepath.Clean(src.skillsDir) == clean {
				scanned = true
				break
			}
		}
		if !scanned {
			findings = append(findings, Finding{
				Kind: KindUnscannedAdoptTarget,
				Message: fmt.Sprintf("adopt target %q is not one of the scanned custom homes (tracked dirs plus the fleet-home fallback) — skills adopted there won't appear in ls — point it at a tracked dir or the fleet-home fallback (%s)",
					target, p.FleetHomeSkills()),
			})
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		if findings[i].Skill != findings[j].Skill {
			return findings[i].Skill < findings[j].Skill
		}
		return findings[i].Message < findings[j].Message
	})
	return findings, nil
}

// analyzeTrackedSet reports what a hand-edited skillsDirs list gets wrong:
// an entry missing from disk or not a directory, an entry that is a
// directory but holds no skills, and duplicate or nested entries that make
// precedence ambiguous. Add refuses all of these at write time, so each
// finding points at a config the user edited by hand. A dir already reported
// missing is not also reported empty, and a duplicate is reported once, not
// once per copy.
func analyzeTrackedSet(p *paths.Paths) ([]Finding, error) {
	tracked, err := trackedset.List(p)
	if err != nil {
		return nil, fmt.Errorf("resolve tracked dirs: %w", err)
	}
	if len(tracked) == 0 {
		return nil, nil
	}

	var findings []Finding
	seen := map[string]bool{}
	for _, dir := range tracked {
		clean := filepath.Clean(dir)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		findings = append(findings, trackedDirFindings(clean)...)
	}
	findings = append(findings, trackedSetOverlapFindings(tracked)...)
	return findings, nil
}

// trackedDirFindings reports a single unusable or empty tracked dir: missing
// from disk, not a directory, unreadable, or a directory holding no skills.
// An unreadable dir yields none here because the incomplete-scan finding
// already names it.
func trackedDirFindings(dir string) []Finding {
	info, err := os.Stat(dir)
	switch {
	case os.IsNotExist(err):
		return []Finding{{
			Kind:    KindTrackedDirMissing,
			Path:    dir,
			Message: fmt.Sprintf("tracked dir %s does not exist — `fleet skill remove-dir` it, or restore the directory", dir),
		}}
	case err != nil:
		return []Finding{{
			Kind:    KindTrackedDirMissing,
			Path:    dir,
			Message: fmt.Sprintf("tracked dir %s can't be read (%v) — fix the path or remove it with `fleet skill remove-dir`", dir, err),
		}}
	case !info.IsDir():
		return []Finding{{
			Kind:    KindTrackedDirMissing,
			Path:    dir,
			Message: fmt.Sprintf("tracked dir %s is not a directory — `fleet skill remove-dir` it, or point the entry at a collection dir", dir),
		}}
	}
	skills, err := scan.ScanStore(dir)
	if err != nil {
		return nil // an unreadable dir is already an incomplete-scan finding
	}
	if len(skills) == 0 {
		return []Finding{{
			Kind:    KindTrackedDirEmpty,
			Path:    dir,
			Message: fmt.Sprintf("tracked dir %s holds no skills — a collection dir's immediate children must each hold a SKILL.md", dir),
		}}
	}
	return nil
}

// trackedSetOverlapFindings reports duplicate and nested entries: for each
// entry, every earlier entry it repeats or overlaps is one finding.
// Duplicates are exact-path repeats; nesting is a strict descendant in
// either direction. Both make precedence ambiguous and only a hand edit can
// produce them.
func trackedSetOverlapFindings(tracked []string) []Finding {
	var findings []Finding
	for i, dir := range tracked {
		clean := filepath.Clean(dir)
		for j := 0; j < i; j++ {
			prev := filepath.Clean(tracked[j])
			switch {
			case prev == clean:
				findings = append(findings, Finding{
					Kind:    KindTrackedSetOverlap,
					Path:    clean,
					Message: fmt.Sprintf("tracked dir %s is listed more than once — remove the duplicate from the skillsDirs list by hand", clean),
				})
			case under(clean, prev):
				findings = append(findings, Finding{
					Kind:    KindTrackedSetOverlap,
					Path:    clean,
					Message: fmt.Sprintf("tracked dir %s is nested inside tracked dir %s — precedence between overlapping collections is ambiguous; remove one from the skillsDirs list by hand", clean, prev),
				})
			case under(prev, clean):
				findings = append(findings, Finding{
					Kind:    KindTrackedSetOverlap,
					Path:    clean,
					Message: fmt.Sprintf("tracked dir %s contains tracked dir %s — precedence between overlapping collections is ambiguous; remove one from the skillsDirs list by hand", clean, prev),
				})
			}
		}
	}
	return findings
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
