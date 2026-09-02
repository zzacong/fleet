# Fleet: per-harness agent skill manager.
# Go targets; `pnpm run fmt:md` / `check:md` handle markdown via oxfmt.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILDINFO := github.com/zzacong/fleet/internal/buildinfo.Version
GOPKGS := ./...

.PHONY: build test vet fmt lint check

build:
	go -C apps/cli build -trimpath -ldflags "-X $(BUILDINFO)=$(VERSION)" -o ../../bin/fleet ./cmd/fleet

test:
	go -C apps/cli test $(GOPKGS)

vet:
	go -C apps/cli vet $(GOPKGS)

fmt:
	go -C apps/cli run mvdan.cc/gofumpt@v0.11.0 -l -w .
	pnpm run fmt:md

lint:
	golangci-lint run ./apps/cli/...
	pnpm run check:md

# Everything CI runs, in one go. fmt rewrites in place, so CI follows with
# `git diff --exit-code` to prove the tree was already formatted.
check: fmt vet test lint build
