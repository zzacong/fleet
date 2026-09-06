# 04: First release, bootstrap publish, and four-door verification

**What to build:** `v0.1.0` exists as a tag and GitHub Release that no human created by hand, installable four ways on a clean machine — with the one-time manual bootstrap behind us and full automation armed for every release after.

**Blocked by:** 03 (Release workflows and install script). Also gated by the owner's push go-ahead and the one-time bootstrap steps below (both human).

**Status:** ready-for-agent

- [ ] A trivial `fix:` commit opens the first Release PR; merging it creates tag `v0.1.0` plus the GitHub Release with no manual `git tag`.
- [ ] One-time bootstrap (owner, from local with personal auth): stamp the five packages to the tag version, `npm publish` each to create the registry entries, then register the trusted publisher (repo `fleet`, workflow `release.yml`) on all five.
- [ ] `v0.1.0` verified from a clean machine through all four doors — `go install`, `npx -y @zzacong/fleet`, `install.sh`, manual tarball — with `--version` agreeing everywhere and `CHANGELOG.md` present as a byproduct.
- [ ] Full automation is armed: the next version bump publishes with zero manual steps (expected proof vehicle is an early follow-up fix → `v0.1.1`; not separately ticketed).
