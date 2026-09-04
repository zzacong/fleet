# 03: Scan + ls + doctor across the tracked set

**What to build:** Listing and health reporting cover the full tracked set: one union table with deterministic precedence, customs honestly badged, and every cross-source name collision surfaced as drift.

**Blocked by:** 01 (Config + tracked-set foundation).

**Status:** resolved

- [x] Listing shows the canonical store plus explicit repos in list order plus auto fleet-home checkouts alphabetically plus the unversioned fallback, winner-only per name.
- [x] Customs (anything resolved through a custom home) carry no install provenance and report the outdated badge as unknown.
- [x] Doctor flags every cross-source name collision as double presence and warns on unscanned adopt targets and non-git explicit entries.

## Comments

- Implemented on branch `ticket/03-scan-ls-doctor`.
- Snapshot seam (`apps/cli/internal/snapshot`): `Build` scans `TrackedRepos()` collections (env, explicit list order, fleet-home checkouts alphabetical) plus the fallback plus the legacy repo (deduped, still highest while both forms coexist); union is winner-only per name with precedence explicit-list order, then checkouts alphabetically, then fallback, then canonical; every custom-home name resolves as custom with no provenance and a null outdated badge with no API call.
- Doctor seam (`apps/cli/internal/doctor`): `analyzeRepoSkills` flags every cross-source name collision (canonical, explicit, checkout, env, fallback, legacy) as one `double-presence` finding naming every copy; stale-lock union covers all custom homes with the home named per copy; new `unscanned-adopt-target` warning when the configured target is outside the scanned homes; new `non-git-repo` warning per explicit entry without `.git`. Managed-link suppression (`scanCustomIndex`) covers all custom homes. CLI sections/summary/help follow; `ls` help describes the tracked-set union.
- No pull verb (02), no adopt destination (04), no single-pointer retirement (05).
- Tests: `go -C apps/cli test ./...` all green; `go vet ./...` clean; `gofmt` clean.
