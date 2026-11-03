# AGENTS.md

## Agent skills

### Issue tracker

Issues are tracked as GitHub issues in `zzacong/fleet`, using the `gh`
CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default triage label vocabulary (labels match role names exactly). See
`docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` + `docs/adr/` at the repo root. See
`docs/agents/domain.md`.

## Commits

Conventional Commits: `<type>(<optional scope>): <imperative description>`.
PR titles must be conventional too, since squash merge uses them as the
commit message.

Scope by area: `cli` (the Go binary, `cmd/` and `internal/`), `www` (the
docs site). Split a change into focused commits by scope when you can.
