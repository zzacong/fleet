package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zacong/fleet/internal/paths"
	"github.com/zacong/fleet/internal/state"
)

// toggleHome builds a home with every harness installed, one stored skill
// ("tdd"), and empty harness configs.
func toggleHome(t *testing.T) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	writeSkillDir(t, p.SkillsStore(), "tdd", "Red-green-refactor workflow.")
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func runToggle(t *testing.T, p *paths.Paths, args ...string) (string, string) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"skill"}, args...))
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	if err := root.Execute(); err != nil {
		t.Fatalf("fleet skill %v: error = %v", args, err)
	}
	return out.String(), errOut.String()
}

func runToggleErr(t *testing.T, p *paths.Paths, args ...string) error {
	t.Helper()
	root := NewRoot(p)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"skill"}, args...))
	return root.Execute()
}

func TestOffRecordsStateAndWritesEachHarnessNativeOff(t *testing.T) {
	p := toggleHome(t)
	// claude can only disable what it can discover: give it a link.
	if err := os.MkdirAll(filepath.Join(p.ClaudeSkills(), "tdd"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, _ := runToggle(t, p, "off", "tdd")

	// State records the pair for every harness fleet can write.
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatalf("state file: %v", err)
	}
	for _, h := range []string{"opencode", "pi", "codex", "claude"} {
		if !st.IsDisabled("tdd", h) {
			t.Errorf("state: tdd/%s not disabled", h)
		}
	}
	if st.IsDisabled("tdd", "cursor") || st.IsDisabled("tdd", "bob") {
		t.Error("state records cursor/bob, which have no write side")
	}

	// Each config carries its native off marker.
	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, `"tdd": "deny"`) {
		t.Errorf("opencode config:\n%s", body)
	}
	if body := readFile(t, p.PiSettings()); !strings.Contains(body, "-skills/tdd/SKILL.md") {
		t.Errorf("pi settings:\n%s", body)
	}
	if body := readFile(t, p.CodexConfig()); !strings.Contains(body, `name = "tdd"`) || !strings.Contains(body, "enabled = false") {
		t.Errorf("codex config:\n%s", body)
	}
	if body := readFile(t, p.ClaudeSettings()); !strings.Contains(body, `"tdd": "off"`) {
		t.Errorf("claude settings:\n%s", body)
	}

	// The report names what changed, and Cursor/Bob get the no-op message.
	for _, want := range []string{
		`sync: opencode: disabled "tdd" (was on)`,
		`sync: pi: disabled "tdd" (was on)`,
		`sync: codex: disabled "tdd" (was on)`,
		`sync: claude: disabled "tdd" (was on)`,
		`cursor: no per-skill disable mechanism — disable "tdd" is a no-op`,
		`bob: no per-skill disable mechanism — disable "tdd" is a no-op`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestOffWithHarnessFlagTouchesOnlyThatHarness(t *testing.T) {
	p := toggleHome(t)

	out, _ := runToggle(t, p, "off", "tdd", "--harness", "pi")

	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDisabled("tdd", "pi") {
		t.Error("state: tdd/pi not disabled")
	}
	if st.IsDisabled("tdd", "opencode") {
		t.Error("state: tdd/opencode disabled, want untouched")
	}
	if body := readFile(t, p.OpenCodeConfig()); body != "" {
		t.Errorf("opencode config written without a target: %q", body)
	}
	if body := readFile(t, p.PiSettings()); !strings.Contains(body, "-skills/tdd/SKILL.md") {
		t.Errorf("pi settings:\n%s", body)
	}
	if !strings.Contains(out, `sync: pi: disabled "tdd" (was on)`) {
		t.Errorf("output missing the pi change:\n%s", out)
	}
	if strings.Contains(out, "opencode: disabled") {
		t.Errorf("output reports untargeted harnesses:\n%s", out)
	}
}

func TestOnRemovesStateAndStripsFleetMarkers(t *testing.T) {
	p := toggleHome(t)
	runToggle(t, p, "off", "tdd")

	out, _ := runToggle(t, p, "on", "tdd")

	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"opencode", "pi", "codex", "claude"} {
		if st.IsDisabled("tdd", h) {
			t.Errorf("state: tdd/%s still disabled", h)
		}
	}

	for _, path := range []string{p.OpenCodeConfig(), p.PiSettings(), p.CodexConfig(), p.ClaudeSettings()} {
		body := readFile(t, path)
		if strings.Contains(body, "tdd") {
			t.Errorf("%s still mentions tdd after enabling:\n%s", filepath.Base(path), body)
		}
	}
	if !strings.Contains(out, `sync: pi: enabled "tdd" (was off)`) {
		t.Errorf("output missing the enable change:\n%s", out)
	}
}

func TestOffRejectsUnknownSkillsAndHarnesses(t *testing.T) {
	p := toggleHome(t)

	if err := runToggleErr(t, p, "off", "no-such-skill"); err == nil {
		t.Error("off with an unknown skill should fail")
	}
	if err := runToggleErr(t, p, "off", "tdd", "--harness", "bogus"); err == nil {
		t.Error("off with an unknown harness should fail")
	}
	if _, err := os.Stat(p.FleetStateFile()); !os.IsNotExist(err) {
		t.Error("a rejected toggle must not create the state file")
	}
}

func TestOffWithUninstalledHarnessFails(t *testing.T) {
	p := paths.New(filepath.Join(t.TempDir(), "home"))
	writeSkillDir(t, p.SkillsStore(), "tdd", "desc")
	if err := os.MkdirAll(p.OpenCodeDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := runToggleErr(t, p, "off", "tdd", "--harness", "pi"); err == nil {
		t.Error("off --harness pi without pi installed should fail")
	}
}

func TestOffOnUninstalledHarnessByNameListsIt(t *testing.T) {
	p := toggleHome(t)
	if err := runToggleErr(t, p, "off", "tdd", "--harness", "cursor", "--harness", "bogus"); err == nil {
		t.Error("want error for unknown harness")
	}
}

func TestOnForSkillNeverToggledIsQuietlyFine(t *testing.T) {
	p := toggleHome(t)
	out, _ := runToggle(t, p, "on", "tdd")

	// No state entry existed, nothing to strip: the command succeeds with
	// nothing to report beyond the no-op messages.
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"opencode", "pi", "codex", "claude"} {
		if st.IsDisabled("tdd", h) {
			t.Errorf("state: tdd/%s disabled after on", h)
		}
	}
	if strings.Contains(out, `enabled "tdd"`) {
		t.Errorf("output claims a change that did not happen:\n%s", out)
	}
}

func TestDisablingNeverTouchesTheCanonicalStore(t *testing.T) {
	// The skill's files must survive off/on byte for byte: the skills CLI
	// keeps updating disabled skills, so the store stays pristine.
	p := toggleHome(t)
	hash := func() string {
		body, err := os.ReadFile(filepath.Join(p.SkillsStore(), "tdd", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	before := hash()

	runToggle(t, p, "off", "tdd")
	runToggle(t, p, "on", "tdd")

	if after := hash(); after != before {
		t.Errorf("canonical store changed:\n%q\nwas\n%q", after, before)
	}
	entries, err := os.ReadDir(p.SkillsStore())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "tdd" {
		t.Errorf("store contents = %v, want just tdd", entries)
	}
}

func TestSyncRunsOnLsAndReportsToStderr(t *testing.T) {
	p := fakeHome(t) // has manual disables: opencode git-*, pi tdd, codex deploy-vercel

	// The state file disables tdd for opencode; sync must add the V2 deny
	// before the table is built.
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	st.SetDisabled("tdd", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	out, errOut := runLsFull(t, p)

	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, `"resource": "tdd"`) {
		t.Errorf("sync did not project the opencode disable:\n%s", body)
	}
	// Machine-readable stdout stays clean; findings go to stderr. The pi
	// exclusion and codex block are manual edits fleet doesn't track.
	if strings.Contains(out, "sync:") {
		t.Errorf("sync findings leaked to stdout:\n%s", out)
	}
	if !strings.Contains(errOut, "sync: pi/tdd: disabled in config but not tracked by fleet's state") {
		t.Errorf("stderr missing the pi manual-edit flag:\n%s", errOut)
	}
	if !strings.Contains(errOut, "sync: codex/deploy-vercel: disabled in config but not tracked by fleet's state") {
		t.Errorf("stderr missing the codex manual-edit flag:\n%s", errOut)
	}
}

func runLsFull(t *testing.T, p *paths.Paths) (string, string) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"skill", "ls"})
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	if err := root.Execute(); err != nil {
		t.Fatalf("fleet skill ls: error = %v", err)
	}
	return out.String(), errOut.String()
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(body)
}
