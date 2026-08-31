package sync

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zacong/fleet/internal/paths"
	"github.com/zacong/fleet/internal/state"
)

// fakeHome builds a home with the given harnesses' config directories
// present, returning the Paths and the harness names installed.
func fakeHome(t *testing.T, harnesses ...string) *paths.Paths {
	t.Helper()
	p := paths.New(filepath.Join(t.TempDir(), "home"))
	dirs := map[string]string{
		"opencode": p.OpenCodeDir(),
		"pi":       p.PiDir(),
		"codex":    p.CodexDir(),
		"claude":   p.ClaudeDir(),
		"cursor":   p.CursorDir(),
		"bob":      p.BobDir(),
	}
	for _, h := range harnesses {
		if err := os.MkdirAll(dirs[h], 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestRunProjectsDisabledSkillsIntoInstalledHarnesses(t *testing.T) {
	p := fakeHome(t, "opencode", "pi", "codex", "claude")
	writeFile(t, p.OpenCodeConfig(), "{\n  \"model\": \"gpt-5\"\n}\n")
	writeFile(t, p.PiSettings(), "{}\n")
	writeFile(t, p.CodexConfig(), "# codex\n")
	// claude can only disable a skill it can discover: give it a link.
	if err := os.MkdirAll(filepath.Join(p.ClaudeSkills(), "tdd"), 0o755); err != nil {
		t.Fatal(err)
	}

	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "opencode")
	st.SetDisabled("tdd", "pi")
	st.SetDisabled("tdd", "codex")
	st.SetDisabled("tdd", "claude")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var harnesses []string
	var totalChanges int
	for _, r := range reports {
		harnesses = append(harnesses, r.Harness)
		totalChanges += len(r.Changed)
	}
	want := []string{"opencode", "pi", "codex", "claude"}
	if !reflect.DeepEqual(harnesses, want) {
		t.Errorf("reports for %v, want %v", harnesses, want)
	}
	if totalChanges != 4 {
		t.Errorf("total changes = %d, want 4 (one per harness)", totalChanges)
	}

	// Each config carries the harness's own off mechanism now.
	if got := readFile(t, p.OpenCodeConfig()); !strings.Contains(got, `"tdd": "deny"`) {
		t.Errorf("opencode config missing the V1 deny:\n%s", got)
	}
	if got := readFile(t, p.PiSettings()); !strings.Contains(got, "-skills/tdd/SKILL.md") {
		t.Errorf("pi settings missing the exclusion:\n%s", got)
	}
	if got := readFile(t, p.CodexConfig()); !strings.Contains(got, "enabled = false") || !strings.Contains(got, `name = "tdd"`) {
		t.Errorf("codex config missing the disabled block:\n%s", got)
	}
	if got := readFile(t, p.ClaudeSettings()); !strings.Contains(got, `"tdd": "off"`) {
		t.Errorf("claude settings missing the override:\n%s", got)
	}

	// Sync is idempotent: a second run has nothing to say.
	reports, err = Run(p)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("second run reported %+v, want nothing", reports)
	}
}

func TestRunFlagsManualEditsWithoutState(t *testing.T) {
	// The manual-edit drift scenario: no state file, configs hold denies
	// fleet didn't write. Sync leaves every byte alone and flags them.
	p := fakeHome(t, "opencode", "pi")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"manual-skill": "deny"}}}`)
	writeFile(t, p.PiSettings(), `{"skills": ["-skills/manual-skill/SKILL.md"]}`)

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantFlags := map[string][]string{
		"opencode": {"disabled in config but not tracked by fleet's state — left alone"},
		"pi":       {"disabled in config but not tracked by fleet's state — left alone"},
	}
	for _, r := range reports {
		want, ok := wantFlags[r.Harness]
		if !ok {
			t.Errorf("unexpected report for %s: %+v", r.Harness, r)
			continue
		}
		var messages []string
		for _, f := range r.Flags {
			messages = append(messages, f.Message)
			if f.Skill != "manual-skill" {
				t.Errorf("%s flag skill = %q, want manual-skill", r.Harness, f.Skill)
			}
		}
		if !reflect.DeepEqual(messages, want) {
			t.Errorf("%s flags = %v, want %v", r.Harness, messages, want)
		}
		if len(r.Changed) != 0 {
			t.Errorf("%s changed = %v, want none (manual edits are untouched)", r.Harness, r.Changed)
		}
	}

	if got := readFile(t, p.OpenCodeConfig()); got != `{"permission": {"skill": {"manual-skill": "deny"}}}` {
		t.Errorf("opencode config was touched:\n%s", got)
	}
	if got := readFile(t, p.PiSettings()); got != `{"skills": ["-skills/manual-skill/SKILL.md"]}` {
		t.Errorf("pi settings were touched:\n%s", got)
	}
}

func TestRunSkipsUninstalledAndNonWritingHarnesses(t *testing.T) {
	// Only cursor is installed, and it has no write side: sync is a no-op.
	p := fakeHome(t, "cursor")
	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "opencode") // opencode is not installed
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	reports, err := Run(p)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("reports = %+v, want none", reports)
	}
}

func TestRunNeverEditsTheStateFile(t *testing.T) {
	p := fakeHome(t, "opencode")
	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, p.FleetStateFile())

	if _, err := Run(p); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if after := readFile(t, p.FleetStateFile()); after != before {
		t.Errorf("state file changed:\n%s\nwas\n%s", after, before)
	}
}

func TestRunReportsErrorsFromBrokenConfigs(t *testing.T) {
	p := fakeHome(t, "opencode")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {`)
	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(p); err == nil {
		t.Fatal("Run() succeeded on a broken config, want error")
	}
}
