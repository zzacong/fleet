# Fleet

Fleet manages agent skills across AI coding agents — opencode, pi, codex, claude code, IBM Bob, and Cursor. Turn a skill off for one harness without uninstalling it: the [`skills` CLI](https://github.com/vercel-labs/skills) stays the install and update backend, and the canonical store (`~/.agents/skills`) is never moved or edited. Fleet keeps a state file as the source of truth and projects it into each harness's own config.

## Install

```sh
go install github.com/zacong/fleet/cmd/fleet@latest
```

Fleet is a single static binary with no runtime dependencies. Release builds report their version through `fleet --version`; `go install` builds report `dev`.

## The three-command tour

```sh
fleet skill ls          # what is installed, where it is active, what is stale
fleet skill off tdd     # turn a skill off for every harness (--harness to pick one)
fleet                   # the interactive skill × harness matrix
```

Bare `fleet` opens the matrix: one screen, every skill against every installed harness, toggles staged and applied together. It needs a terminal — piped output falls back to the `fleet skill ls` listing.

The remaining verbs round out the loop: `skill on` re-enables, `skill update` wraps `skills update` and keeps disabled skills disabled, `skill doctor` explains anything it would change, and `skill adopt` migrates hand-written skills into the repo.

## Shell completions

```sh
source <(fleet completion zsh)   # also: bash, fish, powershell
```

## Development

```sh
make check    # fmt, vet, test, lint, build — everything CI runs
make fmt      # gofumpt + oxfmt (markdown)
make build    # bin/fleet, version stamped from git
```
