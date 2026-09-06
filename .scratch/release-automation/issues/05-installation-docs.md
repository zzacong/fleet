# 05: Installation docs

**What to build:** A user landing on the README or the docs site finds every supported install method in one place, each one tested exactly as written.

**Blocked by:** 04 (First release, bootstrap publish, verification — docs must describe verified reality, not intentions).

**Status:** ready-for-agent

- [ ] New Installation section in the root README and a matching page on the user docs site, in this order: script install to `~/.fleet/bin` (with `INSTALL_DIR` override and version pin), `npx`, `pnpm dlx`, `bunx`, global installs (`npm i -g`, `pnpm add -g`, `bun add -g`), `go install`. No yarn and no project-local install instructions anywhere.
- [ ] Every documented command is executed verbatim as written (clean machine where feasible), including the override and pin variants.
- [ ] Work stays on a local branch until the owner lifts the no-push instruction.
