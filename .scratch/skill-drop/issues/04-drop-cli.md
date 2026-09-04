# 04: `skill drop` CLI wiring

**What to build:** `fleet skill drop <path-or-name> [--force]` in `internal/cli`: calls the 03 operation, then 01 unwire + 02 unlink for the dropped collection, then sync; prints `drop: removed "<path>"` plus unwired/unlinked lines in the `pull:`/`adopt:` palette style.

**Blocked by:** 01, 02, 03.

**Status:** resolved

- [x] Cobra command with one required arg, `--force` flag (dirty-check override only), shell completion for dirs, help text and examples.
- [x] Runs unwire + unlink + sync after a successful drop; sync output preserved.
- [x] Failure modes surface cleanly: unknown target, dirty tree, adoptTarget conflict, missing git.
- [x] CLI-level tests with stubbed runner over a fake home.
