# 01: Root-module and www move, desktop deleted

**What to build:** The Go module lives at the repo root under its existing (now truthful) path, the docs site lives at `www/`, and `apps/` is gone — with every build green and no reference left lying about the old layout.

**Blocked by:** None (can start immediately).

**Status:** resolved

- [x] `cmd/fleet`, `internal/*`, and `go.mod`/`go.sum` moved to root; `go.work`/`go.work.sum` deleted; Makefile carries no `-C apps/cli`; GoReleaser `main` points at `./cmd/fleet`; ldflags pointer unchanged and `fleet --version` reports correctly from `make build`.
- [x] Site moved `apps/docs` → `www/` (package renamed `docs`→`www`); root `docs:*` scripts renamed `www:*`; CI site filter updated; `pnpm-workspace.yaml` is an explicit list (`www`, `npm/*`).
- [x] `apps/desktop/` deleted; ADR 0001 amended noting the nested layout is superseded.
- [x] `make build`, the full Go test suite, and the site build are green; no `apps/` references remain in code, workflows, or docs; the README install line is truthful (the full Installation section is ticket 05, not here).
- [x] Work stays on a local branch; nothing is pushed (owner's standing instruction).
