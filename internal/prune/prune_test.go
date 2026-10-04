package prune

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/state"
)

// fakeHome builds a home whose canonical store exists (so the scan is
// complete) with the given harnesses' config directories present.
func fakeHome(t *testing.T, harnesses ...string) *paths.Paths {
	t.Helper()
	p := paths.New(filepath.Join(t.TempDir(), "home"))
	if err := os.MkdirAll(p.SkillsStore(), 0o755); err != nil {
		t.Fatal(err)
	}
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

// disable records a state disable for the given harnesses.
func disable(t *testing.T, p *paths.Paths, name string, harnesses ...string) {
	t.Helper()
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range harnesses {
		st.SetDisabled(name, h)
	}
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}
}

func TestRunRemovesOnlyFleetOwnedShapes(t *testing.T) {
	p := fakeHome(t, "opencode", "pi", "codex", "claude")
	disable(t, p, "ghost", "opencode", "pi", "codex", "claude")

	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny", "ghost-*": "deny"}}}`)
	writeFile(t, p.PiSettings(), `{"skills": ["-skills/ghost/SKILL.md", "!ghost-*"]}`)
	writeFile(t, p.CodexConfig(),
		"[[skills.config]]\nname = \"ghost\"\nenabled = false\n\n"+
			"[[skills.config]]\npath = \"/tmp/other/SKILL.md\"\nenabled = false\n")
	writeFile(t, p.ClaudeSettings(), `{"skillOverrides": {"ghost": "off", "other": "user-invocable-only"}}`)

	rep, err := Run(p, Options{Apply: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Config) != 4 {
		t.Fatalf("config removals = %+v, want one per harness", rep.Config)
	}
	if len(rep.State) != 4 {
		t.Fatalf("state removals = %+v, want one per harness", rep.State)
	}

	// opencode: the exact deny goes, the pattern stays.
	body := readFile(t, p.OpenCodeConfig())
	if strings.Contains(body, `"ghost":`) {
		t.Errorf("opencode exact deny survived:\n%s", body)
	}
	if !strings.Contains(body, `"ghost-*"`) {
		t.Errorf("opencode pattern rule was removed:\n%s", body)
	}
	// pi: the exact exclusion goes, the !glob stays.
	body = readFile(t, p.PiSettings())
	if strings.Contains(body, "-skills/ghost/SKILL.md") {
		t.Errorf("pi exact exclusion survived:\n%s", body)
	}
	if !strings.Contains(body, "!ghost-*") {
		t.Errorf("pi !glob exclusion was removed:\n%s", body)
	}
	// codex: the simple block goes, the path-selector block stays.
	body = readFile(t, p.CodexConfig())
	if strings.Contains(body, `name = "ghost"`) {
		t.Errorf("codex simple block survived:\n%s", body)
	}
	if !strings.Contains(body, "/tmp/other/SKILL.md") {
		t.Errorf("codex path-selector block was removed:\n%s", body)
	}
	// claude: the "off" override goes, the foreign value stays.
	body = readFile(t, p.ClaudeSettings())
	if strings.Contains(body, `"ghost": "off"`) {
		t.Errorf("claude off override survived:\n%s", body)
	}
	if !strings.Contains(body, `"other": "user-invocable-only"`) {
		t.Errorf("claude foreign value was removed:\n%s", body)
	}
	// The state entries are gone.
	if body := readFile(t, p.FleetStateFile()); strings.Contains(body, "ghost") {
		t.Errorf("state still carries the stale entry:\n%s", body)
	}
}

func TestRunRemovesOpenCodeV2ThreeKeyDeny(t *testing.T) {
	p := fakeHome(t, "opencode")
	disable(t, p, "ghost", "opencode")
	writeFile(t, p.OpenCodeConfig(), `{"permissions": [{"action": "skill", "resource": "ghost", "effect": "deny"}]}`)

	rep, err := Run(p, Options{Apply: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Config) != 1 {
		t.Fatalf("config removals = %+v, want the V2 deny rule", rep.Config)
	}
	if body := readFile(t, p.OpenCodeConfig()); strings.Contains(body, "ghost") {
		t.Errorf("V2 deny rule survived:\n%s", body)
	}
}

func TestRunLeavesOpenCodeV2ExtraKeyRule(t *testing.T) {
	p := fakeHome(t, "opencode")
	disable(t, p, "ghost", "opencode")
	writeFile(t, p.OpenCodeConfig(), `{"permissions": [{"action": "skill", "resource": "ghost", "effect": "deny", "note": "mine"}]}`)
	before := readFile(t, p.OpenCodeConfig())

	rep, err := Run(p, Options{Apply: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Config) != 0 {
		t.Errorf("config removals = %+v, want none for an extra-key rule", rep.Config)
	}
	if got := readFile(t, p.OpenCodeConfig()); got != before {
		t.Errorf("extra-key rule was touched:\n%s", got)
	}
}

func TestRunLeavesBlanketRuleUntouched(t *testing.T) {
	p := fakeHome(t, "opencode")
	disable(t, p, "ghost", "opencode")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": "deny"}}`)
	before := readFile(t, p.OpenCodeConfig())

	rep, err := Run(p, Options{Apply: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Config) != 0 {
		t.Errorf("config removals = %+v, want none for a blanket rule", rep.Config)
	}
	if len(rep.Skipped) != 1 || rep.Skipped[0].Skill != "ghost" {
		t.Errorf("skipped = %+v, want the blanket rule named", rep.Skipped)
	}
	if got := readFile(t, p.OpenCodeConfig()); got != before {
		t.Errorf("blanket rule was touched:\n%s", got)
	}
}

func TestRunRemovesStateEntryOnceNothingRemains(t *testing.T) {
	p := fakeHome(t, "opencode", "pi")
	disable(t, p, "ghost", "opencode", "pi")

	rep, err := Run(p, Options{StateOnly: true, Apply: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.State) != 2 {
		t.Fatalf("state removals = %+v, want two", rep.State)
	}
	if body := readFile(t, p.FleetStateFile()); strings.Contains(body, "ghost") {
		t.Errorf("emptied skill entry survived:\n%s", body)
	}
}

func TestRunWithoutApplyChangesNothing(t *testing.T) {
	p := fakeHome(t, "opencode", "pi", "codex", "claude")
	disable(t, p, "ghost", "opencode", "pi", "codex", "claude")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)
	writeFile(t, p.PiSettings(), `{"skills": ["-skills/ghost/SKILL.md"]}`)
	writeFile(t, p.CodexConfig(), "[[skills.config]]\nname = \"ghost\"\nenabled = false\n")
	writeFile(t, p.ClaudeSettings(), `{"skillOverrides": {"ghost": "off"}}`)
	before := map[string]string{
		"opencode": readFile(t, p.OpenCodeConfig()),
		"pi":       readFile(t, p.PiSettings()),
		"codex":    readFile(t, p.CodexConfig()),
		"claude":   readFile(t, p.ClaudeSettings()),
	}
	beforeState := readFile(t, p.FleetStateFile())

	rep, err := Run(p, Options{Apply: false})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Config) != 4 || len(rep.State) != 4 {
		t.Fatalf("report = %+v, want every config rule and state entry listed", rep)
	}
	after := map[string]string{
		"opencode": readFile(t, p.OpenCodeConfig()),
		"pi":       readFile(t, p.PiSettings()),
		"codex":    readFile(t, p.CodexConfig()),
		"claude":   readFile(t, p.ClaudeSettings()),
	}
	for name, want := range before {
		if after[name] != want {
			t.Errorf("dry run changed the %s config:\n%s", name, after[name])
		}
	}
	if got := readFile(t, p.FleetStateFile()); got != beforeState {
		t.Errorf("dry run changed the state:\n%s", got)
	}
}

func TestRunSecondRunIsNoop(t *testing.T) {
	p := fakeHome(t, "opencode", "pi")
	disable(t, p, "ghost", "opencode", "pi")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)
	writeFile(t, p.PiSettings(), `{"skills": ["-skills/ghost/SKILL.md"]}`)

	if _, err := Run(p, Options{Apply: true}); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	rep, err := Run(p, Options{Apply: true})
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if !rep.Empty() {
		t.Errorf("second run = %+v, want nothing to do", rep)
	}
}

func TestRunHarnessFilterLimitsBothAxes(t *testing.T) {
	p := fakeHome(t, "opencode", "pi")
	disable(t, p, "ghost", "opencode", "pi")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)
	writeFile(t, p.PiSettings(), `{"skills": ["-skills/ghost/SKILL.md"]}`)

	rep, err := Run(p, Options{Harnesses: []string{"opencode"}, Apply: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, c := range rep.Config {
		if c.Harness != "opencode" {
			t.Errorf("config removal outside the filter: %+v", c)
		}
	}
	for _, s := range rep.State {
		if s.Harness != "opencode" {
			t.Errorf("state removal outside the filter: %+v", s)
		}
	}
	// pi keeps both its rule and its state entry.
	if body := readFile(t, p.PiSettings()); !strings.Contains(body, "-skills/ghost/SKILL.md") {
		t.Errorf("pi rule was removed despite the filter:\n%s", body)
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDisabled("ghost", "pi") {
		t.Error("pi state entry was removed despite the filter")
	}
}

func TestRunConfigOnlyLeavesState(t *testing.T) {
	p := fakeHome(t, "opencode")
	disable(t, p, "ghost", "opencode")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)

	rep, err := Run(p, Options{ConfigOnly: true, Apply: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.State) != 0 {
		t.Errorf("config-only removed state: %+v", rep.State)
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDisabled("ghost", "opencode") {
		t.Error("config-only pruned the state entry")
	}
}

func TestRunStateOnlyLeavesConfig(t *testing.T) {
	p := fakeHome(t, "opencode")
	disable(t, p, "ghost", "opencode")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)

	rep, err := Run(p, Options{StateOnly: true, Apply: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Config) != 0 {
		t.Errorf("state-only removed config: %+v", rep.Config)
	}
	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, `"ghost": "deny"`) {
		t.Errorf("state-only pruned the config rule:\n%s", body)
	}
}

func TestRunIncompleteScanRemovesNothingAndNamesBlocker(t *testing.T) {
	p := fakeHome(t, "opencode")
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeFile(t, blocker, "not a dir")
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetSkillsDirs([]string{blocker})
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}
	disable(t, p, "ghost", "opencode")
	writeFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)
	beforeConfig := readFile(t, p.OpenCodeConfig())
	beforeState := readFile(t, p.FleetStateFile())

	rep, err := Run(p, Options{Apply: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !slices.Contains(rep.Blocked, blocker) {
		t.Fatalf("blocked = %v, want %s", rep.Blocked, blocker)
	}
	if !rep.Empty() {
		t.Errorf("incomplete scan removed something: %+v", rep)
	}
	if got := readFile(t, p.OpenCodeConfig()); got != beforeConfig {
		t.Errorf("incomplete scan changed the config:\n%s", got)
	}
	if got := readFile(t, p.FleetStateFile()); got != beforeState {
		t.Errorf("incomplete scan changed the state:\n%s", got)
	}
}

func TestRunMissingStoreBlocks(t *testing.T) {
	p := paths.New(filepath.Join(t.TempDir(), "home"))
	if err := os.MkdirAll(p.OpenCodeDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	disable(t, p, "ghost", "opencode")

	rep, err := Run(p, Options{Apply: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !slices.Contains(rep.Blocked, p.SkillsStore()) {
		t.Fatalf("blocked = %v, want the missing store %s", rep.Blocked, p.SkillsStore())
	}
	if !rep.Empty() {
		t.Errorf("missing store still pruned: %+v", rep)
	}
}
