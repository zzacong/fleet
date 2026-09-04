# skill-drop

Status: resolved

`fleet skill drop <path-or-name>` removes a versioned customs home from the tracked set. Inverse of `pull`, not of `adopt`.

## Decisions

- One required arg: repo-root path or fleet-home slot name, resolved against `TrackedRepos()`. No bare/prompt mode in v1 (destructive verb).
- Explicit repos (outside fleet home): remove the entry from `skillsRepos` in `config.json`, preserving order. Never touch disk.
- Fleet-home checkouts (`~/.config/fleet/repos/<name>`): delete the checkout directory from disk. Presence = tracked, so deletion is the untrack.
- Dirty tree fails surfacing state; `--force` overrides (fleet never stashes). Missing git skips the dirty check with a warning.
- `adoptTarget` pointing inside the dropped repo always fails with a re-point hint — no auto-clear, even with `--force`.
- Harness cleanup is new code: `UnwireSkillSource` (opencode/pi) + `RemoveCustomLinks` (codex/claude/cursor/bob, only symlinks resolving under the dropped collection). Then sync, as every command does.
- Output follows the `pull:`/`adopt:` convention: `drop: removed "<path>"` plus unwired/unlinked lines.
