// Package buildinfo carries build-time injected metadata. `go install`
// builds keep the fallback; `make build` and goreleaser overwrite Version
// via -ldflags -X, so the binary can say where it came from.
package buildinfo

// Version is fleet's version, stamped at build time with
// -ldflags "-X github.com/zzacong/fleet/internal/buildinfo.Version=<v>".
var Version = "dev"
