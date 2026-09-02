# 06: Update all docs for the new custom-home behavior

**What to build:** Every doc a user or contributor reads reflects the explicit pointer and the two custom homes, before any monorepo move happens.

**Blocked by:** 05: Doctor double-presence for the new custom homes

**Status:** resolved

- [x] `README` "Custom vs installed" and "What fleet changes on disk" sections reflect: custom means fleet-home or designated skills repo's `skills/` (never canonical), `fleet config set skills-repo` one-time setup, no walk-up, skills-repo precedence `skillsRepo > fleet-home > canonical`
- [x] `docs/cli.md` documents `fleet config get/set/unset/list [--json]`, updated `fleet skill adopt` target (skillsRepo when set else fleet-home), and `fleet skill ls` source union; error reference includes the new "no skills repo set" hint
- [x] `docs/architecture.md` package map and diagram reflect the new config package, `paths` resolution `env > config > ""`, and snapshot union
- [x] `docs/state-file.md` stays on `state.json` but adds a cross-reference to `~/.config/fleet/config.json` as the separate machine-local pointer
- [x] `CONTEXT.md` already updated in ADR 0001; verify it still matches the shipped CLI wording after 01–05
- [x] `docs/undo.md` and `docs/harnesses.md` note the new custom target for wiring/links and that sync never removes fleet-home/skills-repo links
- [x] Markdown still passes `oxfmt` and `make check` docs steps; no Go code moves in this ticket
