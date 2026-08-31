// Package state will own fleet's state file (~/.config/fleet/state.json):
// the versioned single source of truth for per-harness enablement that sync
// projects into each harness's native config. This ticket only stands up the
// package as a placeholder — `fleet skill ls` is read-only and reports what
// the harnesses' own configs say. The file, schema, and sync arrive in the
// next ticket.
package state
