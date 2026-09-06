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
	// skill homes (canonical store, fleet-home fallback, tracked repos) —
	// the harnesses that read more than one would see it twice. Only the
	// user's hands can remove a copy.
	KindDoublePresence Kind = "double-presence"
	// KindUnscannedAdoptTarget: the configured adopt-target collection dir
	// is not one of the scanned custom homes, so skills adopted there
	// won't appear in ls. Adopt still proceeds; the footgun stays visible.
	KindUnscannedAdoptTarget Kind = "unscanned-adopt-target"
	// KindNonGitRepo: an explicit tracked repo root that is not a git
	// checkout (no .git). Bare pull skips it with a warning instead of
	// failing the whole run; the entry needs a clone or a removal by hand.
	KindNonGitRepo Kind = "non-git-repo"
	// KindStaleLock: a skills CLI lockfile entry for a skill that now
	// lives in a custom home (fleet-home fallback or tracked collection) —
	// the skills CLI would keep trying to update it. Fleet reads the
	// lockfile only; it never writes it.
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
	// Custom skills are recognized across every custom home (fallback and
	// tracked repos) so managed links into any of them are filtered from
	// unknown entries.
	customHomes, customByDir, customByName := scanCustomIndex(p)
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
			if isManagedCustomLink(e, customHomes, customByDir, customByName) {
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

// scanCustomIndex scans every custom home — the fleet-home fallback and
// each tracked repo's collection — and returns the union for managed-link
// filtering: the home dirs, skills keyed by directory, and frontmatter
// names. Stores that fail to scan are skipped, mirroring the old
// best-effort filter.
func scanCustomIndex(p *paths.Paths) ([]string, map[string]string, map[string]bool) {
	dirs := []string{p.FleetHomeSkills()}
	if tracked, err := p.TrackedRepos(); err == nil {
		for _, root := range tracked {
			dirs = append(dirs, filepath.Join(root, "skills"))
		}
	}
	seen := map[string]bool{}
	byDir := map[string]string{}
	byName := map[string]bool{}
	var homes []string
	for _, dir := range dirs {
		clean := filepath.Clean(dir)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		homes = append(homes, dir)
		skills, err := scan.ScanStore(dir)
		if err != nil {
			continue
		}
		for _, s := range skills {
			byDir[s.Dir] = s.Name
			byName[s.Name] = true
		}
	}
	return homes, byDir, byName
}

// isManagedCustomLink reports whether the entry is a managed custom-skill
// link pointing into any custom home (fallback or tracked repo). Such links
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

// analyzeRepoSkills cross-checks every skill home — the canonical store
// (~/.agents/skills), each tracked repo's collection (explicit list order,
// then auto-tracked fleet-home checkouts alphabetically, env override
// first), the unversioned fleet-home fallback — plus the skills CLI lockfile
// and the machine-local config. A name present in more than one home is
// double visibility — opencode and pi read the canonical store and the wired
// custom homes, so they would see the skill twice. A lock entry for a custom
// skill is stale provenance from before its adoption: the skills CLI keys
// updates by it and would keep touching a skill that moved. An adopt target
// outside the scanned homes and an explicit entry without a git checkout are
// warnings with the same shape: visible footguns, never silent failures. All
// of them need the user's hands; fleet never deletes a copy or edits the
// lockfile.
func analyzeRepoSkills(p *paths.Paths) ([]Finding, error) {
	type source struct {
		// key groups copies of one name; one finding lists every key.
		key string
		// label describes the home in messages, e.g. "the explicit repo".
		label string
		// skillsDir is the collection dir scanned.
		skillsDir string
		skills    []scan.Skill
	}
	var sources []source
	addSource := func(key, label, dir string) error {
		ss, err := scan.ScanStore(dir)
		if err != nil {
			return fmt.Errorf("scan %s: %w", label, err)
		}
		sources = append(sources, source{key: key, label: label, skillsDir: dir, skills: ss})
		return nil
	}
	if err := addSource("canonical", "the canonical store", p.SkillsStore()); err != nil {
		return nil, err
	}

	// Reserve the fixed homes so a tracked root that overlaps one is not
	// counted twice.
	reserved := map[string]bool{
		filepath.Clean(p.SkillsStore()):     true,
		filepath.Clean(p.FleetHomeSkills()): true,
	}
	tracked, err := p.TrackedRepos()
	if err != nil {
		return nil, fmt.Errorf("resolve tracked repos: %w", err)
	}
	var envRoot string
	if env := os.Getenv("FLEET_REPO"); env != "" {
		envRoot, err = filepath.Abs(env)
		if err != nil {
			return nil, err
		}
		envRoot = filepath.Clean(envRoot)
	}
	for _, root := range tracked {
		collection := filepath.Join(root, "skills")
		if reserved[filepath.Clean(collection)] {
			continue
		}
		reserved[filepath.Clean(collection)] = true
		label := "the explicit repo"
		switch clean := filepath.Clean(root); {
		case envRoot != "" && clean == envRoot:
			label = "the env override repo"
		case clean != "" && filepath.Dir(clean) == filepath.Clean(p.FleetReposDir()):
			label = "the fleet-home checkout"
		}
		if err := addSource("tracked:"+filepath.Clean(root), label, collection); err != nil {
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

	var findings []Finding
	for _, name := range names {
		homes := byName[name]
		var parts []string
		var copyPaths []string
		for _, src := range sources {
			copyPath, ok := homes[src.key]
			if !ok {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s (%s)", labels[src.key], copyPath))
			copyPaths = append(copyPaths, copyPath)
		}
		// Keep message stable for two-way and N-way cases while
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
				name, prefix, joined, strings.Join(copyPaths, ", ")),
		})
	}
	// Stale lock entries for every custom home (union, deduped by Dir).
	// The message names the home the copy lives in so multi-repo setups
	// say which collection holds it.
	type customCopy struct {
		name     string
		label    string
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
			Kind:  KindStaleLock,
			Skill: c.name,
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
				Message: fmt.Sprintf("adopt target %q is not one of the scanned custom homes (tracked repos plus the fleet-home fallback) — skills adopted there won't appear in ls — point it at a tracked collection or the fleet-home fallback (%s)",
					target, p.FleetHomeSkills()),
			})
		}
	}
	// Explicit entries without a git checkout are skipped with a warning
	// on bare pull; surface the same warning here so one uncloned path
	// never blocks the rest silently.
	for _, root := range cfg.SkillsRepos() {
		if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
			findings = append(findings, Finding{
				Kind: KindNonGitRepo,
				Message: fmt.Sprintf("explicit repo %q is not a git checkout (no .git there) — bare pull skips it instead of failing the run — clone the repo there or remove the path from the explicit list by hand",
					root),
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
