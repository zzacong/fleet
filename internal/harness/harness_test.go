package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zacong/fleet/internal/paths"
)

func mkdir(dir string) error { return os.MkdirAll(dir, 0o755) }

func TestAllCoversTheSixHarnessesInColumnOrder(t *testing.T) {
	p := paths.New(filepath.Join(t.TempDir(), "home"))
	got := All(p)

	want := []Harness{OpenCode, Pi, Codex, Claude, Cursor, Bob}
	if len(got) != len(want) {
		t.Fatalf("All() returned %d adapters, want %d", len(got), len(want))
	}
	for i, a := range got {
		if a.Harness() != want[i] {
			t.Errorf("All()[%d] = %s, want %s", i, a.Harness(), want[i])
		}
	}
}

func TestDetectionProbesTheConfigDirectory(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	adapters := All(p)

	for _, a := range adapters {
		if a.Installed() {
			t.Errorf("%s reported installed with no config dir", a.Harness())
		}
	}

	// Create only some of the config dirs; only those harnesses report
	// installed.
	existing := map[Harness]string{
		OpenCode: p.OpenCodeDir(),
		Pi:       p.PiDir(),
		Bob:      p.BobDir(),
	}
	for _, dir := range existing {
		if err := mkdir(dir); err != nil {
			t.Fatal(err)
		}
	}

	for _, a := range adapters {
		_, probed := existing[a.Harness()]
		if a.Installed() != probed {
			t.Errorf("%s Installed() = %v, want %v", a.Harness(), a.Installed(), probed)
		}
	}
}

func TestMissingConfigFileMeansEverythingOn(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)

	for _, a := range All(p) {
		res, err := a.Read([]string{"tdd", "git-helper"})
		if err != nil {
			t.Fatalf("%s Read() error = %v", a.Harness(), err)
		}
		for _, name := range []string{"tdd", "git-helper"} {
			state := res.States[name]
			if a.Harness() == Claude {
				// Claude discovers through links only; nothing linked yet.
				if state != StateAbsent {
					t.Errorf("%s state for %s = %q, want absent", a.Harness(), name, state)
				}
				continue
			}
			if state != StateOn {
				t.Errorf("%s state for %s = %q, want on", a.Harness(), name, state)
			}
		}
	}
}
