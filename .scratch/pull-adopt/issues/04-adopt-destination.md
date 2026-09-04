# 04: Adopt destination (`--into`, target key, prompt)

**What to build:** Adopting lands exactly where the user means: a run-scoped flag wins silently, else the configured target wins silently, else an explicit choice (tracked collections plus the always-offered fallback) with opt-in persistence, never hanging automation.

**Blocked by:** 01 (Config + tracked-set foundation).

**Status:** resolved

- [x] `--into <skills-dir>` wins for the run with no prompt and no config write; configured target wins with no prompt; zero tracked collections uses the fallback with no prompt.
- [x] At least one tracked collection prompts with a numbered list that always includes the fallback (one tracked means two options); choice is one-shot.
- [x] Follow-up save-back question defaults to No; piped/non-terminal ambiguous runs fail listing candidates plus the flag hint instead of blocking.
- [x] Config verbs cover the adopt-target key (get/set/unset/list) consistent with the existing key behavior.

## Comments

- Implemented on branch `ticket/04-adopt-destination`.
- Customs seam (`apps/cli/internal/customs/destination.go`): `AdoptCandidates()` (tracked collections in order + fallback always last), `AdoptTo()` (explicit collection dir, created on demand, unscanned targets proceed, no config write, double-presence across the tracked set). Legacy `Adopt()` kept byte-identical via the shared `adoptInto` flow; single pointer not retired.
- CLI (`apps/cli/internal/cli/adopt.go`): `--into` flag (absolute after home expansion, created on demand, suppresses prompts + config write); configured-target and zero-tracked fast paths with no prompt; terminal-only numbered prompt (one-shot, invalid fails) plus save-back defaulting to No; non-terminal ambiguous fails listing candidates with the `--into` hint. Transitional: zero-tracked with legacy `p.Repo` set still adopts into the legacy collection.
- Config verbs (`apps/cli/internal/cli/config.go`): `get/set/unset/list` for `adopt-target`/`adoptTarget` (absolute validation + home expansion on set, no existence requirement, no env override, atomic save, unknown-field preservation; list text + JSON).
- Tests: `go -C apps/cli test ./...` all green; `go vet ./...` clean; `golangci-lint` 0 issues; gofmt clean.
