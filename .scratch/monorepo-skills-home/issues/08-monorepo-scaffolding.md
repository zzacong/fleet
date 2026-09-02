# 08: Monorepo scaffolding — `apps/cli` + `go.work` + `pnpm-workspace` + `apps/desktop` placeholder

**What to build:** The repo becomes a monorepo without breaking `go test`, `make build`, or `go install`.

**Blocked by:** 06: Update all docs for the new custom-home behavior, 07: Watcher fleet-home coverage — manual verification

**Status:** ready-for-agent

- [ ] Go CLI moves to `apps/cli` (`cmd/fleet`, `internal/` and `go.mod` move with it); root `go.work` contains `use ./apps/cli`; `module` stays `github.com/zzacong/fleet` inside `apps/cli/go.mod`
- [ ] `goreleaser` main becomes `./apps/cli/cmd/fleet`, `Makefile` `GOPKGS` and `build`/`test`/`vet`/`fmt`/`lint` run via the workspace, CI updated to build `apps/cli`
- [ ] `pnpm-workspace.yaml` at root with `packages: ["apps/*"]`, root `package.json` remains private workspace root
- [ ] `apps/desktop/README.md` placeholder reserves the future GUI name and path; no GUI code yet
- [ ] `skills/` at the repo root is created as an empty skills collection placeholder (so `skillsRepo/skills` resolution has a target right after the move)
- [ ] `bin/fleet` still builds, `make check` (fmt, vet, test, lint, build) is green, and `go install` via the workspace still produces a working binary; no Starlight work in this ticket
