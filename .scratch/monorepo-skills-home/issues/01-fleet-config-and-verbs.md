# 01: Fleet config file and `fleet config` verbs

**What to build:** A machine-local fleet config at fleet home that lets a user point fleet at a versioned skills repo from any working directory, without any walk-up.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `~/.config/fleet/config.json` lives beside `state.json` and `tree-cache.json`; `FLEET_HOME` overrides the home root for it like for state
- [x] File is JSON, atomic write (temp+rename), unknown fields preserved verbatim on round-trip, canonical formatting; missing file is not an error (means no skills repo is set)
- [x] Single key `skillsRepo` holds an absolute path to the skills repo root (the directory whose `skills/` is the skills collection); other keys are preserved as unknown for forward compat
- [x] `FLEET_REPO` env overrides the file when both are set; file is the persistent alternative to the env
- [x] CLI verbs work: `fleet config get skills-repo` (prints value or empty when unset), `fleet config set skills-repo <abs-path>` (validates that path exists and contains `.git` — or warns — then writes atomically), `fleet config unset skills-repo`, `fleet config list` and `fleet config list --json` show the current pointer
- [x] No change to state file or harness configs in this ticket; verified by fake-home (`FLEET_HOME=$(mktemp -d)`) round-trip tests for load/save, env-over-file precedence, and missing-file is empty
