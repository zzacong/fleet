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

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/scan"
	"github.com/zzacong/fleet/internal/state"
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
	// three homes (canonical store, fleet-home, skills repo) — the harnesses
	// that read more than one would see it twice. Only the user's hands can
	// remove a copy.
	KindDoublePresence Kind = "double-presence"
	// KindStaleLock: a skills CLI lockfile entry for a skill that now
	// lives in a custom home (fleet-home or repo) — the skills CLI would
	// keep trying to update it. Fleet reads the lockfile only; it never
	// writes it.
	KindStaleLock Kind = "stale-lock"
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
	// Repo and fleet-home skills are needed to filter managed custom links from unknown entries.
	var repoSkills []scan.Skill
	if p.RepoSkills() != "" {
		if rs, err := scan.ScanStore(p.RepoSkills()); err == nil {
			repoSkills = rs
		}
	}
	repoByDir := map[string]string{}
	repoByName := map[string]bool{}
	for _, s := range repoSkills {
		repoByDir[s.Dir] = s.Name
		repoByName[s.Name] = true
	}
	var fleetSkills []scan.Skill
	if fs, err := scan.ScanStore(p.FleetHomeSkills()); err == nil {
		fleetSkills = fs
	}
	fleetByDir := map[string]string{}
	fleetByName := map[string]bool{}
	for _, s := range fleetSkills {
		fleetByDir[s.Dir] = s.Name
		fleetByName[s.Name] = true
	}
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
			if isManagedCustomLink(e, p, repoByDir, repoByName, fleetByDir, fleetByName) {
				continue
			}
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

	// Customs: the repo's skills/ dir against the canonical store and the
	// skills CLI lockfile.
	repoFindings, err := analyzeRepoSkills(p)
	if err != nil {
		return Report{}, err
	}
	rep.Findings = append(rep.Findings, repoFindings...)

	return rep, nil
}

// linkFinding turns a classified skills-dir entry into a finding. Entries
// that are exactly as they should be (claude's live store links) produce
// none.
func linkFinding(e harness.Entry) (Finding, bool) {
	f := Finding{Harness: string(e.Harness), Skill: e.Name, Path: e.Path}
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

// isManagedCustomLink reports whether the entry is a managed custom-skill
// link pointing into either custom home (repo or fleet-home). Such links
// are fleet's own discovery path for adopted skills, not unknown entries.
func isManagedCustomLink(e harness.Entry, p *paths.Paths, repoByDir map[string]string, repoByName map[string]bool, fleetByDir map[string]string, fleetByName map[string]bool) bool {
	if e.Class != harness.EntryForeign {
		return false
	}
	if e.Target == "" {
		return false
	}
	repoSkills := p.RepoSkills()
	fleetSkills := p.FleetHomeSkills()
	isRepo := repoSkills != "" && (pathInside(e.Target, repoSkills) || e.Target == repoSkills)
	isFleet := fleetSkills != "" && (pathInside(e.Target, fleetSkills) || e.Target == fleetSkills)
	if !isRepo && !isFleet {
		return false
	}
	// The link name should match a custom skill (by dir or frontmatter name) in either home.
	if _, ok := repoByDir[e.Name]; ok {
		return true
	}
	if repoByName[e.Name] {
		return true
	}
	if _, ok := fleetByDir[e.Name]; ok {
		return true
	}
	if fleetByName[e.Name] {
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

// isManagedRepoLink is kept for compatibility; it now also suppresses
// fleet-home managed links by delegating to isManagedCustomLink.
//
//nolint:unused // retained for compatibility; isManagedCustomLink is the primary path
func isManagedRepoLink(e harness.Entry, p *paths.Paths, repoByDir map[string]string, repoByName map[string]bool) bool {
	fleetByDir := map[string]string{}
	fleetByName := map[string]bool{}
	if fs, err := scan.ScanStore(p.FleetHomeSkills()); err == nil {
		for _, s := range fs {
			fleetByDir[s.Dir] = s.Name
			fleetByName[s.Name] = true
		}
	}
	return isManagedCustomLink(e, p, repoByDir, repoByName, fleetByDir, fleetByName)
}

// pathInside reports whether path is inside dir. Local copy to avoid
// importing harness's unexported helper; lexical plus symlink-evaluated
// check mirrors harness.pathInside.
func pathInside(path, dir string) bool {
	if under(path, dir) {
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
	return under(evaled, evalDir)
}

func under(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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

// analyzeRepoSkills cross-checks the three skill homes — the canonical store
// (~/.agents/skills), the fleet-home fallback (~/.config/fleet/skills),
// and the skills repo (when set) — plus the skills CLI lockfile. A name
// present in more than one home is double visibility — opencode and pi
// read the canonical store and the wired custom homes, so they would see
// the skill twice. A lock entry for a repo skill is stale provenance from
// before its adoption: the skills CLI keys updates by it and would keep
// touching a skill that moved. Both need the user's hands; fleet never
// deletes a copy or edits the lockfile.
func analyzeRepoSkills(p *paths.Paths) ([]Finding, error) {
	storeSkills, err := scan.ScanStore(p.SkillsStore())
	if err != nil {
		return nil, fmt.Errorf("scan canonical store: %w", err)
	}
	fleetSkills, err := scan.ScanStore(p.FleetHomeSkills())
	if err != nil {
		return nil, fmt.Errorf("scan fleet home: %w", err)
	}
	var repoSkills []scan.Skill
	if p.RepoSkills() != "" {
		repoSkills, err = scan.ScanStore(p.RepoSkills())
		if err != nil {
			return nil, fmt.Errorf("scan repo skills: %w", err)
		}
	}
	lock, err := scan.ReadLockfile(p.SkillLock())
	if err != nil {
		return nil, fmt.Errorf("read skills lockfile: %w", err)
	}

	// Names are the identity the harnesses see (ls dedupes on them too),
	// so a double presence is a name match, wherever each copy's
	// directory sits. Track every home a name appears in so any pair or
	// triple is flagged as one finding grouped by name.
	type homePresence struct {
		label string
		path  string
	}
	byName := map[string]map[string]homePresence{}
	add := func(name, dir, storePath, label string) {
		if _, ok := byName[name]; !ok {
			byName[name] = map[string]homePresence{}
		}
		if _, exists := byName[name][label]; !exists {
			byName[name][label] = homePresence{label: label, path: filepath.Join(storePath, dir)}
		}
	}
	for _, s := range storeSkills {
		add(s.Name, s.Dir, p.SkillsStore(), "canonical")
	}
	for _, s := range fleetSkills {
		add(s.Name, s.Dir, p.FleetHomeSkills(), "fleet-home")
	}
	for _, s := range repoSkills {
		add(s.Name, s.Dir, p.RepoSkills(), "repo")
	}

	// Deterministic order: sorted names, homes in canonical → fleet-home → repo.
	orderedLabels := []string{"canonical", "fleet-home", "repo"}
	labelDesc := map[string]func(string) string{
		"canonical":  func(path string) string { return fmt.Sprintf("the canonical store (%s)", path) },
		"fleet-home": func(path string) string { return fmt.Sprintf("the fleet home (%s)", path) },
		"repo":       func(path string) string { return fmt.Sprintf("the repo skills dir (%s)", path) },
	}

	var names []string
	for name, homes := range byName {
		if len(homes) > 1 {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var findings []Finding
	for _, name := range names {
		homes := byName[name]
		var parts []string
		var paths []string
		for _, lbl := range orderedLabels {
			if hp, ok := homes[lbl]; ok {
				parts = append(parts, labelDesc[lbl](hp.path))
				paths = append(paths, hp.path)
			}
		}
		// Keep message stable for two-way and three-way cases while
		// guaranteeing it mentions every conflicting path and the manual
		// resolution the ticket requires. Two-way keeps the historical
		// "both" phrasing so existing CLI contract tests stay green.
		joined := strings.Join(parts, " and ")
		if len(parts) > 2 {
			joined = strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1]
		}
		prefix := "exists in "
		if len(parts) == 2 {
			prefix = "exists in both "
		}
		findings = append(findings, Finding{
			Kind:  KindDoublePresence,
			Skill: name,
			Message: fmt.Sprintf("%q %s%s — opencode and pi would see it twice, and one copy's rules may shadow the other — resolve by hand (remove one of the copies: %s)",
				name, prefix, joined, strings.Join(paths, ", ")),
		})
	}
	// Stale lock entries for both custom homes (union, deduped by Dir).
	seen := map[string]bool{}
	for _, s := range fleetSkills {
		if _, ok := lock[s.Dir]; ok && !seen[s.Dir] {
			seen[s.Dir] = true
			findings = append(findings, Finding{
				Kind:  KindStaleLock,
				Skill: s.Name,
				Message: fmt.Sprintf("%q lives in the fleet home (%s), but the skills lockfile (%s) still carries its install entry — the entry's source and hash describe a skill that moved out of the canonical store, so the skills CLI will keep trying to update it — remove the entry by hand; fleet never writes the lockfile",
					s.Name, filepath.Join(p.FleetHomeSkills(), s.Dir), p.SkillLock()),
			})
		}
	}
	for _, s := range repoSkills {
		if _, ok := lock[s.Dir]; ok && !seen[s.Dir] {
			seen[s.Dir] = true
			findings = append(findings, Finding{
				Kind:  KindStaleLock,
				Skill: s.Name,
				Message: fmt.Sprintf("%q lives in the repo skills dir (%s), but the skills lockfile (%s) still carries its install entry — the entry's source and hash describe a skill that moved out of the canonical store, so the skills CLI will keep trying to update it — remove the entry by hand; fleet never writes the lockfile",
					s.Name, filepath.Join(p.RepoSkills(), s.Dir), p.SkillLock()),
			})
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		return findings[i].Skill < findings[j].Skill
	})
	return findings, nil
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
