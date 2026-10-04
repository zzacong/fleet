package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
)

// legacyScope returns a scope whose one custom home is collection and whose
// one custom skill is "my-notes".
func legacyScope(collection string) LegacyScope {
	return LegacyScope{
		Homes:    []string{collection},
		IsCustom: func(name string) bool { return name == "my-notes" },
	}
}

func writeHarnessFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readHarnessFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestOpenCodeCleanLegacyLeavesNonDenyCustomRules(t *testing.T) {
	// A custom name with an "ask" effect is not an off-entry fleet wrote:
	// it stays, byte for byte.
	p := paths.New(filepath.Join(t.TempDir(), "home"))
	fixture := `{"permission": {"skill": {"my-notes": "ask"}}}`
	writeHarnessFile(t, p.OpenCodeConfig(), fixture)

	removed, err := NewOpenCode(p).CleanLegacy(legacyScope("/repo/skills"))
	if err != nil {
		t.Fatalf("CleanLegacy() error = %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %+v, want none", removed)
	}
	if got := readHarnessFile(t, p.OpenCodeConfig()); got != fixture {
		t.Errorf("config touched:\n%s\nwas\n%s", got, fixture)
	}
}

func TestCodexCleanLegacyLeavesEnabledBlocks(t *testing.T) {
	// enabled = true is not a fleet off-entry, so it stays.
	p := paths.New(filepath.Join(t.TempDir(), "home"))
	fixture := "[[skills.config]]\nname = \"my-notes\"\nenabled = true\n"
	writeHarnessFile(t, p.CodexConfig(), fixture)

	removed, err := NewCodex(p).CleanLegacy(legacyScope("/repo/skills"))
	if err != nil {
		t.Fatalf("CleanLegacy() error = %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %+v, want none", removed)
	}
	if got := readHarnessFile(t, p.CodexConfig()); got != fixture {
		t.Errorf("config touched:\n%s\nwas\n%s", got, fixture)
	}
}

func TestPiCleanLegacyLeavesGlobExclusions(t *testing.T) {
	// The exact fleet exclusion goes; the user's !glob form stays.
	p := paths.New(filepath.Join(t.TempDir(), "home"))
	writeHarnessFile(t, p.PiSettings(),
		`{"skills": ["!my-notes", "-skills/my-notes/SKILL.md"]}`)

	removed, err := NewPi(p).CleanLegacy(legacyScope("/repo/skills"))
	if err != nil {
		t.Fatalf("CleanLegacy() error = %v", err)
	}
	if len(removed) != 1 || removed[0].Entry != "my-notes" || removed[0].Kind != legacyDisableEntry {
		t.Errorf("removed = %+v, want one disable entry for my-notes", removed)
	}
	got := readHarnessFile(t, p.PiSettings())
	if strings.Contains(got, "-skills/my-notes/SKILL.md") {
		t.Errorf("exact exclusion survived:\n%s", got)
	}
	if !strings.Contains(got, `"!my-notes"`) {
		t.Errorf("!glob exclusion was removed:\n%s", got)
	}
}

func TestCleanLegacySkipsUninstalledHarnesses(t *testing.T) {
	// No config dirs: nothing is installed, so nothing is read or written.
	p := paths.New(filepath.Join(t.TempDir(), "home"))

	removed, err := CleanLegacy(p, legacyScope("/repo/skills"))
	if err != nil {
		t.Fatalf("CleanLegacy() error = %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %+v, want none", removed)
	}
}
