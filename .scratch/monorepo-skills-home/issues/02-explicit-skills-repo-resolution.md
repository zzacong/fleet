# 02: Explicit skills-repo resolution, no walk-up

**What to build:** Fleet resolves the designated skills repo explicitly, never by walking up to `.git`, so running in an unrelated checkout cannot pick up the wrong collection.

**Blocked by:** 01: Fleet config file and `fleet config` verbs

**Status:** ready-for-agent

- [ ] Resolution order is `FLEET_REPO` env > `~/.config/fleet/config.json: skillsRepo` > `""` (no value). There is no `DiscoverRepo` walk-up on the read path
- [ ] `Paths.Repo` and `RepoSkills()` reflect the resolved skills repo (`<repo>/skills`) when set, empty when unset; `FleetConfigFile()` and `FleetHomeSkills()` accessors exist and are `FLEET_HOME`-aware
- [ ] `FromEnv` reads `config.json` (beside `state.json`) instead of calling `DiscoverRepo`; `DiscoverRepo` helper is removed from the read path (may remain only for `fleet config` suggestion text, never called implicitly)
- [ ] When no skills repo is set, fleet does not error — it simply scans canonical store plus fleet home. Commands that need the repo (`adopt` when it would target the repo) report "no skills repo set — set with `fleet config set skills-repo <path>` or `FLEET_REPO`" rather than "no fleet repo found — run inside the repo"
- [ ] Fake-home tests assert precedence `env > config > ""`, that being in an unrelated git checkout with no config does not set `Repo`, and that `FLEET_REPO` with a relative path is resolved to absolute
