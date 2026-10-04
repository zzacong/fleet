package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/state"
)

// runPrune runs `fleet skill prune` with the given args, returning stdout,
// stderr, and the command error.
func runPrune(t *testing.T, p *paths.Paths, args ...string) (string, string, error) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"skill", "prune"}, args...))
	stdoutTTY = func() bool { return false }
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	t.Setenv("FLEET_REPO", "")
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func writePruneFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// disablePrune records a state disable for the named harnesses.
func disablePrune(t *testing.T, p *paths.Paths, name string, harnesses ...string) {
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

func TestPruneWithoutYesListsAndChangesNothing(t *testing.T) {
	p := toggleHome(t)
	disablePrune(t, p, "ghost", "opencode", "pi")
	writePruneFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)
	writePruneFile(t, p.PiSettings(), `{"skills": ["-skills/ghost/SKILL.md"]}`)
	beforeConfig := readFile(t, p.OpenCodeConfig())
	beforeState := readFile(t, p.FleetStateFile())

	out, _, err := runPrune(t, p)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	for _, want := range []string{
		`prune: opencode: would remove "ghost" (config)`,
		`prune: pi: would remove "ghost" (config)`,
		`prune: opencode: would remove "ghost" (state)`,
		`prune: pi: would remove "ghost" (state)`,
		"re-run with --yes",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if got := readFile(t, p.OpenCodeConfig()); got != beforeConfig {
		t.Errorf("dry run changed the config:\n%s", got)
	}
	if got := readFile(t, p.FleetStateFile()); got != beforeState {
		t.Errorf("dry run changed the state:\n%s", got)
	}
}

func TestPruneYesAppliesAndSyncs(t *testing.T) {
	p := toggleHome(t)
	disablePrune(t, p, "ghost", "opencode", "pi")
	writePruneFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)
	writePruneFile(t, p.PiSettings(), `{"skills": ["-skills/ghost/SKILL.md"]}`)
	// A redundant link proves the post-prune sync ran.
	writeSkillDir(t, p.SkillsStore(), "tdd2", "Second skill.")
	if err := os.MkdirAll(p.OpenCodeSkills(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(p.SkillsStore(), "tdd2"), filepath.Join(p.OpenCodeSkills(), "tdd2")); err != nil {
		t.Fatal(err)
	}

	out, _, err := runPrune(t, p, "--yes")
	if err != nil {
		t.Fatalf("prune --yes: %v", err)
	}
	for _, want := range []string{
		`prune: opencode: removed "ghost" (config)`,
		`prune: opencode: removed "ghost" (state)`,
		`sync: opencode: removed redundant link "tdd2"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if body := readFile(t, p.OpenCodeConfig()); strings.Contains(body, `"ghost"`) {
		t.Errorf("opencode rule survived --yes:\n%s", body)
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if st.IsDisabled("ghost", "opencode") || st.IsDisabled("ghost", "pi") {
		t.Error("state entry survived --yes")
	}
}

func TestPruneHarnessLimitsBothAxes(t *testing.T) {
	p := toggleHome(t)
	disablePrune(t, p, "ghost", "opencode", "pi")
	writePruneFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)
	writePruneFile(t, p.PiSettings(), `{"skills": ["-skills/ghost/SKILL.md"]}`)

	out, _, err := runPrune(t, p, "--harness", "opencode", "--yes")
	if err != nil {
		t.Fatalf("prune --harness: %v", err)
	}
	if !strings.Contains(out, `prune: opencode: removed "ghost" (config)`) {
		t.Errorf("output missing the opencode removal:\n%s", out)
	}
	if strings.Contains(out, `pi: removed`) {
		t.Errorf("pi was pruned despite the filter:\n%s", out)
	}
	if body := readFile(t, p.PiSettings()); !strings.Contains(body, "-skills/ghost/SKILL.md") {
		t.Errorf("pi rule was pruned despite the filter:\n%s", body)
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDisabled("ghost", "pi") {
		t.Error("pi state entry was pruned despite the filter")
	}
}

func TestPruneConfigOnlyAndStateOnlyAreMutuallyExclusive(t *testing.T) {
	p := toggleHome(t)
	_, _, err := runPrune(t, p, "--config-only", "--state-only", "--yes")
	if err == nil {
		t.Fatal("config-only + state-only should fail")
	}
	if !strings.Contains(err.Error(), "config-only") && !strings.Contains(err.Error(), "state-only") {
		t.Errorf("error = %v, want the mutually-exclusive flags named", err)
	}
}

func TestPruneConfigOnlyIsNotRewrittenBySync(t *testing.T) {
	p := toggleHome(t)
	disablePrune(t, p, "ghost", "opencode")
	writePruneFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)

	_, _, err := runPrune(t, p, "--config-only", "--yes")
	if err != nil {
		t.Fatalf("prune --config-only: %v", err)
	}
	if body := readFile(t, p.OpenCodeConfig()); strings.Contains(body, `"ghost"`) {
		t.Errorf("sync re-wrote the config-only prune:\n%s", body)
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDisabled("ghost", "opencode") {
		t.Error("config-only pruned the state entry")
	}
}

func TestPruneStateOnlyLeavesConfigRule(t *testing.T) {
	p := toggleHome(t)
	disablePrune(t, p, "ghost", "opencode")
	writePruneFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)

	_, _, err := runPrune(t, p, "--state-only", "--yes")
	if err != nil {
		t.Fatalf("prune --state-only: %v", err)
	}
	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, `"ghost": "deny"`) {
		t.Errorf("state-only pruned the config rule:\n%s", body)
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if st.IsDisabled("ghost", "opencode") {
		t.Error("state-only left the state entry")
	}
}

func TestPruneIncompleteScanNamesBlockerAndChangesNothing(t *testing.T) {
	p := toggleHome(t)
	missing := filepath.Join(t.TempDir(), "moved-repo")
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	f.SetSkillsRepos([]string{missing})
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}
	disablePrune(t, p, "ghost", "opencode")
	writePruneFile(t, p.OpenCodeConfig(), `{"permission": {"skill": {"ghost": "deny"}}}`)
	beforeConfig := readFile(t, p.OpenCodeConfig())
	beforeState := readFile(t, p.FleetStateFile())

	out, _, err := runPrune(t, p, "--yes")
	if err != nil {
		t.Fatalf("prune --yes: %v", err)
	}
	if !strings.Contains(out, filepath.Join(missing, "skills")) {
		t.Errorf("output does not name the blocking home:\n%s", out)
	}
	if got := readFile(t, p.OpenCodeConfig()); got != beforeConfig {
		t.Errorf("incomplete scan changed the config:\n%s", got)
	}
	if got := readFile(t, p.FleetStateFile()); got != beforeState {
		t.Errorf("incomplete scan changed the state:\n%s", got)
	}
}

func TestPruneCleanHomeReportsNothingToDo(t *testing.T) {
	p := toggleHome(t)
	out, _, err := runPrune(t, p)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if !strings.Contains(out, "nothing to prune") {
		t.Errorf("clean home output = %q, want nothing to prune", out)
	}
}

func TestPruneHelpMentionsFlagsAndExample(t *testing.T) {
	p := toggleHome(t)
	out, err := runBare(t, p, "skill", "prune", "--help")
	if err != nil {
		t.Fatalf("prune --help: %v", err)
	}
	for _, want := range []string{"--yes", "--harness", "--config-only", "--state-only", "fleet skill prune"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q:\n%s", want, out)
		}
	}
}
