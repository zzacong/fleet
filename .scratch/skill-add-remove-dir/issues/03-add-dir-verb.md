# 03: `fleet skill add-dir <path>` end to end

**What to build:** Register an existing directory of skills and make its skills reach every installed harness, with the validation stated in the spec. `~` expands to the home directory and a relative path resolves against the working directory.

**Blocked by:** 01, 02.

**Status:** resolved

- [x] `fleet skill add-dir <path>` resolves `~` and a relative path to a cleaned absolute path, then appends it to `skillsDirs` without reordering existing entries.
- [x] It refuses, with a specific message, a missing path, a non-directory, a directory with no skills, the canonical store, the fleet-home fallback, a path inside fleet home, an already tracked path, a nested path (ancestor or descendant), and a skill name that collides with another custom home.
- [x] On success it links every skill into every installed harness, syncs, and prints one headline plus one linked line per harness per skill.
- [x] Re-running on the same cleaned path is a no-op.
- [x] A name collision with the canonical store is accepted and left for doctor.
- [x] No git binary is consulted on any path.

## Comments

- The already-tracked rule has two shapes: re-running `add-dir` on a path
  already in `skillsDirs` is the no-op of criterion 4 (prints "already
  tracked", writes nothing), while a path already tracked through the
  transitional legacy repo list is refused with a specific message. That
  way "refuses an already tracked path" and "re-running is a no-op" both
  hold without double-scanning a legacy collection.
- Validation, `~`/relative resolution, and the append all live in
  `trackedset.Add`; the command layer only prints and syncs. Collision
  checks cover every custom home (`skillsDirs`, transitional legacy
  collections, and the fallback); the canonical store is deliberately not
  checked, so a shadow is accepted and left to doctor.
