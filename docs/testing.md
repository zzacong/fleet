# Testing guide

Fleet touches real config files in real homes, so its tests are built around one rule that admits no exceptions.

## The injected-home rule

Every filesystem path in fleet derives from an injected `*paths.Paths` — a home root plus the repo root. Adapters, sync, doctor, toggle, customs, and the CLI all receive it; nothing below `cmd/fleet` resolves paths on its own, and nothing calls `os.UserHomeDir()`.

Tests exploit that directly:

```go
p := paths.New(filepath.Join(t.TempDir(), "home"))
```

Every test builds a fake home in `t.TempDir()`. Nothing outside the project directory or the temp dir is touched, and the tests run in parallel-safe isolation for free. The env override `FLEET_HOME` exists so the same sandboxing works for manual runs (below).

Two consequences worth keeping:

- **Assert external behavior** — the written config file's content, the returned report — never internal call order. A refactor that reorders writes must not break a test; a behavior change must.
- **The network is a seam, not a dependency.** GitHub calls, `skills` process calls, and git calls are injected vars, stubbed in tests.

## Anatomy of an adapter test

Adapter tests are fixture-driven: write the config file, run the operation, compare the whole file's bytes against the expected text. The full-file comparison is the point — it catches dropped comments, reordered keys, and formatting churn that field-level assertions would miss.

`internal/harness/codex_write_test.go`, slightly trimmed:

```go
// runCodexProject writes fixture (when non-empty), runs Project, and
// returns the report plus the file's exact content afterwards.
func runCodexProject(t *testing.T, home, fixture string, writes []SkillWrite) (WriteReport, string) {
    t.Helper()
    p := paths.New(home)
    if fixture != "" {
        os.MkdirAll(p.CodexDir(), 0o755)
        os.WriteFile(p.CodexConfig(), []byte(fixture), 0o644)
    }
    rep, err := NewCodex(p).Project(writes)
    if err != nil {
        t.Fatalf("Project() error = %v", err)
    }
    body, _ := os.ReadFile(p.CodexConfig())
    return rep, string(body)
}

func TestCodexProjectFlipsExistingFleetBlock(t *testing.T) {
    fixture := `[[skills.config]]
name = "tdd"
enabled = true # keep an eye on this
`
    want := `[[skills.config]]
name = "tdd"
enabled = false # keep an eye on this
`

    rep, got := runCodexProject(t, filepath.Join(t.TempDir(), "home"), fixture,
        []SkillWrite{{Name: "tdd", State: StateOff}})
    if got != want {
        t.Errorf("config =\n%s\nwant\n%s", got, want)
    }
    if len(rep.Changed) != 1 || rep.Changed[0].From != StateOn || rep.Changed[0].To != StateOff {
        t.Errorf("Changed = %v, want one on->off flip", rep.Changed)
    }
}
```

The three moves, in order:

1. **Fixture** — a realistic config, comments and foreign keys included, written into the fake home.
2. **Act** — call the seam method (`Project`, `Read`, `sync.Run`, `Adopt`…), never private helpers.
3. **Assert** — the whole file's bytes, plus the report (`Changed` flips, `Flags` for what was deliberately left alone).

To write an adapter test for a new harness, copy the pattern: a `run<Name>Project` helper, then one test per behavior — block appended, existing block flipped, satisfied disable stays quiet and untouched, enable removes fleet's entries, unmodeled shapes are flagged not touched. `opencode_write_test.go` shows the same suite for the harder JSONC case (dialect detection, comment preservation, foreign-rule flags).

## Where each behavior is tested

| Area                                                                 | Files                                                                                                                                      |
| -------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| Adapter detection, read/write per harness                            | `internal/harness/*_test.go` (one pair per harness, plus `disables_test.go`, `links_test.go`, `wiring_test.go` for cross-harness behavior) |
| State file round-trips, unknown fields, version rules                | `internal/state/state_test.go`                                                                                                             |
| Config list/scalar round-trips, aliases, home expansion, retired key | `internal/config/config_test.go`                                                                                                           |
| Tracked-set resolution (explicit order, checkout scan, env prepend)  | `internal/paths/paths_test.go`                                                                                                             |
| Pull flow via the stubbed git runner, per-repo reports               | `internal/pull/pull_test.go`, `internal/cli/pull_test.go`                                                                                  |
| Snapshot union and precedence across the tracked set                 | `internal/snapshot/*_test.go`                                                                                                              |
| Adopt destination (`--into`, target, prompt, save-back)              | `internal/customs/*_test.go`, `internal/cli/adopt*_test.go`                                                                                |
| Sync drift scenarios (re-created links, manual edits)                | `internal/sync/sync_test.go`                                                                                                               |
| Doctor problem classes, keep/restore                                 | `internal/doctor/doctor_test.go`                                                                                                           |
| Store scan, lockfile parsing, sanitized names                        | `internal/scan/*_test.go`                                                                                                                  |
| Wrapped skills CLI (explicit args, closed stdin, failure output)     | `internal/skillscli/exec_test.go`, `update_test.go`                                                                                        |
| CLI verbs, JSON output, completion                                   | `internal/cli/*_test.go`                                                                                                                   |
| TUI keys, staged apply, pipe fallback                                | `internal/tui/tui_test.go`, `guard_test.go`                                                                                                |

## The seams tests stub

| Var               | Replaces                                    | Used by                                                        |
| ----------------- | ------------------------------------------- | -------------------------------------------------------------- |
| `newSkillsRunner` | the `skills` process (absent in sandboxes)  | `fleet skill update`                                           |
| `newTreeClient`   | the GitHub trees API                        | `ls --json`, `ls`, the TUI's update badges                     |
| `newPullRunner`   | the git binary (never shelled out in tests) | `fleet skill pull` (clone, fast-forward-only)                  |
| `stdoutTTY`       | terminal detection                          | the summary line, the piped `fleet` fallback, the adopt prompt |

## Running the suite

```sh
pnpm run check     # fmt-check + lint + typecheck + Go lint/test — single entrypoint
pnpm run check:ci  # lint + typecheck + Go lint/test — what CI runs (no fmt)
go test ./...      # Go only
go test ./internal/harness -run TestCodex -v # one slice
make check         # fmt + test + lint + build — Go only
```

CI runs `pnpm run check:ci` then `make build` and `pnpm --filter www build`; format checks are `oxfmt --check` / `prettier --check` (no `git diff` guard). Run `pnpm run fmt` (JS/TS/MD/Astro) and `make fmt` (Go) before committing.

## Manual sandbox runs with FLEET_HOME

The same injected-home rule works from the shell. Build, then point `FLEET_HOME` at a throwaway directory — fleet will happily create an empty canonical store, state file, and harness dirs inside it:

```sh
make build
sandbox=$(mktemp -d)

FLEET_HOME=$sandbox ./bin/fleet skill ls          # empty store: "no skills found"
mkdir -p $sandbox/.agents/skills/my-skill         # drop a skill in by hand
printf -- '---\nname: my-skill\ndescription: demo\n---\n' > $sandbox/.agents/skills/my-skill/SKILL.md
mkdir -p $sandbox/.config/opencode                # mark opencode installed
FLEET_HOME=$sandbox ./bin/fleet skill off my-skill
cat $sandbox/.config/opencode/opencode.jsonc      # see the written shape
FLEET_HOME=$sandbox ./bin/fleet skill doctor      # inspect without changing anything
```

`FLEET_HOME` sandboxes fleet, and `skill pull` accepts any git URL — including a local path — so the customs flow works end to end in a sandbox:

```sh
FLEET_HOME=$sandbox ./bin/fleet skill pull /tmp/fake-customs
FLEET_HOME=$sandbox ./bin/fleet skill adopt my-skill --into /tmp/fake-customs/skills
```

One trap: **`FLEET_HOME` sandboxes fleet, not the `skills` CLI.** `fleet skill update` shells out to the real `skills update -g -y`, which ignores `FLEET_HOME` and operates on your actual home. When testing `update` in a sandbox, put a stub `skills` executable earlier on `PATH` first.
