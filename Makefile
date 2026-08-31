# Fleet: per-harness agent skill manager.
# Go module targets; `pnpm run fmt:md` / `lint:md` handle markdown.

.PHONY: build test vet fmt lint check

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w cmd internal

lint:
	golangci-lint run

# Everything CI would run, in one go.
check: test vet lint
