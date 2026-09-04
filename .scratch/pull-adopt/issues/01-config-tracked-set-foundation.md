# 01: Config + tracked-set foundation (expand)

**What to build:** The machine-local config accepts and round-trips the explicit repo-root list and the adopt-target collection dir alongside the existing single pointer (both forms coexist; nothing migrates yet), and the tracked set (explicit list order, auto fleet-home convention scan, env prepended) resolves deterministically for a fake home.

**Blocked by:** None (can start immediately).

**Status:** resolved

- [x] New list key and scalar key load, validate (absolute paths, home expansion), save atomically with canonical formatting, preserve unknown fields, and honor the existing alias convention.
- [x] Tracked-set resolution returns env entry first when set, then explicit list order, then on-disk fleet-home checkouts alphabetically; inside-vs-outside fleet home classification is unit-verified with fake homes only.
- [x] Old single-pointer reads still work untouched; no consumer is moved onto the new model in this ticket.

## Comments

- Implemented on branch `ticket/01-config-tracked-set`.
- Config seam (`apps/cli/internal/config`): file keys `skillsRepos` (list, order significant, empty omitted) and `adoptTarget` (scalar) coexist with `skillsRepo`; canonical render order `skillsRepo, skillsRepos, adoptTarget`, unknowns sorted; atomic temp+rename save; `NormalizeKey` covers the kebab/camel pairs; `ExpandPath`/`AbsolutePath` cover home expansion + absolute validation. Also fixed a latent blank-line in render when the first key is absent.
- Paths seam (`apps/cli/internal/paths`): `FleetReposDir()` (`~/.config/fleet/repos`), `InsideFleetHome()` (lexical, fake-home tested), `TrackedRepos()` (env prepended, explicit order, fleet-home slots alphabetical, deduped, missing dir is empty not an error).
- No CLI, snapshot, doctor, customs, or docs changes; old single-pointer reads byte-identical.
- Tests: `go -C apps/cli test ./...` all green; `go vet ./...` clean; gofumpt clean.
