// The per-agent skills-dir surface, both halves: managed custom-skill
// links (the write side of link-based discovery) and link classification
// (what sync removes, what doctor reports, what is never touched).
//
// codex, claude code, Cursor, and Bob reach skills outside the canonical
// store only through a symlink named after the skill in their own skills
// dir, so each custom skill gets one link per harness — pointed at the
// fleet repo. By construction these links never target the canonical
// store: a link into ~/.agents/skills would make opencode and pi see the
// skill twice, and it would make the link indistinguishable from the
// skills CLI's redundant per-agent links.
//
// For harnesses that scan the canonical store natively those per-agent
// links are redundant double-coverage; sync removes them. Everything else
// found in a skills dir is classified here so sync and doctor can act on
// exactly what fleet owns — and nothing else.

package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zzacong/fleet/internal/paths"
)

// LinkAction says what a LinkSkill call did to the harness's skills dir.
type LinkAction string

const (
	// LinkCreated: the link did not exist and was made.
	LinkCreated LinkAction = "created"
	// LinkRepointed: an existing symlink pointed elsewhere and now targets
	// the repo (e.g. the skills CLI's link to the canonical store, left
	// dangling by the move).
	LinkRepointed LinkAction = "repointed"
	// LinkUnchanged: the link already targeted the repo.
	LinkUnchanged LinkAction = "unchanged"
	// LinkSkipped: something fleet doesn't own is in the way.
	LinkSkipped LinkAction = "skipped"
)

// LinkChange is the outcome of one managed-link action.
type LinkChange struct {
	Action LinkAction
	// From is the link's previous target for LinkRepointed.
	From string
	// Note explains a LinkSkipped.
	Note string
}

// SkillLinker is the write side of link-based discovery, implemented by
// every harness that reaches extra skills through a symlink in its own
// skills dir (codex, claude code, Cursor, Bob).
type SkillLinker interface {
	// LinkSkill points the harness's <name> link at target (the skill's
	// repo dir). It creates the link when missing, repoints a symlink that
	// targets something else, and never touches a real directory or file —
	// that is the user's, so it is reported and left alone.
	LinkSkill(name, target string) (LinkChange, error)
}

// LinkResult reports one harness's managed link for one custom skill.
// Unchanged links are omitted by LinkCustomSkill.
type LinkResult struct {
	Harness Harness
	Name    string
	Target  string
	Change  LinkChange
}

// LinkCustomSkill manages every installed link-based harness's link for
// one custom skill: <harness skills dir>/<name> → target. Harnesses that
// discover through config paths (opencode, pi) get no links, and harnesses
// with nothing left to change are omitted.
func LinkCustomSkill(p *paths.Paths, name, target string) ([]LinkResult, error) {
	var results []LinkResult
	for _, a := range All(p) {
		l, ok := a.(SkillLinker)
		if !ok || !a.Installed() {
			continue
		}
		change, err := l.LinkSkill(name, target)
		if err != nil {
			return nil, fmt.Errorf("link %s skill %q: %w", a.Harness(), name, err)
		}
		if change.Action != LinkUnchanged {
			results = append(results, LinkResult{Harness: a.Harness(), Name: name, Target: target, Change: change})
		}
	}
	return results, nil
}

// LinkSkill implements SkillLinker: customs reach codex through
// ~/.codex/skills (codex scans the canonical store natively).
func (a *CodexAdapter) LinkSkill(name, target string) (LinkChange, error) {
	return manageLink(a.home.CodexSkills(), name, target)
}

// LinkSkill implements SkillLinker: links are claude's only path to skills
// outside the canonical store.
func (a *ClaudeAdapter) LinkSkill(name, target string) (LinkChange, error) {
	return manageLink(a.home.ClaudeSkills(), name, target)
}

// LinkSkill implements SkillLinker: customs reach Cursor through
// ~/.cursor/skills.
func (a *CursorAdapter) LinkSkill(name, target string) (LinkChange, error) {
	return manageLink(a.home.CursorSkills(), name, target)
}

// LinkSkill implements SkillLinker: the link is Bob's only path to custom
// skills (repo skills are outside what Bob scans natively).
func (a *BobAdapter) LinkSkill(name, target string) (LinkChange, error) {
	return manageLink(a.home.BobSkills(), name, target)
}

// manageLink keeps dir/<name> a symlink to target: created when missing,
// repointed when a symlink points elsewhere, skipped with a note when a
// real directory or file is in the way. Idempotent.
func manageLink(dir, name, target string) (LinkChange, error) {
	link := filepath.Join(dir, name)
	info, err := os.Lstat(link)
	switch {
	case os.IsNotExist(err):
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return LinkChange{}, err
		}
		if err := os.Symlink(target, link); err != nil {
			return LinkChange{}, err
		}
		return LinkChange{Action: LinkCreated}, nil
	case err != nil:
		return LinkChange{}, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return LinkChange{Action: LinkSkipped, Note: fmt.Sprintf("%s exists and is not a link — left alone", link)}, nil
	}
	current, err := os.Readlink(link)
	if err != nil {
		return LinkChange{}, err
	}
	if current == target {
		return LinkChange{Action: LinkUnchanged}, nil
	}
	if err := os.Remove(link); err != nil {
		return LinkChange{}, err
	}
	if err := os.Symlink(target, link); err != nil {
		return LinkChange{}, err
	}
	return LinkChange{Action: LinkRepointed, From: current}, nil
}

// SkillDir describes one harness's per-agent skills directory and the
// redundancy rule that applies to the links in it.
type SkillDir struct {
	Harness Harness
	Path    string
	// NativeScan reports whether the harness discovers the canonical store
	// on its own, making a per-agent symlink into the store redundant:
	// the harness already sees every store skill.
	NativeScan bool
}

// nativeScanHarnesses: the harnesses that read ~/.agents/skills directly.
// Claude code is the exception — a store link is its only discovery path,
// so its links are load-bearing (and a missing target breaks them).
var nativeScanHarnesses = map[Harness]bool{
	OpenCode: true,
	Pi:       true,
	Codex:    true,
	Cursor:   true,
	Bob:      true,
}

// skillDirPaths maps each harness to its per-agent skills directory.
func skillDirPaths(p *paths.Paths) map[Harness]string {
	return map[Harness]string{
		OpenCode: p.OpenCodeSkills(),
		Pi:       p.PiSkills(),
		Codex:    p.CodexSkills(),
		Claude:   p.ClaudeSkills(),
		Cursor:   p.CursorSkills(),
		Bob:      p.BobSkills(),
	}
}

// SkillDirs returns one SkillDir per installed harness, in the column order
// the skill ls table uses. Uninstalled harnesses are skipped: their dirs
// are not fleet's business.
func SkillDirs(p *paths.Paths) []SkillDir {
	dirs := skillDirPaths(p)
	var out []SkillDir
	for _, a := range All(p) {
		if !a.Installed() {
			continue
		}
		out = append(out, SkillDir{
			Harness:    a.Harness(),
			Path:       dirs[a.Harness()],
			NativeScan: nativeScanHarnesses[a.Harness()],
		})
	}
	return out
}

// EntryClass is what fleet knows about one entry in a harness's skills dir.
type EntryClass string

const (
	// EntryRedundant: a symlink into the canonical store in a harness that
	// scans the store natively. Sync removes it.
	EntryRedundant EntryClass = "redundant"
	// EntryBroken: a symlink whose target is missing or that loops.
	EntryBroken EntryClass = "broken"
	// EntryForeign: a symlink pointing outside the canonical store — a
	// managed custom-skill link to the repo, or the user's own link.
	// Never redundant by construction; never touched.
	EntryForeign EntryClass = "foreign"
	// EntryUntracked: a real directory or file, not a symlink. Reported,
	// never touched.
	EntryUntracked EntryClass = "untracked"
	// EntryExpected: a store link in a harness that discovers through
	// links (claude code) — the skills CLI's mechanism working as
	// intended, not a finding.
	EntryExpected EntryClass = "expected"
)

// Entry is one immediate entry of a harness's skills dir.
type Entry struct {
	Harness Harness
	// Name is the entry's file name; for per-skill links, the skill name.
	Name string
	Path string
	// Class is what fleet knows about the entry.
	Class EntryClass
	// Target is the final path the symlink chain resolves to, or "" when
	// the entry is not a symlink (or its chain loops).
	Target string
	// Reason explains the classification in one line.
	Reason string
}

// maxLinkHops caps symlink-chain resolution; past it the chain loops.
const maxLinkHops = 40

// ScanSkillDir classifies every immediate entry of one skills dir against
// the canonical store at store. Hidden (dot-prefixed) entries are skipped:
// they belong to the harness itself, not to any skill fleet manages.
// Classification is safe by construction:
// only links that provably resolve into the store in a native scanner are
// redundant; everything ambiguous stays untouched. A missing dir yields no
// entries and no error.
func ScanSkillDir(d SkillDir, store string) ([]Entry, error) {
	links, err := os.ReadDir(d.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []Entry
	for _, link := range links {
		if strings.HasPrefix(link.Name(), ".") {
			// Hidden entries are the harness's own reserved namespace
			// (codex's .system skills, .DS_Store noise): never fleet-managed
			// skill links, so not classified and not reported.
			continue
		}
		e, err := classifyEntry(d, filepath.Join(d.Path, link.Name()), store)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// classifyEntry works out what one skills-dir entry is and what fleet may
// do about it.
func classifyEntry(d SkillDir, path, store string) (Entry, error) {
	e := Entry{Harness: d.Harness, Name: filepath.Base(path), Path: path}

	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return e, nil // raced away between ReadDir and here: nothing to say
		}
		return e, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		e.Class = EntryUntracked
		if info.IsDir() {
			e.Reason = "a real directory, not a symlink — left alone"
		} else {
			e.Reason = "a plain file, not a symlink — left alone"
		}
		return e, nil
	}

	final, err := resolveLink(path)
	if err != nil {
		e.Class = EntryBroken
		e.Reason = "symlink loop — left alone"
		return e, nil
	}
	e.Target = final

	switch {
	case pathInside(final, store):
		if d.NativeScan {
			e.Class = EntryRedundant
			e.Reason = fmt.Sprintf("%s scans the canonical store natively — this link double-covers the skill", d.Harness)
			if !pathExists(final) {
				e.Reason += " (and its target is missing)"
			}
			return e, nil
		}
		if pathExists(final) {
			e.Class = EntryExpected
			return e, nil
		}
		e.Class = EntryBroken
		e.Reason = fmt.Sprintf("%s discovers skills only through links, but this one's target is missing", d.Harness)
		return e, nil

	case pathExists(final):
		e.Class = EntryForeign
		if final == store {
			e.Reason = "the link points at the canonical store itself — fleet only manages per-skill links, so it is left alone"
		} else {
			e.Reason = "the symlink points outside the canonical store — fleet doesn't manage it"
		}
		return e, nil

	default:
		e.Class = EntryBroken
		e.Reason = "the symlink target is missing"
		return e, nil
	}
}

// resolveLink follows a chain of symlinks to its final path, resolving
// relative targets against the directory of the link they hang from. The
// final path may not exist — the caller checks. An error reports a chain
// that never settles (a loop).
func resolveLink(link string) (string, error) {
	cur := link
	for i := 0; i < maxLinkHops; i++ {
		target, err := os.Readlink(cur)
		if err != nil {
			// Not a symlink (or it vanished mid-chain): the chain ends
			// here, whether or not the final path exists.
			return cur, nil
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(cur), target)
		}
		cur = filepath.Clean(target)
	}
	return "", fmt.Errorf("symlink chain from %s does not settle within %d hops", link, maxLinkHops)
}

// pathInside reports whether path is a strict child of dir, after
// resolving symlinks in both. Lexical containment decides when the paths
// can't be evaluated (missing store or missing leaf): the skills CLI's
// links name the store literally, so a link whose written target sits in
// the store double-covers the skill even if the skill is gone.
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

// under reports whether path is strictly below dir, lexically.
func under(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// pathExists reports whether path exists, following symlinks.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
