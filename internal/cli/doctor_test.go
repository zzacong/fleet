package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/state"
)

// doctorHome builds a home with every harness installed, one stored skill
// ("tdd"), and no state file: clean by construction.
func doctorHome(t *testing.T) *paths.Paths {
	t.Helper()
	p := toggleHome(t)
	// claude's link dir exists and is empty, so the missing-dir finding
	// stays out of the clean case.
	if err := os.MkdirAll(p.ClaudeSkills(), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// makeLink creates a symlink, creating the link's parent dir.
func makeLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// runDoctor runs `fleet skill doctor` with stdin set to input, returning
// stdout. Extra args (e.g. "--interactive") are appended.
func runDoctor(t *testing.T, p *paths.Paths, input string, args ...string) string {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"skill", "doctor"}, args...))
	if input != "" {
		root.SetIn(strings.NewReader(input))
	}
	// Plain output deterministically, whatever the test runner's stdout.
	stdoutTTY = func() bool { return false }
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	if err := root.Execute(); err != nil {
		t.Fatalf("fleet skill doctor: error = %v (stderr: %s)", err, errOut.String())
	}
	return out.String()
}

func TestDoctorCleanHomeReportsNothing(t *testing.T) {
	p := doctorHome(t)

	out := runDoctor(t, p, "")
	if !strings.Contains(out, "no problems found") {
		t.Errorf("output =\n%s\nwant a clean bill of health", out)
	}
}

func TestDoctorFlagsHandEditedConfig(t *testing.T) {
	// The ticket's integration path: hand-edit a config → doctor flags it,
	// and with no terminal input the edit is left exactly as it was.
	p := doctorHome(t)
	if err := os.MkdirAll(filepath.Dir(p.OpenCodeConfig()), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := `{"permission": {"skill": {"tdd": "deny"}}}`
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "")

	if !strings.Contains(out, "manual edit conflicts (1) · left as is") {
		t.Errorf("output missing the conflict report section:\n%s", out)
	}
	if !strings.Contains(out, "opencode  tdd    config off · state on") {
		t.Errorf("output missing the conflict row:\n%s", out)
	}
	if !strings.Contains(out, "k keep my change · r restore — run `fleet skill doctor -i` to pick per skill") {
		t.Errorf("output missing the options legend:\n%s", out)
	}
	if !strings.Contains(out, "1 manual edit to resolve, run `fleet skill doctor -i` to resolve") {
		t.Errorf("output missing the count summary:\n%s", out)
	}
	// Default: no prompt is asked, the edit is reported and left as is.
	if body := readFile(t, p.OpenCodeConfig()); body != fixture {
		t.Errorf("config was changed without consent:\n%s", body)
	}
	if _, err := os.Stat(p.FleetStateFile()); !os.IsNotExist(err) {
		t.Errorf("state file created without consent: %v", err)
	}
}

func TestDoctorDefaultIgnoresStdin(t *testing.T) {
	// Without --interactive doctor never reads stdin: input that would
	// answer a prompt changes nothing.
	p := doctorHome(t)
	if err := os.MkdirAll(filepath.Dir(p.OpenCodeConfig()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(`{"permission": {"skill": {"tdd": "deny"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "k\n")

	if strings.Contains(out, "kept:") || strings.Contains(out, "restored:") {
		t.Errorf("doctor resolved a conflict without --interactive:\n%s", out)
	}
	if _, err := os.Stat(p.FleetStateFile()); !os.IsNotExist(err) {
		t.Errorf("state file written without --interactive: %v", err)
	}
	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, `"tdd": "deny"`) {
		t.Error("config was changed without --interactive")
	}
}

func TestDoctorKeepAdoptsTheManualEdit(t *testing.T) {
	p := doctorHome(t)
	if err := os.MkdirAll(filepath.Dir(p.OpenCodeConfig()), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := `{"permission": {"skill": {"tdd": "deny"}}}`
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "k\n", "--interactive")

	if !strings.Contains(out, `kept: "tdd" recorded as disabled for opencode in the state file`) {
		t.Errorf("output missing the keep receipt:\n%s", out)
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDisabled("tdd", "opencode") {
		t.Error("state does not record the kept disable")
	}
	if body := readFile(t, p.OpenCodeConfig()); body != fixture {
		t.Errorf("keep must not touch the config:\n%s", body)
	}

	// The adoption converges: a second doctor run is clean.
	if out := runDoctor(t, p, ""); !strings.Contains(out, "no problems found") {
		t.Errorf("second run =\n%s\nwant clean", out)
	}
}

func TestDoctorRestoreSyncsTheConfigBack(t *testing.T) {
	p := doctorHome(t)
	if err := os.MkdirAll(filepath.Dir(p.OpenCodeConfig()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(`{"permission": {"skill": {"tdd": "deny"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "r\n", "--interactive")

	if !strings.Contains(out, `restored: opencode: enabled "tdd" (was off)`) {
		t.Errorf("output missing the restore receipt:\n%s", out)
	}
	if body := readFile(t, p.OpenCodeConfig()); strings.Contains(body, "tdd") {
		t.Errorf("config still holds the manual deny:\n%s", body)
	}
	// Restore never writes state: the config simply matches again.
	if _, err := os.Stat(p.FleetStateFile()); !os.IsNotExist(err) {
		t.Errorf("state file written by a restore: %v", err)
	}
}

func TestDoctorSkipLeavesEverythingAsIs(t *testing.T) {
	p := doctorHome(t)
	if err := os.MkdirAll(filepath.Dir(p.OpenCodeConfig()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(`{"permission": {"skill": {"tdd": "deny"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "s\n", "--interactive")

	if !strings.Contains(out, "skipped") {
		t.Errorf("output missing the skip receipt:\n%s", out)
	}
	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, "tdd") {
		t.Error("skip changed the config")
	}
	if _, err := os.Stat(p.FleetStateFile()); !os.IsNotExist(err) {
		t.Error("skip changed the state file")
	}
}

func TestDoctorReportsRedundantLinks(t *testing.T) {
	// The other integration path: recreate a redundant link by hand, then
	// let sync (run inside on/off/ls) remove it. Doctor reports it first,
	// and never removes anything itself.
	p := doctorHome(t)
	makeLink(t, filepath.Join(p.SkillsStore(), "tdd"), filepath.Join(p.OpenCodeSkills(), "tdd"))

	out := runDoctor(t, p, "")
	if !strings.Contains(out, "redundant links") || !strings.Contains(out, `"tdd"`) {
		t.Errorf("output missing the redundant link:\n%s", out)
	}
	// Read-only: the link is still there for sync to remove.
	if _, err := os.Lstat(filepath.Join(p.OpenCodeSkills(), "tdd")); err != nil {
		t.Errorf("doctor removed the link itself: %v", err)
	}
}

func TestDoctorFlagsAdoptionFollowups(t *testing.T) {
	// Ticket 03's doctor follow-ups: the store copy came back while the
	// adopted repo copy stayed, and the lockfile still carries the
	// pre-adoption install entry.
	p := doctorHome(t)
	p.Repo = filepath.Join(t.TempDir(), "repo")
	writeSkillDir(t, p.RepoSkills(), "tdd", "Red-green-refactor workflow.")
	lock := `{"skills": {"tdd": {"source": "mattpocock/skills", "sourceType": "github", "skillFolderHash": "abc123"}}}`
	if err := os.WriteFile(p.SkillLock(), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "")

	if !strings.Contains(out, "double presence (store and repo) (1)") || !strings.Contains(out, `"tdd" exists in both`) {
		t.Errorf("output missing the double-presence finding:\n%s", out)
	}
	if !strings.Contains(out, "stale lockfile entries (1) · fleet never writes the lockfile") {
		t.Errorf("output missing the stale-lock section:\n%s", out)
	}
	if !strings.Contains(out, "1 double-presence finding, 1 stale lockfile entry") {
		t.Errorf("output missing the count summary:\n%s", out)
	}
	// Read-only: the lockfile is untouched and both copies stay put.
	if body := readFile(t, p.SkillLock()); body != lock {
		t.Errorf("doctor modified the lockfile:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(p.SkillsStore(), "tdd")); err != nil {
		t.Errorf("store copy disturbed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.RepoSkills(), "tdd")); err != nil {
		t.Errorf("repo copy disturbed: %v", err)
	}
}

func TestDoctorSyncAfterReportLeavesHomeClean(t *testing.T) {
	// Doctor says what it would change; sync does it. Both paths green in
	// a fake home: report → resolve → sync (on the next command) → clean.
	p := doctorHome(t)
	makeLink(t, filepath.Join(p.SkillsStore(), "tdd"), filepath.Join(p.OpenCodeSkills(), "tdd"))

	if err := os.MkdirAll(filepath.Dir(p.OpenCodeConfig()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(`{"permission": {"skill": {"tdd": "deny"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if out := runDoctor(t, p, ""); !strings.Contains(out, "redundant links") || !strings.Contains(out, "manual edit") {
		t.Errorf("first doctor run missing findings:\n%s", out)
	}

	// The user adopts the manual edit; the redundant link has no prompt —
	// sync owns it.
	if out := runDoctor(t, p, "k\n", "--interactive"); !strings.Contains(out, "kept:") {
		t.Errorf("output missing the keep receipt:\n%s", out)
	}

	// Any other command runs sync, which removes the redundant link.
	if _, errOut := runToggle(t, p, "ls", "--json"); strings.Contains(errOut, "error") {
		t.Fatalf("ls failed: %s", errOut)
	}
	if _, err := os.Lstat(filepath.Join(p.OpenCodeSkills(), "tdd")); !os.IsNotExist(err) {
		t.Error("sync did not remove the redundant link")
	}

	if out := runDoctor(t, p, ""); !strings.Contains(out, "no problems found") {
		t.Errorf("final doctor run =\n%s\nwant clean", out)
	}
}

func TestDoctorDriftConflictRestoreRediscables(t *testing.T) {
	p := doctorHome(t)
	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "r\n", "--interactive")

	if !strings.Contains(out, `restored: opencode: disabled "tdd" (was on)`) {
		t.Errorf("output missing the re-disable receipt:\n%s", out)
	}
	if body := readFile(t, p.OpenCodeConfig()); !strings.Contains(body, `"tdd": "deny"`) {
		t.Errorf("config missing the restored deny:\n%s", body)
	}
}

func TestDoctorDriftConflictKeepAdoptsTheDeletion(t *testing.T) {
	p := doctorHome(t)
	st, _ := state.Load(p.FleetStateFile())
	st.SetDisabled("tdd", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "k\n", "--interactive")

	if !strings.Contains(out, `kept: "tdd" recorded as enabled for opencode in the state file`) {
		t.Errorf("output missing the adoption receipt:\n%s", out)
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if st.IsDisabled("tdd", "opencode") {
		t.Error("state still disables a skill the user enabled")
	}
}
