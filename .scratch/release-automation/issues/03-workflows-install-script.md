# 03: Release workflows and install script

**What to build:** Merging to `main` opens Release PRs and pushing a tag ships every artifact — with the whole pipeline proven by dry-runs before anything goes live.

**Blocked by:** 01 (Root-module and www move), 02 (npm layout, launcher, scripts).

**Status:** ready-for-agent

- [ ] `release-please.yml` runs on push to `main` with `release-type: go` and PAT auth; rewritten `release.yml` runs on `v*.*.*` with a GoReleaser job and an OIDC npm job (`id-token: write`, npm ≥ 11.5.1 upgrade step, stamp-after-build ordering, platforms published before the wrapper). The `release.yml` filename is final (npm's trust record will pin it).
- [ ] `install.sh` at root detects OS/arch, downloads the matching tarball, verifies SHA-256 against `checksums.txt`, installs to `~/.fleet/bin` (overridable) with a PATH hint, and never edits shell rc; shellcheck clean.
- [ ] `goreleaser release --snapshot` is green with the archive `name_template` frozen; `npm pack --dry-run` is clean for all five packages.
- [ ] Branch only; merging waits for the owner's push go-ahead (standing instruction: nothing pushes yet).
