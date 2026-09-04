# 03: Drop domain — resolve, unlist vs delete, guards

**What to build:** The pure drop operation (new `internal/drop` package or inside `internal/pull`): resolve `<path-or-name>` against `TrackedRepos()`, then either unlist (explicit: rewrite `skillsRepos` preserving order) or delete (fleet-home: remove the checkout dir from disk). Guards: dirty tree fails surfacing state unless `--force`; `adoptTarget` inside the target always fails with a re-point hint; non-tracked path errors listing tracked repos.

**Blocked by:** none (harness cleanup wires in at the CLI layer in 04).

**Status:** resolved

- [x] Path or slot-name resolution against the tracked set; unknown target errors with the tracked list.
- [x] Explicit target: config rewrite only, order preserved, disk untouched.
- [x] Fleet-home target: recursive dir removal; nothing outside the fleet home ever deleted.
- [x] Dirty check via the pull `Runner` seam (`git status --porcelain`); `--force` skips it; missing git warns and proceeds.
- [x] `adoptTarget` inside target fails with the `fleet config set adopt-target` hint even with `--force`.
- [x] Stub-runner unit tests for resolve/unlist/delete/guards.
