# 08: Polish & tooling

**What to build:** The finishing pass: completions, version, lint/CI wiring so everything before it stays clean, README stub, and retiring the prototypes now that the real thing exists.

**Blocked by:** 07.

**Status:** ready-for-agent

- [ ] Shell completions (zsh at minimum, bash/fish if cheap via Cobra's generator).
- [ ] `fleet --version` wired to a build-time ldflags version.
- [ ] Makefile targets: `fmt` (gofmt/gofumpt + `pnpm fmt:md`), `lint` (golangci-lint + `pnpm lint:md`), `test`, `build`; CI runs all four. Markdown formatting via oxfmt as a project-local devDependency (minimal `package.json`, already set up); `.scratch/` is excluded by scoping the commands to README/CONTEXT/docs.
- [ ] `goreleaser` config for single-binary builds (macOS arm64 first); `go install` path documented.
- [ ] README stub: what fleet is, install, the three-command tour (`ls`, `off`, TUI).
- [ ] Repo hygiene: no build artifacts committed, `.gitignore` updated for the Go build outputs.
