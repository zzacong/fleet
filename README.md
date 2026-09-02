# Fleet

Fleet manages agent skills across AI coding agents — opencode, pi, codex, claude code, IBM Bob, and Cursor. Turn a skill off for one harness without uninstalling it: the [`skills` CLI](https://github.com/vercel-labs/skills) stays the install and update backend, and the canonical store (`~/.agents/skills`) is never moved or edited. Fleet keeps a state file as the source of truth and projects it into each harness's own config.

## Install

```sh
go install github.com/zzacong/fleet/apps/cli/cmd/fleet@latest
```

Fleet is a single static binary with no runtime dependencies. Release builds report their version through `fleet --version`; `go install` builds report `dev`.

## The three-command tour

```sh
fleet skill ls          # what is installed, where it is active, what is stale
fleet skill off tdd     # turn a skill off for every harness (--harness to pick one)
fleet                   # the interactive skill × harness matrix
```

Bare `fleet` opens the matrix: one screen, every skill against every installed harness, toggles staged and applied together, and `u` running a one-key update-all — the same wrapped `skills update` the `skill update` verb runs, with the matrix reloading when it lands. It needs a terminal — piped output falls back to the `fleet skill ls` listing.

The remaining verbs round out the loop: `skill on` re-enables, `skill update` wraps `skills update` and keeps disabled skills disabled, `skill doctor` explains anything it would change, `skill sync` repairs drift on demand, and `skill adopt` migrates custom skills into the repo. `harness ls` shows which of the six supported harnesses are installed and which have a per-skill off switch. Every verb is documented with examples in the [command reference](apps/docs/src/content/docs/cli.md).

## Custom vs installed

Two kinds of skill show up in `fleet skill ls`, and the split decides how each one travels:

- **Installed** skills came from a source repo through the `skills` CLI and have provenance (source, hash) in its lockfile. They live in the canonical store (`~/.agents/skills`), `ls` groups them by source repo, and the update badge compares the recorded hash against the repo's current tree. Install and update them with the `skills` CLI, never by hand.
- **Custom** skills are your own: no lockfile entry. They live in the resolved custom home — fleet home `~/.config/fleet/skills/` (unversioned fallback, always scanned when present) or the designated skills repo's `skills/` directory (`<skillsRepo>/skills` when a repo is set) — never through the canonical store, so a custom skill can't be shadowed by a store link. Harnesses discover them through their own config (opencode, pi) or a managed symlink (codex, claude code, Cursor, Bob). Point fleet once with `fleet config set skills-repo ~/Developer/fleet` (`FLEET_REPO` env overrides `~/.config/fleet/config.json: skillsRepo`); there is no walk-up to `.git`, so running in an unrelated checkout never picks up the wrong collection. When a name exists in more than one of the three sources, `ls` shows it once with precedence `skillsRepo > fleet-home > canonical`; `fleet skill doctor` reports every collision as double presence.

`fleet skill adopt <name>` is the bridge: it moves a custom skill out of the canonical store into the resolved home (`<skillsRepo>/skills` when a skills repo is set, otherwise `~/.config/fleet/skills`), wires that home's path into every installed harness, and manages the symlinks. It also promotes a forked installed skill. Adoption is reversible by hand — see [undo and escape hatches](apps/docs/src/content/docs/undo.md).

## What fleet changes on disk

Exactly one thing per harness, in each harness's own config format: opencode gets a deny rule in `opencode.jsonc`, pi a force-exclude in `settings.json`, codex an `enabled = false` block in `config.toml`, claude code an `off` override in `settings.json`. Cursor and Bob read the canonical store natively and have no per-skill off switch; fleet says so instead of pretending. Custom skills add one wiring step: opencode and pi get the resolved home's path as an extra discovery source in the same file and dialect they already use, and link-based harnesses get a managed symlink in their skills dir pointing at the resolved home — always the home `ls` scans (`<skillsRepo>/skills` when one is set, otherwise `~/.config/fleet/skills`), never the canonical store, so sync never removes those links. Skills' files are never moved, renamed, or edited aside from an `adopt` move into the resolved home — [per-harness reference](apps/docs/src/content/docs/harnesses.md) has the full list of touched files, written shapes, and untouched ground.

## Shell completions

```sh
source <(fleet completion zsh)   # also: bash, fish, powershell
```

## Documentation

| Document                                                         | Audience                                                    |
| ---------------------------------------------------------------- | ----------------------------------------------------------- |
| [Command reference](apps/docs/src/content/docs/cli.md)           | every verb, flag, and error, with examples                  |
| [Per-harness reference](apps/docs/src/content/docs/harnesses.md) | files touched, written shapes, limitations                  |
| [Undo and escape hatches](apps/docs/src/content/docs/undo.md)    | running `skills` by hand, resetting state, how sync decides |
| [Architecture](docs/architecture.md)                             | state file → adapters → sync, adding a harness              |
| [State file schema](apps/docs/src/content/docs/state-file.md)    | versioned, forward-compatible format                        |
| [Testing guide](docs/testing.md)                                 | injected homes, fixture tests, `FLEET_HOME` sandboxes       |

## Development

```sh
make check    # fmt, vet, test, lint, build — everything CI runs
make fmt      # gofumpt + oxfmt (markdown)
make build    # bin/fleet, version stamped from git
```

The [architecture overview](docs/architecture.md) explains how the core fits together, and the [testing guide](docs/testing.md) covers the injected-home rule every test follows.
