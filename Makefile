# Fleet: per-harness agent skill manager.
# Go targets; `pnpm run fmt:md` / `lint:md` handle markdown via oxfmt.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILDINFO := github.com/zzacong/fleet/internal/buildinfo.Version
GOPKGS := cmd internal

.PHONY: build test vet fmt lint check

build:
	go build -trimpath -ldflags "-X $(BUILDINFO)=$(VERSION)" -o bin/fleet ./cmd/fleet

test:
	go test ./...

vet:
	go vet ./...

fmt:
	go run mvdan.cc/gofumpt@v0.11.0 -l -w $(GOPKGS)
	pnpm run fmt:md

lint:
	golangci-lint run
	pnpm run lint:md

# Everything CI runs, in one go. fmt rewrites in place, so CI follows with
# `git diff --exit-code` to prove the tree was already formatted.
check: fmt vet test lint build
