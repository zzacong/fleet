# 05: Retire the single pointer (contract)

**What to build:** The old single repo-root pointer is fully gone from code and user-facing surfaces: every consumer reads the tracked set, and help, examples, completions, and error hints tell the multi-repo story with CI green.

**Blocked by:** 02 (`skill pull` end-to-end), 03 (Scan + ls + doctor across the tracked set), 04 (Adopt destination).

**Status:** resolved

- [x] No command, scan, adopt, doctor, or sync path reads the old key; old-key-only configs behave as unset (fallback display), with no automatic migration in code.
- [x] Help text, examples, completions, and error hints reference the list/target model and the auto-tracked convention; no stale single-pointer wording remains in CLI output.
- [x] Full test suite passes with the old key absent from fixtures except a single superseded-key test if needed.

## Comments

- Implemented on branch `ticket/05-retire-pointer`.
- Config seam (`apps/cli/internal/config`): `skillsRepo` field, accessors, `EffectiveRepo`, and the `skills-repo` `NormalizeKey` pair removed; the old file key loads as a preserved-verbatim unknown (never interpreted, never validated, never migrated), so old-key-only configs behave as unset. Canonical render order is now `skillsRepos, adoptTarget`, unknowns sorted.
- Paths seam (`apps/cli/internal/paths`): `Repo`, `WithRepo`, `RepoSkills`, `loadSkillsRepo` removed; `FromEnv` resolves the home only. `TrackedRepos` keeps the env override prepended (code only, undocumented in help).
- Snapshot seam (`apps/cli/internal/snapshot`): legacy repo scan removed; precedence is tracked order, then fleet-home checkouts alphabetical, then fallback, then canonical.
- Doctor seam (`apps/cli/internal/doctor`): legacy collection removed from the custom index and the drift analysis; dead `isManagedRepoLink` helper removed. Kind comments updated to the tracked-set vocabulary.
- Customs seam (`apps/cli/internal/customs`): legacy `Adopt` removed; `customHomes` is exactly the adopt candidates (tracked collections plus fallback).
- CLI (`apps/cli/internal/cli`): adopt drops the legacy-pointer fallback; update's custom check scans every custom home via `AdoptCandidates`; `config get/set/unset` cover `adopt-target` only with the retired `skills-repo`/`skillsRepo` failing with a retired-key hint toward pull + `adopt-target`; `config list` shows `skills-repos` lines plus `adopt-target` (JSON: `skillsRepos` array + `adoptTarget`); completions offer only `adopt-target`; no `FLEET_REPO` or single-pointer wording remains in help/examples.
- Tests: old-key file fixtures removed everywhere except the single `TestSupersededSinglePointerKeyBehavesAsUnset`; legacy-`Adopt` tests migrated to `AdoptTo`, `WithRepo` fixtures migrated to explicit-list config (fake repos carry `.git` where doctor runs), CLI adopt fixtures set the adopt target so bare runs stay prompt-free.
- Tests: `go -C apps/cli test -count=1 ./...` all green; `go vet ./...` clean; `gofmt` clean; `golangci-lint` shows only the two pre-existing issues on the base commit (pull_test gofumpt, cli/pull_test unused field, both untouched).
