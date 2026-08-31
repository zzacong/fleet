# 09: Documentation

**What to build:** Real docs, both audiences. User-facing: how to install and use fleet, what it changes in each harness's config, and how to undo anything it does. Dev-facing: how the core works, how to add a harness adapter, the state file schema, and how the sandboxed testing works — written so a fresh agent (or human) can extend fleet without archaeology.

**Blocked by:** 08.

**Status:** ready-for-agent

- [ ] User docs (README or `docs/`): install, quickstart, command reference for every verb with examples, what "custom vs installed" means, what adopt does.
- [ ] Per-harness reference: exactly which file fleet touches, what it writes, what it never touches, and each harness's limitations (Cursor: no per-skill disable; Bob: link-based, never cleaned; opencode: next-session reload).
- [ ] "Undo / escape hatch" section: how to run the `skills` CLI independently, how to reset fleet's state, how sync decides what to touch.
- [ ] Dev docs: architecture overview (state file → adapters → sync), the adapter interface contract (read side, write side, what preservation rules apply per config format), and a step-by-step "add a new harness" guide.
- [ ] State file schema documented (versioned, forward-compatible rules).
- [ ] Testing guide: the injected-home rule, golden files, how to write an adapter test, `FLEET_HOME` for manual sandbox runs.
- [ ] Glossary consistency: docs use `CONTEXT.md` vocabulary (harness, canonical store, sync, doctor, adopt) with no leftover jargon (no "reconcile", "repair" as separate terms).
- [ ] Docs reviewed against actual behavior: every command example is run once before landing.
