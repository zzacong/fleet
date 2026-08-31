// Package harness is fleet's one seam to the six supported harnesses
// (opencode, pi, codex, claude code, Cursor, IBM Bob). Every adapter
// implements the same interface: detect the harness by its config directory
// and read per-skill enablement from the harness's own config. Tests build
// fake homes in t.TempDir() and run through this seam; no adapter touches
// the filesystem outside the injected home root.
package harness

import "github.com/zacong/fleet/internal/paths"

// Harness identifies a harness fleet manages.
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
	// Disables lists every skill name the harness's own config disables
	// through an exact-name entry fleet could own — including names that
	// aren't in the canonical store. Glob patterns, blanket rules, and
	// shapes fleet can't write are excluded: they surface as flags, never
	// as state-tracked entries. Adapters without a config lever leave it
	// nil. Doctor compares this against the state file.
	Disables []string
}

// SkillWrite is one skill's desired enablement for one harness.
type SkillWrite struct {
	Name string
	// State is the desired state: StateOff ensures fleet's own marker is
	// present, StateOn ensures it is gone.
	State State
}

// Change records one enablement flip a write actually applied: the skill's
// effective state moved from From to To.
type Change struct {
	Skill string
	From  State
	To    State
}

// Flag describes a config entry that affects skill enablement but that
// fleet left untouched because it isn't fleet's own. Skill is empty when
// the entry isn't tied to one skill (blankets and patterns).
type Flag struct {
	Skill   string
	Message string
}

// WriteReport is what a Project call changed and what it left alone.
type WriteReport struct {
	Changed []Change
	Flags   []Flag
}

// Empty reports whether there is nothing to tell the user about.
func (r WriteReport) Empty() bool { return len(r.Changed) == 0 && len(r.Flags) == 0 }

// Adapter is the seam every harness read side goes through.
type Adapter interface {
	// Harness names the harness this adapter talks to.
	Harness() Harness
	// Installed reports whether the harness's config directory exists.
	Installed() bool
	// Read reads current enablement for the named skills. Read never
	// writes; it reports what the harness's own config expresses.
	Read(names []string) (ReadResult, error)
	// CanProject reports whether the adapter can write enablement into
	// the harness's own config. Cursor and Bob have no per-skill disable
	// mechanism; toggles for them are a no-op with a message.
	CanProject() bool
	// Project projects desired enablement into the harness's config with
	// a read-modify-write that preserves everything fleet doesn't own:
	// unknown keys, comments, formatting, and unrecognized enablement
	// entries (which are flagged, not touched). Only call it when
	// CanProject() is true. Only writes the file when something changed.
	Project(writes []SkillWrite) (WriteReport, error)
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

// Installed returns the adapters whose harness is present on this machine,
// in All's order. Most call sites want this subset, not all six.
func Installed(p *paths.Paths) []Adapter {
	var out []Adapter
	for _, a := range All(p) {
		if a.Installed() {
			out = append(out, a)
		}
	}
	return out
}
