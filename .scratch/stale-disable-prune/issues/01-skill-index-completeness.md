# 01: Skill index completeness and installed names

**What to build:** Extend the skill index, the single scanner, so callers can ask two things: is this skill installed, and can that answer be trusted. A skill is installed when it exists in the canonical store or any tracked custom home (the explicit repo list, the fleet-home checkouts, or the fleet-home fallback), matched by directory name or frontmatter name. The completeness signal is true only when the canonical store exists and scans and every tracked collection is present on disk and scans without error. A missing fleet-home fallback is not incomplete, since it is optional by design. A tracked repo root missing from disk, or any per-home scan error, is incomplete.

This is a prefactor with no user surface. Sync, doctor, and prune all consume it so the definition of "installed" and the fail-closed rule cannot drift between them.

**Blocked by:** None (can start immediately).

**Status:** resolved

- [x] The index exposes the set of installed names across the canonical store and every tracked custom home, matched by directory name or frontmatter name.
- [x] The index exposes a completeness signal: true only when the store exists and scans and every tracked collection is present on disk and scans cleanly.
- [x] A missing fleet-home fallback does not mark the scan incomplete.
- [x] A tracked repo root missing from disk marks the scan incomplete.
- [x] A per-home scan error marks the scan incomplete.
- [x] Tests cover each case over a fake home.
