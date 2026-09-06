# 06: Final docs sweep

**What to build:** No doc anywhere in the repo contradicts the shipped reality — a last pass over everything installation-adjacent after the install pages land.

**Blocked by:** 05 (Installation docs).

**Status:** ready-for-agent

- [ ] Every remaining install/path/version reference across the root README, the docs site, and contributor docs matches what ticket 04 proved; no stale `apps/` paths; no hand-maintained version numbers or changelog entries anywhere (CHANGELOG and release notes stay tool-owned).
- [ ] Known wart stands as-is: root `docs/` (contributor markdown) versus `www/` (Starlight site) naming is confusing but out of scope — no rename in this ticket.
- [ ] Work stays on a local branch until the owner lifts the no-push instruction.
