# 02: Remove a dropped collection's managed links

**What to build:** `RemoveCustomLinks(p, collectionDir)` in `internal/harness`, deleting only the symlinks in link-based harness skills dirs (codex, claude, cursor, bob) that resolve under the dropped collection dir.

**Blocked by:** none.

**Status:** resolved

- [x] Removes symlinks resolving under `collectionDir` in each installed link-based harness dir; reports per-harness removals.
- [x] Never touches real directories/files, foreign user links, redundant store links, or claude's expected store links.
- [x] Missing harness dirs are no-ops, not errors.
- [x] Unit tests with fixture skills dirs covering managed, foreign, untracked, and broken entries.
