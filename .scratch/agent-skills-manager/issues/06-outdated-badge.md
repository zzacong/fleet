# 06: Outdated badge

**What to build:** Know which skills are stale without running update. Fleet implements its own check — the skills CLI has no check-only mode (`check` is an undocumented alias of `update`) — by comparing each lockfile entry's `skillFolderHash` against the current GitHub tree hash for its source repo. Surfaced in `ls` and consumed by the TUI.

**Blocked by:** 01.

**Status:** ready-for-agent

- [ ] For each installed skill: one GitHub API call per source repo (grouped, conditional requests honored for rate limits), comparing the skill folder's current tree hash against `skillFolderHash`.
- [ ] `fleet skill ls` shows an update-available marker; `--json` includes an `outdated` boolean.
- [ ] Non-GitHub source types (git, local, well-known) are reported as "unknown" rather than guessed.
- [ ] Network failure degrades gracefully: badge shows "unknown", command still exits 0, error is visible not fatal.
- [ ] Results are cached (with a TTL) so repeated `ls` calls don't hammer the API.
- [ ] Unit tests with a stubbed API client; no network in tests.
