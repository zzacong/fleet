# 03: `fleet skill add-dir <path>` end to end

**What to build:** Register an existing directory of skills and make its skills reach every installed harness, with the validation stated in the spec. `~` expands to the home directory and a relative path resolves against the working directory.

**Blocked by:** 01, 02.

**Status:** ready-for-agent

- [ ] `fleet skill add-dir <path>` resolves `~` and a relative path to a cleaned absolute path, then appends it to `skillsDirs` without reordering existing entries.
- [ ] It refuses, with a specific message, a missing path, a non-directory, a directory with no skills, the canonical store, the fleet-home fallback, a path inside fleet home, an already tracked path, a nested path (ancestor or descendant), and a skill name that collides with another custom home.
- [ ] On success it links every skill into every installed harness, syncs, and prints one headline plus one linked line per harness per skill.
- [ ] Re-running on the same cleaned path is a no-op.
- [ ] A name collision with the canonical store is accepted and left for doctor.
- [ ] No git binary is consulted on any path.
