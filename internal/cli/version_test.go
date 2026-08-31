// `fleet --version` prints the build-stamped version. `go install` builds
// keep the `dev` fallback; `make build` and goreleaser overwrite it via
// -ldflags -X. Cobra answers the flag before RunE, so the TUI never starts.

package cli

import (
	"strings"
	"testing"

	"github.com/zacong/fleet/internal/buildinfo"
	"github.com/zacong/fleet/internal/paths"
)

func TestVersionFlagPrintsTheBuildStamp(t *testing.T) {
	out, err := runBare(t, paths.New(t.TempDir()), "--version")
	if err != nil {
		t.Fatalf("fleet --version: %v", err)
	}

	if !strings.Contains(out, buildinfo.Version) {
		t.Errorf("--version output missing version %q:\n%s", buildinfo.Version, out)
	}
}

func TestRootCarriesTheBuildVersion(t *testing.T) {
	root := NewRoot(paths.New(t.TempDir()))
	if root.Version != buildinfo.Version {
		t.Errorf("root version %q, want build-stamped %q", root.Version, buildinfo.Version)
	}
}
