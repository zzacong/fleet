# 06: Outdated badge

**What to build:** Know which skills are stale without running update. Fleet implements its own check — the skills CLI has no check-only mode (`check` is an undocumented alias of `update`) — by comparing each lockfile entry's `skillFolderHash` against the current GitHub tree hash for its source repo. Surfaced in `ls` and consumed by the TUI.

**Blocked by:** 01.

**Status:** resolved

- [x] For each installed skill: one GitHub API call per source repo (grouped, conditional requests honored for rate limits), comparing the skill folder's current tree hash against `skillFolderHash`.
- [x] `fleet skill ls` shows an update-available marker; `--json` includes an `outdated` boolean.
- [x] Non-GitHub source types (git, local, well-known) are reported as "unknown" rather than guessed.
- [x] Network failure degrades gracefully: badge shows "unknown", command still exits 0, error is visible not fatal.
- [x] Results are cached (with a TTL) so repeated `ls` calls don't hammer the API.
- [x] Unit tests with a stubbed API client; no network in tests.

## Comments

- 2026-08-31 (implementing agent): done on branch `ticket/06-outdated-badge` in new package `internal/outdated`. `Check` classifies every skill into the tri-state (outdated / current / unknown), grouping API calls by source **and ref** (two skills from one repo pinned to different refs need different trees — same trick `skills update` uses). Comparison only runs when the lockfile hash is a real 40-hex tree SHA and `skillPath` is present; anything else is unknown, never a guess (this is stricter than the skills CLI, which would compare any hash shape). Conditional requests: the persistent cache holds the per-repo ETag and revalidates with `If-None-Match` after a 1h TTL — a 304 costs no rate limit (verified live against api.github.com); failed checks are negative-cached for 10 minutes so an offline machine's next `ls` doesn't time out again. The cache is fleet-owned state at `<FLEET_HOME>/.config/fleet/tree-cache.json`; the skills lockfile is still never written. `--json` carries `outdated` as `true`/`false`/`null` — null = unknown (custom, non-GitHub, or failed check), and the key is present for every skill. `fleet skill ls` renders it as the UPDATE column (↑ available, ✓ current, ? unknown) and counts pending updates in the summary line. Live-verified on this machine: 8 source repos → 8 calls, second run zero calls (cache mtime unchanged), a doctored hash flips exactly its skill to ↑, and a dead proxy yields one warning per repo on stderr with exit 0 and every badge ?. HTTP translation is unit-tested against a loopback `httptest` server; all other tests use stub clients — no test touches the network. Deviations worth knowing: `internal/scan` Provenance gained the `skillPath`/`ref` fields the check needs (additive); token support (`GITHUB_TOKEN`/`GH_TOKEN`, matching the skills CLI) raises the 60 req/hr anonymous ceiling.
