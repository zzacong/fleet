// Package harness is fleet's one seam to the six supported coding agents
// (opencode, pi, codex, claude code, Cursor, IBM Bob). Every adapter
// implements the same interface: detect the harness by its config directory
// and read per-skill enablement from the harness's own config. Tests build
// fake homes in t.TempDir() and run through this seam; no adapter touches
// the filesystem outside the injected home root.
package harness

import "github.com/zacong/fleet/internal/paths"

// Harness identifies a coding agent fleet manages.
type Harness string

const (
	OpenCode Harness = "opencode"
	Pi       Harness = "pi"
	Codex    Harness = "codex"
	Claude   Harness = "claude"
	Cursor   Harness = "cursor"
	Bob      Harness = "bob"
)

// State is the per-skill enablement a harness's own config expresses.
type State string

const (
	// StateOn: the harness discovers the skill and nothing disables it.
	StateOn State = "on"
	// StateOff: the harness discovers the skill but its config disables it.
	StateOff State = "off"
	// StateAbsent: the harness cannot discover the skill at all (for
	// harnesses that discover through links, no link exists).
	StateAbsent State = "absent"
)

// ReadResult is what an adapter read observed in the harness's config.
type ReadResult struct {
	// States maps every requested skill name to its state.
	States map[string]State
	// Linked lists skill names with a link or directory in the harness's
	// own skills dir. Populated by the harnesses that discover through
	// links (claude code, Bob); nil for the others.
	Linked []string
	// Dialect is the opencode config dialect detected in the file:
	// "v1", "v2", or "" when no marker was found. Only opencode sets it.
	Dialect string
	// SkillSources lists extra skill discovery sources configured in the
	// harness config (opencode's `skills` key, in either dialect). Only
	// opencode sets it.
	SkillSources []string
}

// Adapter is the seam every harness read side goes through.
type Adapter interface {
	// Harness names the agent this adapter talks to.
	Harness() Harness
	// Installed reports whether the harness's config directory exists.
	Installed() bool
	// Read reads current enablement for the named skills. Read never
	// writes; it reports what the harness's own config expresses.
	Read(names []string) (ReadResult, error)
}

// All returns one adapter per supported harness, in the column order the
// skill ls table uses.
func All(p *paths.Paths) []Adapter {
	return []Adapter{
		NewOpenCode(p),
		NewPi(p),
		NewCodex(p),
		NewClaude(p),
		NewCursor(p),
		NewBob(p),
	}
}
