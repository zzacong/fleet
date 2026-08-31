# 09: Documentation

**What to build:** Real docs, both audiences. User-facing: how to install and use fleet, what it changes in each harness's config, and how to undo anything it does. Dev-facing: how the core works, how to add a harness adapter, the state file schema, and how the sandboxed testing works — written so a fresh agent (or human) can extend fleet without archaeology.

**Blocked by:** 08.

**Status:** resolved

- [x] User docs (README or `docs/`): install, quickstart, command reference for every verb with examples, what "custom vs installed" means, what adopt does.
- [x] Per-harness reference: exactly which file fleet touches, what it writes, what it never touches, and each harness's limitations (Cursor: no per-skill disable; Bob: link-based, never cleaned; opencode: next-session reload).
- [x] "Undo / escape hatch" section: how to run the `skills` CLI independently, how to reset fleet's state, how sync decides what to touch.
- [x] Dev docs: architecture overview (state file → adapters → sync), the adapter interface contract (read side, write side, what preservation rules apply per config format), and a step-by-step "add a new harness" guide.
- [x] State file schema documented (versioned, forward-compatible rules).
- [x] Testing guide: the injected-home rule, golden files, how to write an adapter test, `FLEET_HOME` for manual sandbox runs.
- [x] Glossary consistency: docs use `CONTEXT.md` vocabulary (harness, canonical store, sync, doctor, adopt) with no leftover jargon (no "reconcile", "repair" as separate terms).
- [x] Docs reviewed against actual behavior: every command example is run once before landing.

## Comments

- **Layout.** README stays the front door (install, quickstart, custom vs installed, adopt, docs index); the depth lives in `docs/`: `cli.md` (command reference, every verb with verified examples + error table), `harnesses.md` (per-harness reference), `undo.md` (escape hatches + sync's decision list), `architecture.md` (package map, adapter contract, preservation rules per config format, add-a-harness guide), `state-file.md` (versioned schema and forward-compatibility rules), `testing.md` (injected-home rule, fixture test anatomy, seams, `FLEET_HOME` recipes). All cross-linked; `pnpm fmt:md`/`lint:md` cover the new files via their existing `docs/` scope.
- **One deliberate divergence from the spec's verb list:** there is no `fleet skill sync` verb. Tickets 02–05 landed sync as a mechanism that runs on every command (and after the wrapped update), which `CONTEXT.md` records as the vocabulary. The docs state this explicitly ("Sync is not a verb" in `cli.md`, with the decision list in `undo.md`) rather than documenting a verb that doesn't exist.
- **"Golden files" in the testing guide are documented as what the code actually does:** fixture → act through the seam → compare the whole file's bytes against the expected text (inline golden strings, e.g. `TestCodexProjectFlipsExistingFleetBlock`), plus report assertions. No separate golden-file directory exists in the implementation.
- **Every command example in the docs was run against a sandbox home** (`FLEET_HOME` under `/private/var/.../T/opencode/fleet-docs`, never the real home) with the binary built in this worktree (`make build` → `fleet version 31d13e4`). Sandbox contents: two skills (`tdd` installed via a lockfile entry with a non-GitHub source so runs stay offline, `git-helper` custom), fake harness dirs for all six harnesses, opencode config carrying a comment + an unknown key + a hand-written `ask` rule, codex config carrying a comment. Verified, with outcomes:
    1. `fleet --version` → `fleet version 31d13e4`.
    2. `fleet skill ls` → table with custom first, per-harness columns, `?` badges, claude `-` (absent, no link); stderr empty.
    3. `fleet skill ls --json` → `harnesses` + `skills`, full provenance on the installed skill, `states` with `"claude": "absent"`, `"outdated": null`.
    4. `fleet skill off tdd` → `sync:` lines for opencode/pi/codex + explicit no-op lines for cursor/bob; opencode JSONC kept the comment, unknown key, and `ask` rule while appending `"tdd": "deny"`; pi got `-skills/tdd/SKILL.md`; codex got an appended `[[skills.config]]` block with the comment untouched; claude wrote nothing (skill absent, nothing to disable) while the state file still records it.
    5. State file after: `{"version":1,"skills":{"tdd":{"harnesses":{…:"off"}}}}` (schema doc's example is this real output).
    6. `fleet skill off tdd --harness codex` (already off) → silent, exit 0 (idempotent).
    7. `fleet skill on tdd --harness codex` → `sync: codex: enabled "tdd" (was off)`; block removed from config.toml.
    8. `fleet skill off tdd --harness cursor` → `cursor: no per-skill disable mechanism — disable "tdd" is a no-op`.
    9. `--harness emacs` → `unknown harness "emacs" (want one of: opencode, pi, codex, claude, cursor, bob)`, exit 1; uninstalled harness in an empty home → `opencode is not installed on this machine`; `off typo-skill` → `skill "typo-skill" not found in …/.agents/skills`.
    10. `fleet skill doctor` → drift + missing-directory findings for the claude state entry with no link; with a skills-CLI-style store link planted in `~/.config/opencode/skills` → "redundant links (sync removes them)" section.
    11. Next `fleet skill ls` → stderr `sync: opencode: removed redundant link "tdd" — …`; link gone. A foreign symlink (pointing outside the store) survived sync untouched.
    12. Manual pi edit disabling an untracked skill → doctor conflict prompt; with piped stdin → `[k]/[r]/[s]` options then `left as is (no input)`.
    13. `FLEET_REPO=<scratch> fleet skill adopt git-helper` → moved into the repo, wired opencode (`skills.paths`) and pi (`skills`), linked codex/claude/cursor/bob; links verified to point at the repo. Re-run → `git-helper is already in the repo — nothing to move`. Outside any repo → `no fleet repo found — run inside the repo or set FLEET_REPO`.
    14. `fleet skill update` with a stubbed `skills` on PATH (the real CLI is installed here and ignores `FLEET_HOME` — the trap is called out in `testing.md`): success → `1 skill in …/.agents/skills (1 installed, 0 custom)` + `disabled: "tdd" for opencode, pi, claude`; failing stub → raw CLI output on stderr + `Error: skills update -g -y: exit status 1`, exit 1.
    15. Bare `fleet` piped → the `ls` listing fallback. `fleet completion zsh` → zsh completion script. Doctor on a fresh home → `no problems found`.
    16. The `testing.md` manual-sandbox recipe was executed step by step as written (empty-store `ls` → `no skills found`; hand-made skill + `off` → fresh `opencode.jsonc` with only the deny rule; `doctor` clean; `adopt` with `FLEET_REPO`).
- **Quality gates:** `make check` passes (gofumpt, oxfmt over README/CONTEXT/docs, `go vet`, full `go test ./...`, golangci-lint 0 issues, `lint:md` clean, build). Glossary grep across README + `docs/`: no "reconcile"/"repair" as separate terms; harness / canonical store / sync / doctor / adopt / state file used per `CONTEXT.md` (the only "apply" references are the TUI's own staged-apply vocabulary and `toggle.Apply`).
