# Fleet: per-harness agent skill manager.
# Go targets; JS/TS/MD/Astro formatting lives in `pnpm run fmt` (oxfmt + prettier).

set shell := ["bash", "-euo", "pipefail", "-c"]

buildinfo := "github.com/zzacong/fleet/internal/buildinfo.Version"
version := env_var_or_default("VERSION", `git describe --tags --always --dirty 2>/dev/null || echo dev`)

# List the available recipes.
default:
    just --list

# Build bin/fleet, version stamped from git (override with VERSION=…).
build:
    go build -trimpath -ldflags "-X {{buildinfo}}={{version}}" -o bin/fleet ./cmd/fleet

test:
    go test ./...

fmt:
    go run mvdan.cc/gofumpt@v0.11.0 -l -w .

# Fail if Go files need gofumpt (no writes).
fmt-check:
    @test -z "$(go run mvdan.cc/gofumpt@v0.11.0 -l .)" || (echo "Go files need gofumpt: run 'just fmt'" && go run mvdan.cc/gofumpt@v0.11.0 -l . && exit 1)

lint:
    golangci-lint run ./...

check: fmt-check test lint build
