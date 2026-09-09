// Package customs owns the custom-skills homes: every tracked collection
// plus the fleet-home fallback ~/.config/fleet/skills. Adopt moves a skill
// from the canonical store into the resolved home, wires that home into the
// harnesses that take extra discovery paths (opencode, pi), and manages
// every link-based harness's symlink to it (codex, claude code, Cursor,
// Bob).
//
// Adoption records nothing but the location itself: custom means lives in
// the resolved home, so the state file is untouched. Disables recorded
// before adoption keep applying (their config rules target the skill's
// name, wherever it lives), and moving the directory back by hand undoes
// the adoption cleanly.
package customs

import (
	"github.com/zzacong/fleet/internal/harness"
)

// Report is what one adopt run did. Empty slices mean nothing to do.
type Report struct {
	// Skill is the skill's directory name — the discovery name every
	// harness and link uses.
	Skill string
	// From and To are the directory's old and new locations. From is empty
	// when nothing moved (the skill was already adopted: Moved is false
	// and the wiring and links were still ensured).
	From, To string
	// Moved reports whether the directory actually moved this run.
	Moved bool
	// Wired lists the config-path harnesses the resolved home was wired into
	// this run.
	Wired []harness.WireResult
	// Linked lists the managed links created or repointed this run.
	Linked []harness.LinkResult
}
