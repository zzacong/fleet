package harness

import (
	"os"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

// hideInConfig writes names into the fleet config under home, as
// `fleet harness hide` would.
func hideInConfig(t *testing.T, p *paths.Paths, names ...string) {
	t.Helper()
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, n := range names {
		f.Hide(n)
	}
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestManagedExcludesHiddenInstalledHarness(t *testing.T) {
	p := paths.New(t.TempDir())
	for _, dir := range []string{p.OpenCodeDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	hideInConfig(t, p, "bob")

	got, err := Managed(p)
	if err != nil {
		t.Fatalf("Managed() error = %v", err)
	}
	if len(got) != 1 || got[0].Harness() != OpenCode {
		t.Errorf("Managed() = %v, want only opencode", harnessNames(got))
	}
	if err := RefuseHidden(p, "bob"); err == nil || !strings.Contains(err.Error(), "fleet harness unhide bob") {
		t.Errorf("RefuseHidden(bob) = %v, want an error pointing at fleet harness unhide bob", err)
	}
	if err := RefuseHidden(p, "opencode"); err != nil {
		t.Errorf("RefuseHidden(opencode) = %v, want nil", err)
	}
}

func harnessNames(adapters []Adapter) []Harness {
	out := make([]Harness, len(adapters))
	for i, a := range adapters {
		out[i] = a.Harness()
	}
	return out
}
