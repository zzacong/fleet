# 08: Polish & tooling

**What to build:** The finishing pass: completions, version, lint/CI wiring so everything before it stays clean, README stub, and retiring the prototypes now that the real thing exists.

**Blocked by:** 07.

**Status:** resolved

- [x] Shell completions (zsh at minimum, bash/fish if cheap via Cobra's generator).
- [x] `fleet --version` wired to a build-time ldflags version.
- [x] Makefile targets: `fmt` (gofmt/gofumpt + `pnpm fmt:md`), `lint` (golangci-lint + `pnpm lint:md`), `test`, `build`; CI runs all four. Markdown formatting via oxfmt as a project-local devDependency (minimal `package.json`, already set up); `.scratch/` is excluded by scoping the commands to README/CONTEXT/docs.
- [x] `goreleaser` config for single-binary builds (macOS arm64 first); `go install` path documented.
- [x] README stub: what fleet is, install, the three-command tour (`ls`, `off`, TUI).
- [x] Repo hygiene: no build artifacts committed, `.gitignore` updated for the Go build outputs.

## Comments

- **Prototypes were already retired before this ticket ran.** Commit `da3a493` ("chore(prototypes): retire stack-comparison prototypes after Go decision") deleted `prototypes/` from git, and nothing remained on disk or in history's working tree — verified in this worktree at WAVE_BASE `b924ba4`. No archive branch was created: git history retains every prototype file, and the stack comparison lives on as `.scratch/agent-skills-manager/2026-08-31-tui-stack-comparison.md`.
- Implementation notes: completions come from Cobra's built-in generator (`fleet completion bash|zsh|fish|powershell`), pinned by tests in `internal/cli/completion_test.go`; `--version` reads `internal/buildinfo.Version` (default `dev` for `go install` builds), stamped by `make build` and goreleaser via `-ldflags -X`. `make check` runs fmt + vet + test + lint + build; CI (`.github/workflows/ci.yml`) runs `make check` followed by `git diff --exit-code` so unformatted trees fail. `.golangci.yml` enables the gofumpt formatter so `make lint` rejects what `make fmt` would fix. The goreleaser config (`.goreleaser.yaml`, darwin/arm64 first) was validated with `goreleaser check` and a full snapshot build of the four-target matrix under goreleaser v2.18.0.
