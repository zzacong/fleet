# AGENTS.md

## Agent skills

### Issue tracker

Issues are tracked as local markdown files under `.scratch/<feature-slug>/`. See `docs/agents/issue-tracker.md`.

### Triage labels

Default triage label vocabulary (labels match role names exactly). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.

### Commit messages

release-please (`release-type: go`) is the only version driver. See ADR 0003 + `release-please-config.json`.

Write Conventional Commits so release-please can infer semver: `<type>(<optional scope>): <imperative description>`

- Allowed `<type>`: `feat`, `fix`, `docs`, `chore`, `ci`, `refactor`, `style`, `test`, `perf`, `build`, `revert`, `deps` (enforced by `pr-title.yml`).
- Only `feat`, `fix`, `deps` open a Release PR; everything else is invisible to versioning.
- Breaking change: `feat!:`, `fix!:` or `BREAKING CHANGE:` footer.
- Squash merges use the PR title as the commit message, so the PR title itself must be conventional.
- Pre-1.0 bumps: `feat`/`fix` → patch, breaking → minor (`bump-minor-pre-major`, `bump-patch-for-minor-pre-major`).

Changes limited to `www/` or `skills/` must use `docs(www):` or `docs(skills):`, never `feat` or `fix`. A wrong type ships an empty version bump.
