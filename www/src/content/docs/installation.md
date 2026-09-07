---
title: Installation
description: Every supported way to install fleet.
---

# Installation

macOS or Linux, arm64 or x86_64. Three ways to install the same single static
binary with no runtime dependencies: a shell script, npm, or Go. Release
builds report their version through `fleet --version`; `go install` builds
report `dev`.

## Script

No toolchain required. Latest release into `~/.local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/zzacong/fleet/main/install.sh | sh
```

Pin a version (leading `v` optional, `0.1.0` == `v0.1.0`):

```sh
curl -fsSL https://raw.githubusercontent.com/zzacong/fleet/main/install.sh | sh -s -- 0.1.0
```

Install somewhere else:

```sh
curl -fsSL https://raw.githubusercontent.com/zzacong/fleet/main/install.sh | INSTALL_DIR=/usr/local/bin sh
```

The script detects OS and architecture, downloads the matching
`fleet_<os>_<arch>.tar.gz` tarball from GitHub Releases, verifies its SHA-256
against the release `checksums.txt`, and copies the binary to `INSTALL_DIR`
(default `~/.local/bin`). It never edits shell rc files — if the install dir
is not on `PATH` it prints an `export PATH=...` hint instead. Full interface:
`sh install.sh [VERSION]` (default `latest`), with `VERSION`, `INSTALL_DIR`,
and `REPO` overridable in the environment.

## npm

Works with npm, pnpm, and bun. The `@zzacong/fleet` package wraps the
prebuilt binary for your OS and architecture.

### Try it without installing

```sh
npx -y @zzacong/fleet --version
pnpm dlx @zzacong/fleet --version
bunx @zzacong/fleet --version
```

Anything after the package name passes through, so
`npx -y @zzacong/fleet skill ls` works the same as the installed binary.

### Install globally

```sh
npm i -g @zzacong/fleet
pnpm add -g @zzacong/fleet
bun add -g @zzacong/fleet
```

This puts `fleet` on your `PATH`. The wrapper depends on four optional
platform packages and your manager installs only the one matching your
machine — reinstall without `--no-optional` if the binary ever goes missing.

## Go

Needs a Go toolchain, and the resulting binary reports `dev` from
`fleet --version` instead of the release version.

```sh
go install github.com/zzacong/fleet/cmd/fleet@latest
```

Pin a version:

```sh
go install github.com/zzacong/fleet/cmd/fleet@v0.1.0
```

## Verify

```sh
fleet --version
```
