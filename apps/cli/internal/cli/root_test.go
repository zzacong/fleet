// Bare `fleet` opens the TUI on a terminal. With piped output there is no
// one to steer the matrix, so fleet prints the same listing `fleet skill
// ls` prints instead — and the hero banner, a TUI-launch thing, never
// appears. The tests stub the TTY probe so the piped path is exercised.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/outdated"
	"github.com/zzacong/fleet/internal/paths"
)

func runBare(t *testing.T, p *paths.Paths, args ...string) (string, error) {
	t.Helper()
	out := &bytes.Buffer{}
	prev := newTreeClient
	newTreeClient = func(*paths.Paths) outdated.TreeClient { return &fakeTrees{} }
	t.Cleanup(func() { newTreeClient = prev })
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	err := root.Execute()
	return out.String(), err
}

func TestBareFleetPipedPrintsTheListingWithoutBanner(t *testing.T) {
	p := toggleHome(t)

	out, err := runBare(t, p)
	if err != nil {
		t.Fatalf("bare fleet: %v", err)
	}

	// The piped fallback is the ls face: same rows, no hero banner.
	if !strings.Contains(out, "tdd") {
		t.Errorf("listing missing skills:\n%s", out)
	}
	if strings.Contains(out, "████") {
		t.Error("piped output shows the hero banner")
	}
}

func TestRootHasQuietFlag(t *testing.T) {
	root := NewRoot(paths.New(t.TempDir()))
	if flag := root.Flags().Lookup("quiet"); flag == nil {
		t.Error("root has no --quiet flag")
	}
}
