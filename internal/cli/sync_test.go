// The sync verb: the ambient sync every fleet command runs, on demand.
// These tests drive the CLI face only — the machinery's own behavior is
// covered in internal/sync — asserting the external contract: the report
// the other verbs print, the error (non-zero exit) on failure, drift
// actually repaired, and unknown entries reported without being touched.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/state"
)

// runSync runs `fleet skill sync` (or the given args), returning stdout,
// stderr, and the command error so tests can assert the exit path.
func runSync(t *testing.T, p *paths.Paths, args ...string) (string, string, error) {
	t.Helper()
	if len(args) == 0 {
		args = []string{"sync"}
	}
	root := NewRoot(p)
	out, errOut := &strings.Builder{}, &strings.Builder{}
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"skill"}, args...))
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	err := root.Execute()
	return out.String(), errOut.String(), err
}

// recordDisables writes a state file disabling name for every harness with
// a write side, without touching any config: maximal drift by construction.
func recordDisables(t *testing.T, p *paths.Paths, name string) {
	t.Helper()
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"opencode", "pi", "codex", "claude"} {
		st.SetDisabled(name, h)
	}
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}
}

func TestSyncRepairsRecordedDisablesAndReportsThem(t *testing.T) {
	p := toggleHome(t)
	recordDisables(t, p, "tdd")

	out, errOut, err := runSync(t, p)
	if err != nil {
		t.Fatalf("fleet skill sync: %v (stderr: %s)", err, errOut)
	}

	// The same report the other verbs print for their ambient sync.
	for _, want := range []string{
		`sync: opencode: disabled "tdd" (was on)`,
		`sync: pi: disabled "tdd" (was on)`,
		`sync: codex: disabled "tdd" (was on)`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Claude can only disable a skill it can discover, and toggleHome
	// gives it no link: nothing to do, nothing reported.
	if strings.Contains(out, "claude") {
		t.Errorf("output reports work on claude that cannot happen:\n%s", out)
	}
	// Cursor and Bob have no write side; unlike on/off, sync has no
	// per-skill toggle to call a no-op, so it says nothing about them.
	if strings.Contains(out, "cursor") || strings.Contains(out, "bob") {
		t.Errorf("output mentions harnesses sync never writes:\n%s", out)
	}

	// The drift is actually repaired: each config carries its own native
	// off marker.
	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, `"tdd": "deny"`) {
		t.Errorf("opencode config not repaired:\n%s", body)
	}
	if body := readFile(t, p.PiSettings()); !strings.Contains(body, "-skills/tdd/SKILL.md") {
		t.Errorf("pi settings not repaired:\n%s", body)
	}
	if body := readFile(t, p.CodexConfig()); !strings.Contains(body, `name = "tdd"`) || !strings.Contains(body, "enabled = false") {
		t.Errorf("codex config not repaired:\n%s", body)
	}

	// Sync never edits the state file.
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"opencode", "pi", "codex", "claude"} {
		if !st.IsDisabled("tdd", h) {
			t.Errorf("state: tdd/%s no longer disabled after sync", h)
		}
	}
}

func TestSyncRepairsWhatAHandRunSkillsCLIDisturbs(t *testing.T) {
	// The undo.md scenario: a hand-run `skills update` resets the config
	// and re-creates its link spam; the sync verb converges the home again.
	p := toggleHome(t)
	if _, _, err := runSync(t, p, "off", "tdd"); err != nil {
		t.Fatal(err)
	}
	// The hand run: enablement resurrected (config gone) and the
	// per-agent link re-created.
	if err := os.Remove(p.OpenCodeConfig()); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p.OpenCodeSkills(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(p.SkillsStore(), "tdd"), filepath.Join(p.OpenCodeSkills(), "tdd")); err != nil {
		t.Fatal(err)
	}

	out, _, err := runSync(t, p)
	if err != nil {
		t.Fatalf("fleet skill sync: %v", err)
	}

	if !strings.Contains(out, `sync: opencode: removed redundant link "tdd" — opencode scans the canonical store natively`) {
		t.Errorf("output missing the link removal:\n%s", out)
	}
	if !strings.Contains(out, `sync: opencode: disabled "tdd" (was on)`) {
		t.Errorf("output missing the re-disable:\n%s", out)
	}
	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, `"tdd": "deny"`) {
		t.Errorf("opencode config not re-projected:\n%s", body)
	}
	if _, err := os.Lstat(filepath.Join(p.OpenCodeSkills(), "tdd")); !os.IsNotExist(err) {
		t.Error("the redundant link survived sync")
	}

	// Idempotent: a converged home syncs to silence.
	out, _, err = runSync(t, p)
	if err != nil {
		t.Fatalf("second fleet skill sync: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("second run reported work it did not do:\n%s", out)
	}
}

func TestSyncFlagsUnknownEntriesWithoutTouchingThem(t *testing.T) {
	// A manual edit fleet doesn't track: reported, never touched, and no
	// state file invented to adopt it.
	p := toggleHome(t)
	if err := os.MkdirAll(filepath.Dir(p.PiSettings()), 0o755); err != nil {
		t.Fatal(err)
	}
	edit := `{"skills": ["-skills/tdd/SKILL.md"]}`
	if err := os.WriteFile(p.PiSettings(), []byte(edit), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, err := runSync(t, p)
	if err != nil {
		t.Fatalf("fleet skill sync: %v", err)
	}

	if !strings.Contains(out, "sync: pi/tdd: disabled in config but not tracked by fleet's state") {
		t.Errorf("output missing the flag:\n%s", out)
	}
	if body := readFile(t, p.PiSettings()); body != edit {
		t.Errorf("the manual edit was touched:\n%s\nwas\n%s", body, edit)
	}
	if _, err := os.Stat(p.FleetStateFile()); !os.IsNotExist(err) {
		t.Errorf("sync wrote a state file: %v", err)
	}
}

func TestSyncGroupsRepeatedFlagsIntoOneLine(t *testing.T) {
	// Several hand-edits with the same shape: one line with the message
	// and the skill names — not one near-identical line per skill.
	p := toggleHome(t)
	if err := os.MkdirAll(filepath.Dir(p.PiSettings()), 0o755); err != nil {
		t.Fatal(err)
	}
	edit := `{"skills": ["-skills/tdd/SKILL.md", "-skills/git-helper/SKILL.md", "-skills/deploy-vercel/SKILL.md"]}`
	if err := os.WriteFile(p.PiSettings(), []byte(edit), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, err := runSync(t, p)
	if err != nil {
		t.Fatalf("fleet skill sync: %v", err)
	}

	if !strings.Contains(out, "sync: pi: disabled in config but not tracked by fleet's state — left alone (3 skills): tdd, git-helper, deploy-vercel") {
		t.Errorf("output missing the grouped flag:\n%s", out)
	}
	if strings.Count(out, "disabled in config but not tracked") != 1 {
		t.Errorf("the repeated message should appear once, not per skill:\n%s", out)
	}
}

func TestSyncChangeLinesCarryTheOutcomeWeight(t *testing.T) {
	// The flip is the report's headline, so it gets the treatment the
	// on/off outcome line gives its verb: green verb, dim "sync:" prefix,
	// cyan harness, and the "(was on)" provenance as a faint annotation.
	// Unit-level with a fake palette: the composition is the contract,
	// not the escape codes.
	pal := palette{
		dim:  func(s string) string { return "{" + s + "}" },
		info: func(s string) string { return "[" + s + "]" },
		good: func(s string) string { return "<" + s + ">" },
	}
	got := styleChange(`sync: pi: disabled "deploy-to-vercel" (was on)`, pal)
	want := `{sync: }[pi]: <disabled> "deploy-to-vercel" {(was on)}`
	if got != want {
		t.Errorf("styled change line:\n%s\nwant:\n%s", got, want)
	}

	// Flags share the "disabled…" wording but are findings, not flips:
	// they get the dim/cyan scoping, never the verb's weight.
	flag := `sync: pi: disabled in config but not tracked by fleet's state — left alone`
	wantFlag := `{sync: }[pi]: disabled in config but not tracked by fleet's state — left alone`
	if got := styleSyncLine(flag, pal); got != wantFlag {
		t.Errorf("styled flag line:\n%s\nwant:\n%s", got, wantFlag)
	}
}

func TestSyncOnAConvergedHomePrintsNothing(t *testing.T) {
	p := toggleHome(t)

	out, _, err := runSync(t, p)
	if err != nil {
		t.Fatalf("fleet skill sync: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("a converged home should sync to silence:\n%s", out)
	}
}

func TestSyncFailsOnABrokenHarnessConfig(t *testing.T) {
	// Exit path: a config sync cannot read fails the command, like it
	// fails every command whose ambient sync hits it.
	p := toggleHome(t)
	recordDisables(t, p, "tdd")
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(`{"permission": {`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runSync(t, p)
	if err == nil {
		t.Fatal("sync on an unreadable config should fail")
	}
}

func TestSyncRejectsArguments(t *testing.T) {
	p := toggleHome(t)

	if _, _, err := runSync(t, p, "sync", "tdd"); err == nil {
		t.Error("sync takes no arguments")
	}
}
