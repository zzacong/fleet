# AGENTS.md

## Agent skills

### Issue tracker

Issues are tracked as local markdown files under `.scratch/<feature-slug>/`. See `docs/agents/issue-tracker.md`.

### Triage labels

Default triage label vocabulary (labels match role names exactly). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.

### Commit messages

release-please cuts a CLI release on any `feat`, `fix`, or `deps` commit. See ADR 0003.

Changes limited to `www/` or `skills/` must use `docs(www):` or `docs(skills):`, never `feat` or `fix`. A wrong type ships an empty version bump.
