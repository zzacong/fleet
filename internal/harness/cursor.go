package harness

import (
	"fmt"

	"github.com/zzacong/fleet/internal/paths"
)

// Cursor reads ~/.agents/skills natively (documented) and has no
// config-level per-skill disable mechanism — the only documented lever,
// `disable-model-invocation` frontmatter, would cross-talk with pi and
// claude. The read side is therefore derived, not read: every skill is on.
// Doctor explains the limitation in a later ticket.
type CursorAdapter struct {
	home *paths.Paths
}

// NewCursor builds the Cursor adapter under an injected home root.
func NewCursor(p *paths.Paths) *CursorAdapter { return &CursorAdapter{home: p} }

// Harness implements Adapter.
func (a *CursorAdapter) Harness() Harness { return Cursor }

// Installed implements Adapter.
func (a *CursorAdapter) Installed() bool { return isDir(a.home.CursorDir()) }

// Read implements Adapter.
func (a *CursorAdapter) Read(names []string) (ReadResult, error) {
	return ReadResult{States: onForAll(names)}, nil
}

// CanProject implements Adapter: there is no config lever to write.
func (a *CursorAdapter) CanProject() bool { return false }

// Project implements Adapter; Cursor has no write side, so callers must
// check CanProject first.
func (a *CursorAdapter) Project(writes []SkillWrite) (WriteReport, error) {
	return WriteReport{}, fmt.Errorf("cursor has no per-skill disable mechanism")
}
