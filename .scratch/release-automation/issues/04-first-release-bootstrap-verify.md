# 04: First release, bootstrap publish, and four-door verification

**What to build:** `v0.1.0` exists as a tag and GitHub Release that no human created by hand, installable four ways on a clean machine — with the one-time manual bootstrap behind us and full automation armed for every release after.

**Blocked by:** 03 (Release workflows and install script). Also gated by the owner's push go-ahead and the one-time bootstrap steps below (both human).

**Status:** resolved

- [x] A trivial `fix:` commit opens the first Release PR; merging it creates tag `v0.1.0` plus the GitHub Release with no manual `git tag`.
- [x] One-time bootstrap (owner, from local with personal auth): stamp the five packages to the tag version, `npm publish` each to create the registry entries, then register the trusted publisher (repo `fleet`, workflow `release.yml`) on all five.
- [x] `v0.1.0` verified from a clean machine through all four doors — `go install`, `npx -y @zzacong/fleet`, `install.sh`, manual tarball — with `--version` agreeing everywhere and `CHANGELOG.md` present as a byproduct.
- [x] Full automation is armed: the next version bump publishes with zero manual steps (expected proof vehicle is an early follow-up fix → `v0.1.1`; not separately ticketed).

## Resolution log (2026-09-07)

Done by the owner:

- `v0.1.0` released through the pipeline (release PR #1 merged → tag + GitHub
  Release, no manual tag); bootstrap publish + trusted-publisher registration
  completed by hand.
- `v0.1.1` released with zero manual steps — the proof vehicle this ticket
  called for (`fix:` commits → release PR #6 → tag + release + OIDC publish).
  Registry state: npm has `0.1.0` and `0.1.1`, `latest` → `0.1.1`; both tags
  resolvable through the Go module proxy. `CHANGELOG.md` carries both entries
  (release-please-owned).
- 0.1.1 also shipped a launcher hardening found in rehearsal:
  `fix(npm): launcher restores exec bit on platform binary` (matters for
  stale pnpm dlx caches holding a pre-fix copy — see ticket 05 log).

Four-door verification was run post-publish on this machine on 2026-09-07;
full command-by-command log lives in ticket 05.
