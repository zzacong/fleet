# 04: Docs, glossary, and ADR for the unified link model

**What to build:** Documentation matches the new behavior: custom skills are exposed by a managed symlink in every harness; a custom toggles by its link on native scanners while a canonical skill toggles by harness config; Claude Code is the config-toggled exception. A glossary term captures the link, and a new ADR records why.

**Blocked by:** 03.

**Status:** resolved

- [x] CONTEXT.md gains a "Managed custom link" term, retires the config-path-versus-link-based exposure split, and rewrites Sync, Enable/Disable, and Custom-skill around exposure-by-link plus the custom-versus-canonical lever rule.
- [x] The per-harness reference, CLI reference, and undo guide describe uniform linking and the new toggle rule; no wiring narrative remains.
- [x] A new ADR records the decision and its trade-off; existing ADRs are left as history.
- [x] Docs reflect that an unversioned canonical-store skill shown as "custom" stays config-disabled, not link-toggled.
