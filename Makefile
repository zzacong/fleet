# Fleet: per-harness agent skill manager.
# Go targets; JS/TS/MD/Astro formatting lives in `pnpm run fmt` (oxfmt + prettier).

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILDINFO := github.com/zzacong/fleet/internal/buildinfo.Version
GOPKGS := ./...

.PHONY: build test fmt lint check

build:
	go -C apps/cli build -trimpath -ldflags "-X $(BUILDINFO)=$(VERSION)" -o ../../bin/fleet ./cmd/fleet

test:
	go -C apps/cli test $(GOPKGS)

fmt:
	go -C apps/cli run mvdan.cc/gofumpt@v0.11.0 -l -w .

lint:
	golangci-lint run ./apps/cli/...

check: fmt test lint build
