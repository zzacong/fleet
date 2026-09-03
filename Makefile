# Fleet: per-harness agent skill manager.
# Go targets; JS/TS/MD/Astro formatting lives in `pnpm run fmt` (oxfmt + prettier).

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILDINFO := github.com/zzacong/fleet/internal/buildinfo.Version
GOPKGS := ./...

.PHONY: build test fmt fmt-check lint check

build:
	go -C apps/cli build -trimpath -ldflags "-X $(BUILDINFO)=$(VERSION)" -o ../../bin/fleet ./cmd/fleet

test:
	go -C apps/cli test $(GOPKGS)

fmt:
	go -C apps/cli run mvdan.cc/gofumpt@v0.11.0 -l -w .

fmt-check:
	@test -z "$$(go -C apps/cli run mvdan.cc/gofumpt@v0.11.0 -l .)" || (echo "Go files need gofumpt: run 'make fmt'" && go -C apps/cli run mvdan.cc/gofumpt@v0.11.0 -l . && exit 1)

lint:
	golangci-lint run ./apps/cli/...

check: fmt test lint build
