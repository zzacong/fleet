// Managed custom-skill links: the write side of link-based discovery.
// codex, claude code, Cursor, and Bob reach skills outside the canonical
// store only through a symlink named after the skill in their own skills
// dir, so each custom skill gets one link per harness — pointed at the
// fleet repo. By construction these links never target the canonical
// store: a link into ~/.agents/skills would make opencode and pi see the
// skill twice, and it would make the link indistinguishable from the
// skills CLI's redundant per-agent links.

package harness

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zacong/fleet/internal/paths"
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
