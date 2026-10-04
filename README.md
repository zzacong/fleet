# Fleet

Fleet manages agent skills across AI coding agents — OpenCode, Codex, Claude Code, and more. Turn a skill off for one harness without uninstalling it: the [`skills` CLI](https://github.com/vercel-labs/skills) stays the install and update backend, and the canonical store (`~/.agents/skills`) is never moved or edited. Fleet keeps a state file as the source of truth and projects it into each harness's own config.

![fleet demo: list skills, disable one for a harness, toggle in the matrix](www/public/demo.gif)

| `fleet skill ls` + `off`                         | Interactive matrix (`fleet`)                             |
| ------------------------------------------------ | -------------------------------------------------------- |
| ![fleet skill ls output](www/public/demo-ls.png) | ![fleet skill × harness matrix](www/public/demo-tui.png) |

Regenerate with `pnpm --filter www demo:render` (`www/scripts/demo/render.mjs` renders the SVG terminal windows; content mirrors the `fleet skill ls` shape in `www/src/content/docs/cli.md` and the matrix glyphs in `internal/tui/view.go`).

## Supported OS

| OS      | Supported |
| ------- | --------- |
| macOS   | ✅        |
| Linux   | ✅        |
| Windows | ❌        |

Fleet ships arm64 and x86_64 builds for macOS and Linux.

## Supported harnesses

| Harness     | Per-skill off switch |
| ----------- | -------------------- |
| OpenCode    | ✅                   |
| Pi          | ✅                   |
| Codex       | ✅                   |
| Claude Code | ✅                   |
| Cursor      | ❌                   |
| IBM Bob     | ❌                   |

Cursor and Bob read the canonical store natively and have no per-skill off
switch; fleet says so instead of pretending. See the [per-harness
reference](www/src/content/docs/harnesses.md) for detection paths, written
shapes, and limitations.

## Installation

Three ways to install the same single static binary with no runtime
dependencies: a shell script, npm, or Go. Release builds report their version
through `fleet --version`; `go install` builds report `dev`. Every variant is
on the [Installation](https://fleet.zzacong.com/installation/) docs page.

```sh
curl -fsSL https://raw.githubusercontent.com/zzacong/fleet/main/install.sh | sh   # script → ~/.local/bin
```

```sh
npx -y @zzacong/fleet --version   # npm: try it without installing (pnpm dlx, bunx too)
npm i -g @zzacong/fleet           # …or install globally (pnpm add -g, bun add -g)
```

```sh
go install github.com/zzacong/fleet/cmd/fleet@latest
```

## The three-command tour

```sh
fleet skill ls          # what is installed, where it is active, what is stale
fleet skill off tdd     # turn a skill off for every harness (--harness to pick one)
fleet                   # the interactive skill × harness matrix
```

Bare `fleet` opens the matrix: one screen, every skill against every installed harness, toggles staged and applied together, and `u` running a one-key update-all — the same wrapped `skills update` the `skill update` verb runs, with the matrix reloading when it lands. It needs a terminal — piped output falls back to the `fleet skill ls` listing.

The remaining verbs round out the loop: `skill on` re-enables, `skill update` wraps `skills update` and keeps disabled skills disabled, `skill add-dir` and `skill remove-dir` track and untrack your own collection dirs, `skill adopt` migrates custom skills into a collection, `skill doctor` explains anything fleet would change, `skill sync` repairs drift on demand, `skill prune` clears the leftover config rules and state entries for skills you uninstalled, and `harness ls` shows which of the six supported harnesses are installed and which have a per-skill off switch. A disable survives an uninstall as dormant intent — reinstalling comes back disabled — but fleet stops writing it into harness configs until then. Every verb is documented with examples in the [command reference](www/src/content/docs/cli.md).

## Custom skill dirs

```sh
fleet skill add-dir ~/Developer/projects/agent-skills/skills     # register a collection you already have
fleet skill remove-dir ~/Developer/projects/agent-skills/skills  # untrack it, leave the files alone
```

Fleet tracks an ordered list of collection dirs — the `skillsDirs` key in `~/.config/fleet/config.json`. A collection dir's immediate children are skill dirs, each holding a `SKILL.md`. `~` expands to your home directory and a relative path resolves against the working directory; the cleaned absolute path is stored, so re-running `add-dir` on the same dir is a no-op. `add-dir` appends to the end of the list (it never reorders), links every skill it holds into every installed harness, and syncs. `remove-dir` unlists the dir, removes its managed links, and syncs — it never deletes the directory or anything in it. Fleet never clones, pulls, or deletes a collection: you check the repo out with plain git and edit it where it lives.

## Custom vs installed

Two kinds of skill show up in `fleet skill ls`, and the split decides how each one travels:

- **Installed** skills came from a source repo through the `skills` CLI and have provenance (source, hash) in its lockfile. They live in the canonical store (`~/.agents/skills`), `ls` groups them by source repo, and the update badge compares the recorded hash against the repo's current tree. Install and update them with the `skills` CLI, never by hand.
- **Custom** skills are your own: no lockfile entry. They live in the tracked set — the collection dirs listed in `skillsDirs` plus the unversioned fleet-home fallback `~/.config/fleet/skills/` — never through the canonical store, so a custom skill can't be shadowed by a store link. Every harness discovers them through a managed symlink in its own skills directory. When a name exists in more than one source, `ls` shows it once with precedence `skillsDirs` order, then the fallback, then the canonical store; `fleet skill doctor` reports every collision as double presence, grouped by harness and by the pair of homes.

`fleet skill adopt <name>` is the bridge: it moves a custom skill out of the canonical store into the adopt destination — `--into <skills-dir>` for one run, else the configured target (`fleet config set adopt-target ~/Developer/customs/skills`), else a numbered prompt over the tracked collection dirs plus the always-offered fleet-home fallback — links the skill into every installed harness. It also promotes a forked installed skill. Adoption is reversible by hand — see [undo and escape hatches](www/src/content/docs/undo.md).

## What fleet changes on disk

For a canonical-store skill, fleet writes each harness's own native off-entry: OpenCode a deny rule in `opencode.jsonc`, Pi a force-exclude in `settings.json`, Codex an `enabled = false` block in `config.toml`, Claude Code an `off` override in `settings.json`. Cursor and Bob read the canonical store natively and have no per-skill off switch; fleet says so instead of pretending. Custom skills reach every harness through a managed symlink in that harness's own skills directory, pointing at the skill directory in its custom home. On OpenCode, Codex, Pi, Cursor, and Bob, which link skills and scan the canonical store natively, that link is also the custom's enable/disable lever: off removes it, on recreates it. Claude Code is the exception: it links skills but does not scan the store natively, so it toggles both custom and canonical skills through `skillOverrides`. The link always points inside a home `ls` scans, never the canonical store, so sync never treats it as redundant. Skills' files are never moved, renamed, or edited aside from an `adopt` move — [per-harness reference](www/src/content/docs/harnesses.md) has the full list of touched files, written shapes, and untouched ground.

## Shell completions

```sh
source <(fleet completion zsh)   # also: bash, fish, powershell
```

## Documentation

| Document                                                   | Audience                                                    |
| ---------------------------------------------------------- | ----------------------------------------------------------- |
| [Command reference](www/src/content/docs/cli.md)           | every verb, flag, and error, with examples                  |
| [Per-harness reference](www/src/content/docs/harnesses.md) | files touched, written shapes, limitations                  |
| [Undo and escape hatches](www/src/content/docs/undo.md)    | running `skills` by hand, resetting state, how sync decides |
| [Architecture](docs/architecture.md)                       | state file → adapters → sync, adding a harness              |
| [State file schema](www/src/content/docs/state-file.md)    | versioned, forward-compatible format                        |
| [Testing guide](docs/testing.md)                           | injected homes, fixture tests, `FLEET_HOME` sandboxes       |

## Development

```sh
pnpm run check    # fmt-check + lint + typecheck + Go lint/test — single entrypoint
pnpm run check:ci # lint + typecheck + Go lint/test — what CI runs (no fmt)
pnpm run fmt      # oxfmt + prettier (JS/TS/MD/Astro)
just fmt          # gofumpt (Go only)
just build        # bin/fleet, version stamped from git
```

The [architecture overview](docs/architecture.md) explains how the core fits together, and the [testing guide](docs/testing.md) covers the injected-home rule every test follows.
