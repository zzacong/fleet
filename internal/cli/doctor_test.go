package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	if !strings.Contains(out, "redundant links") || !strings.Contains(out, "opencode  scans the canonical store natively — this link double-covers the skill\n") {
		t.Errorf("output missing the grouped redundant link:\n%s", out)
	}
	if !strings.Contains(out, "tdd\n") {
		t.Errorf("output missing the skill name:\n%s", out)
	}
	if strings.Contains(out, `link "tdd"`) {
		t.Errorf("per-finding redundant prose still present:\n%s", out)
	}
	// Read-only: the link is still there for sync to remove.
	if _, err := os.Lstat(filepath.Join(p.OpenCodeSkills(), "tdd")); err != nil {
		t.Errorf("doctor removed the link itself: %v", err)
	}
}

func TestDoctorFlagsAdoptionFollowups(t *testing.T) {
	// Ticket 03's doctor follow-ups: the store copy came back while the
	// adopted tracked copy stayed, and the lockfile still carries the
	// pre-adoption install entry.
	p := doctorHome(t)
	tracked := filepath.Join(t.TempDir(), "tracked")
	trackedCollection := filepath.Join(tracked, "skills")
	writeAdoptConfig(t, p, `{"skillsDirs": ["`+trackedCollection+`"]}`)
	writeSkillDir(t, trackedCollection, "tdd", "Red-green-refactor workflow.")
	lock := `{"skills": {"tdd": {"source": "mattpocock/skills", "sourceType": "github", "skillFolderHash": "abc123"}}}`
	if err := os.WriteFile(p.SkillLock(), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "")

	if !strings.Contains(out, "double presence (1)") {
		t.Errorf("output missing the double-presence section:\n%s", out)
	}
	if !strings.Contains(out, "duplicated in ~/.agents/skills and "+trackedCollection+" — remove one copy by hand") {
		t.Errorf("output missing the grouped double-presence label:\n%s", out)
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
	if _, err := os.Stat(filepath.Join(trackedCollection, "tdd")); err != nil {
		t.Errorf("tracked copy disturbed: %v", err)
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

// piConflictsHome builds a home whose pi config hand-disables two skills
// ("tdd" and "deploy-to-vercel", both installed in the store but untracked
// by the state), so doctor has a two-conflict pi batch to walk.
func piConflictsHome(t *testing.T) *paths.Paths {
	t.Helper()
	p := doctorHome(t)
	writeSkillDir(t, p.SkillsStore(), "deploy-to-vercel", "Deploy to Vercel.")
	if err := os.MkdirAll(filepath.Dir(p.PiSettings()), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := `{"skills": ["-skills/tdd/SKILL.md", "-skills/deploy-to-vercel/SKILL.md"]}`
	if err := os.WriteFile(p.PiSettings(), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDoctorKeepAllAdoptsTheHarnessBatch(t *testing.T) {
	p := piConflictsHome(t)

	// First prompt answers keep for one conflict, second takes the batch
	// option for what remains of pi.
	out := runDoctor(t, p, "k\na\n", "--interactive")

	if !strings.Contains(out, "[a] keep all 2 pi conflicts") || !strings.Contains(out, "[x] skip all 2 pi conflicts") {
		t.Errorf("output missing the pi batch options:\n%s", out)
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDisabled("tdd", "pi") || !st.IsDisabled("deploy-to-vercel", "pi") {
		t.Error("keep all did not record both pi disables")
	}
	if st.IsDisabled("tdd", "opencode") {
		t.Error("keep all reached beyond the pi batch")
	}
	if strings.Count(out, "kept:") != 2 {
		t.Errorf("want two keep receipts:\n%s", out)
	}

	// The adoption converges: a second doctor run is clean.
	if out := runDoctor(t, p, ""); !strings.Contains(out, "no problems found") {
		t.Errorf("second run =\n%s\nwant clean", out)
	}
}

func TestDoctorSkipAllLeavesTheHarnessBatchAsIs(t *testing.T) {
	p := piConflictsHome(t)

	out := runDoctor(t, p, "x\n", "--interactive")

	if !strings.Contains(out, "skipped 2 pi conflicts") {
		t.Errorf("output missing the batch skip receipt:\n%s", out)
	}
	if _, err := os.Stat(p.FleetStateFile()); !os.IsNotExist(err) {
		t.Error("skip all changed the state file")
	}
	if body := readFile(t, p.PiSettings()); !strings.Contains(body, `-skills/tdd/SKILL.md`) {
		t.Error("skip all changed the pi config")
	}
}

func TestDoctorBatchOptionsNeverCrossHarnesses(t *testing.T) {
	// opencode comes before pi: keep-all at opencode's lone conflict must
	// stop there, and pi's batch is offered and answered separately.
	p := piConflictsHome(t)
	if err := os.MkdirAll(filepath.Dir(p.OpenCodeConfig()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(`{"permission": {"skill": {"tdd": "deny"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "k\na\n", "--interactive")

	if strings.Contains(out, "[a] keep all 1 opencode conflict") {
		t.Errorf("batch options offered for a single conflict:\n%s", out)
	}
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsDisabled("tdd", "opencode") {
		t.Error("the opencode keep was not applied")
	}
	if !st.IsDisabled("tdd", "pi") || !st.IsDisabled("deploy-to-vercel", "pi") {
		t.Error("the pi batch keep was not applied")
	}
	if strings.Count(out, "kept:") != 3 {
		t.Errorf("want three keep receipts:\n%s", out)
	}
}

func TestDoctorReportsUnscannedAdoptTarget(t *testing.T) {
	p := doctorHome(t)
	target := filepath.Join(t.TempDir(), "elsewhere", "skills")
	cfg, err := json.Marshal(map[string]any{
		"adoptTarget": target,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p.FleetConfigFile()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.FleetConfigFile(), cfg, 0o644); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "")

	if !strings.Contains(out, "unscanned adopt target (1)") || !strings.Contains(out, target) {
		t.Errorf("output missing the unscanned adopt target section:\n%s", out)
	}
	if !strings.Contains(out, "1 unscanned adopt target") {
		t.Errorf("output missing the count summary:\n%s", out)
	}
}

func TestDoctorReportsStaleDisables(t *testing.T) {
	// A state entry and a config rule for a skill installed nowhere: both
	// leftovers show up and point at prune.
	p := doctorHome(t)
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	st.SetDisabled("ghost", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p.OpenCodeConfig()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(`{"permission": {"skill": {"ghost": "deny"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "")

	if !strings.Contains(out, "stale config rules (1)") || !strings.Contains(out, "stale state entries (1)") {
		t.Errorf("output missing the stale sections:\n%s", out)
	}
	if !strings.Contains(out, "`fleet skill prune`") {
		t.Errorf("output must point at prune:\n%s", out)
	}
	if !strings.Contains(out, "1 stale config rule, 1 stale state entry") {
		t.Errorf("output missing the count summary:\n%s", out)
	}
}

func TestDoctorStaleSectionsGroupByHarnessAndCap(t *testing.T) {
	// Many stale skills on pi and a couple on opencode: each section is one
	// line per harness with the skills comma-joined, the remediation lives
	// once in the header, and a long list is capped.
	p := doctorHome(t)
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	piSkills := []string{"ghost-a", "ghost-b", "ghost-c", "ghost-d", "ghost-e", "ghost-f", "ghost-g"}
	for _, name := range piSkills {
		st.SetDisabled(name, "pi")
	}
	for _, name := range []string{"ghost-a", "ghost-b"} {
		st.SetDisabled(name, "opencode")
	}
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p.OpenCodeConfig()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(`{"permission": {"skill": {"ghost-a": "deny", "ghost-b": "deny"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := make([]string, len(piSkills))
	for i, name := range piSkills {
		entries[i] = fmt.Sprintf("%q", "-skills/"+name+"/SKILL.md")
	}
	if err := os.MkdirAll(filepath.Dir(p.PiSettings()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.PiSettings(), []byte(`{"skills": [`+strings.Join(entries, ", ")+`]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "")

	// The boilerplate is hoisted: the reinstall warning appears once in the
	// state header, and no per-finding prose remains.
	if got := strings.Count(out, "pruning loses the disable-on-reinstall behavior"); got != 1 {
		t.Errorf("reinstall warning appears %d times, want 1 (hoisted):\n%s", got, out)
	}
	if strings.Contains(out, "but it is installed nowhere") {
		t.Errorf("per-finding boilerplate still present:\n%s", out)
	}
	// One line per harness, skills comma-joined.
	if !strings.Contains(out, "opencode  ghost-a, ghost-b\n") {
		t.Errorf("opencode line missing:\n%s", out)
	}
	// pi has seven: five shown, two summarized.
	if !strings.Contains(out, "ghost-a, ghost-b, ghost-c, ghost-d, ghost-e, … (+2 more)\n") {
		t.Errorf("pi capped line missing:\n%s", out)
	}
	if !strings.Contains(out, "stale config rules (9)") || !strings.Contains(out, "stale state entries (9)") {
		t.Errorf("section counts wrong:\n%s", out)
	}
}

func TestDoctorDriftGroupsByHarnessAndDirection(t *testing.T) {
	// Custom skills in a tracked repo, unlinked on every native scanner.
	// opencode also carries a disabled custom whose managed link survived,
	// so it holds two directions and must print two groups. The shared
	// cause and fix print once per group, not once per finding.
	p := doctorHome(t)
	repo := filepath.Join(t.TempDir(), "customs")
	collection := filepath.Join(repo, "skills")
	writeAdoptConfig(t, p, `{"skillsDirs": ["`+collection+`"]}`)
	writeSkillDir(t, collection, "my-notes", "Notes.")
	writeSkillDir(t, collection, "my-docs", "Docs.")
	// my-docs is disabled on opencode, but its managed link is still there.
	makeLink(t, filepath.Join(collection, "my-docs"), filepath.Join(p.OpenCodeSkills(), "my-docs"))
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	st.SetDisabled("my-docs", "opencode")
	if err := state.Save(p.FleetStateFile(), st); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "")

	// opencode: enabled-unlinked (my-notes) plus disabled-linked (my-docs).
	// pi, codex, cursor, bob: enabled-unlinked (my-notes, my-docs).
	if !strings.Contains(out, "state drift (10)") {
		t.Errorf("want a drift count of 10:\n%s", out)
	}
	if got := strings.Count(out, "enabled but not linked"); got != 5 {
		t.Errorf("enabled-unlinked groups = %d, want 5 (one per native scanner):\n%s", got, out)
	}
	if got := strings.Count(out, "disabled but still linked"); got != 1 {
		t.Errorf("disabled-linked groups = %d, want 1:\n%s", got, out)
	}
	// The full sentence is replaced by the group label.
	if strings.Contains(out, "is enabled in fleet's state") || strings.Contains(out, "is disabled in fleet's state") {
		t.Errorf("per-finding drift prose still present:\n%s", out)
	}
	// Names hang under the harness column: 2 + width(8) + 2 spaces.
	if !strings.Contains(out, "            my-docs, my-notes\n") {
		t.Errorf("skills not listed under their group:\n%s", out)
	}
	// The injected home shortens to ~; the absolute prefix stays out.
	if !strings.Contains(out, "in ~/.config/opencode/skills") {
		t.Errorf("home directory not shortened to ~:\n%s", out)
	}
	if strings.Contains(out, p.OpenCodeSkills()) {
		t.Errorf("absolute home path still shown:\n%s", out)
	}
}

// TestDoctorDoublePresenceGroupsByHarnessAndHomes checks that two tracked
// repos sharing two names collapse into one group per native-scanning
// harness: the shared homes and the manual resolution print once, then both
// names, not one paragraph each.
func TestDoctorDoublePresenceGroupsByHarnessAndHomes(t *testing.T) {
	p := doctorHome(t)
	first := filepath.Join(p.Home, "repos-a")
	second := filepath.Join(p.Home, "repos-b")
	firstCollection := filepath.Join(first, "skills")
	secondCollection := filepath.Join(second, "skills")
	writeAdoptConfig(t, p, `{"skillsDirs": ["`+firstCollection+`", "`+secondCollection+`"]}`)
	writeSkillDir(t, firstCollection, "babysit-pr", "Reviews PRs.")
	writeSkillDir(t, secondCollection, "babysit-pr", "Reviews PRs.")
	writeSkillDir(t, firstCollection, "choose-flow", "Picks a flow.")
	writeSkillDir(t, secondCollection, "choose-flow", "Picks a flow.")

	out := runDoctor(t, p, "")

	if !strings.Contains(out, "double presence (2)") {
		t.Errorf("want a double-presence count of 2:\n%s", out)
	}
	// One group per native scanner (opencode, pi, codex, cursor, bob);
	// claude is link-only and never sees a name twice.
	label := "duplicated in " + shortenHome(p.Home, filepath.Join(first, "skills")) +
		" and " + shortenHome(p.Home, filepath.Join(second, "skills")) + " — remove one copy by hand"
	if got := strings.Count(out, label); got != 5 {
		t.Errorf("grouped label appears %d times, want 5 (one per native scanner):\n%s", got, out)
	}
	// Each native scanner is the group key; claude is link-only and absent.
	for _, h := range []string{"opencode", "pi", "codex", "cursor", "bob"} {
		if !strings.Contains(out, padRight(h, 8)+"  duplicated in ") {
			t.Errorf("harness %s missing from a double-presence group:\n%s", h, out)
		}
	}
	if strings.Contains(out, padRight("claude", 8)+"  duplicated in ") {
		t.Errorf("claude must not appear: it does not scan the canonical store:\n%s", out)
	}
	if !strings.Contains(out, "babysit-pr, choose-flow\n") {
		t.Errorf("names not listed under their group:\n%s", out)
	}
	if strings.Contains(out, "exists in both") || strings.Contains(out, "would see it twice") {
		t.Errorf("per-finding double-presence prose still present:\n%s", out)
	}
	if strings.Contains(out, p.Home) {
		t.Errorf("absolute home path still shown:\n%s", out)
	}
}

// TestDoctorGroupsLinkAndManualSections checks that the sections which
// otherwise print a full sentence per finding collapse to one line per
// harness and cause, with the skill names under it: redundant links, broken
// symlinks, unknown entries, and manual edits.
func TestDoctorGroupsLinkAndManualSections(t *testing.T) {
	p := doctorHome(t)
	for _, name := range []string{"alpha", "bravo", "trace"} {
		writeSkillDir(t, p.SkillsStore(), name, "Does "+name+".")
	}
	// Redundant links into the store on two native scanners.
	for _, dir := range []string{p.OpenCodeSkills(), p.PiSkills()} {
		for _, name := range []string{"alpha", "bravo"} {
			makeLink(t, filepath.Join(p.SkillsStore(), name), filepath.Join(dir, name))
		}
	}
	// Two broken symlinks on opencode: one group, two names.
	for _, name := range []string{"gone-a", "gone-b"} {
		makeLink(t, filepath.Join(p.Home, "missing", name), filepath.Join(p.OpenCodeSkills(), name))
	}
	// Two real directories, reported and never touched.
	for _, name := range []string{"notes-a", "notes-b"} {
		if err := os.MkdirAll(filepath.Join(p.OpenCodeSkills(), name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A pattern rule disables tdd and trace: one manual-edit group.
	if err := os.MkdirAll(filepath.Dir(p.OpenCodeConfig()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(`{"permission": {"skill": {"t*": "deny"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "")

	redundant := "scans the canonical store natively — this link double-covers the skill"
	if got := strings.Count(out, redundant); got != 2 {
		t.Errorf("redundant cause appears %d times, want 2 (opencode, pi):\n%s", got, out)
	}
	if got := strings.Count(out, "alpha, bravo\n"); got != 2 {
		t.Errorf("redundant names appear %d times, want 2:\n%s", got, out)
	}
	if got := strings.Count(out, "the symlink target is missing"); got != 1 {
		t.Errorf("broken cause appears %d times, want 1:\n%s", got, out)
	}
	if !strings.Contains(out, "gone-a, gone-b\n") {
		t.Errorf("broken names not grouped:\n%s", out)
	}
	if got := strings.Count(out, "a real directory, not a symlink — left alone"); got != 1 {
		t.Errorf("unknown cause appears %d times, want 1:\n%s", got, out)
	}
	if !strings.Contains(out, "notes-a, notes-b\n") {
		t.Errorf("unknown names not grouped:\n%s", out)
	}
	if !strings.Contains(out, "disabled by an entry fleet doesn't manage (a pattern or blanket rule) — edit the config by hand if that's wrong") {
		t.Errorf("manual-edit cause missing:\n%s", out)
	}
	if !strings.Contains(out, "tdd, trace\n") {
		t.Errorf("manual-edit names not grouped:\n%s", out)
	}
	// The old per-finding sentences are gone.
	for _, prose := range []string{`link "alpha"`, `"notes-a" —`, `"tdd" is disabled`} {
		if strings.Contains(out, prose) {
			t.Errorf("per-finding prose %q still present:\n%s", prose, out)
		}
	}
}

// TestDoctorGroupsStaleLockByHome checks that stale lockfile entries collapse
// to one line per custom home (with the home path shortened) and the skill
// names under it, instead of one long sentence each.
func TestDoctorGroupsStaleLockByHome(t *testing.T) {
	p := doctorHome(t)
	tracked := filepath.Join(p.Home, "repos")
	if err := os.MkdirAll(filepath.Join(tracked, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeAdoptConfig(t, p, `{"skillsRepos": ["`+tracked+`"]}`)
	names := []string{"lock-a", "lock-b", "lock-c"}
	for _, name := range names {
		writeSkillDir(t, filepath.Join(tracked, "skills"), name, "Does "+name+".")
	}
	writeSkillDir(t, p.FleetHomeSkills(), "lock-d", "Does lock-d.")
	lock := `{"skills": {`
	for i, name := range append(append([]string{}, names...), "lock-d") {
		if i > 0 {
			lock += ","
		}
		lock += fmt.Sprintf("%q: {\"source\": \"mattpocock/skills\", \"sourceType\": \"github\", \"skillFolderHash\": \"abc123\"}", name)
	}
	lock += `}}`
	if err := os.WriteFile(p.SkillLock(), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runDoctor(t, p, "")

	if !strings.Contains(out, "stale lockfile entries (4) · fleet never writes the lockfile") {
		t.Errorf("stale-lock header missing:\n%s", out)
	}
	// One line per home, shortened to ~, names hanging under it.
	if !strings.Contains(out, "the explicit repo  "+shortenHome(p.Home, filepath.Join(tracked, "skills"))) {
		t.Errorf("tracked home line missing:\n%s", out)
	}
	if !strings.Contains(out, "lock-a, lock-b, lock-c\n") {
		t.Errorf("tracked names not grouped:\n%s", out)
	}
	if !strings.Contains(out, "the fleet home") || !strings.Contains(out, shortenHome(p.Home, p.FleetHomeSkills())) {
		t.Errorf("fallback home line missing:\n%s", out)
	}
	if !strings.Contains(out, "lock-d\n") {
		t.Errorf("fallback name missing:\n%s", out)
	}
	if strings.Contains(out, "still carries its install entry") {
		t.Errorf("per-finding stale-lock prose still present:\n%s", out)
	}
	if strings.Contains(out, p.Home) {
		t.Errorf("absolute home path still shown:\n%s", out)
	}
}

func TestShortenHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	cases := []struct {
		home, in, want string
	}{
		{home, filepath.Join(home, ".config", "opencode", "skills"), filepath.Join("~", ".config", "opencode", "skills")},
		{home, home, "~"},
		{home, filepath.Join(string(filepath.Separator), "elsewhere", "skills"), filepath.Join(string(filepath.Separator), "elsewhere", "skills")},
		{home, "", ""},
		{"", filepath.Join(home, ".config"), filepath.Join(home, ".config")},
	}
	for _, c := range cases {
		if got := shortenHome(c.home, c.in); got != c.want {
			t.Errorf("shortenHome(%q, %q) = %q, want %q", c.home, c.in, got, c.want)
		}
	}
}

func TestDoctorReportsTrackedSetProblems(t *testing.T) {
	// A hand-edited skillsDirs list with a missing entry, an empty entry,
	// and a repeated entry: each is its own section in the report.
	p := doctorHome(t)
	missing := filepath.Join(t.TempDir(), "gone")
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	duplicated := filepath.Join(t.TempDir(), "duplicated")
	writeSkillDir(t, duplicated, "helper", "Helper.")
	body, err := json.Marshal(map[string]any{
		"skillsDirs": []string{missing, empty, duplicated, duplicated},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeAdoptConfig(t, p, string(body))

	out := runDoctor(t, p, "")

	if !strings.Contains(out, "missing tracked dirs (1)") {
		t.Errorf("output missing the missing-tracked-dir section:\n%s", out)
	}
	if !strings.Contains(out, missing) {
		t.Errorf("output missing the missing dir path:\n%s", out)
	}
	if !strings.Contains(out, "empty tracked dirs (1)") {
		t.Errorf("output missing the empty-tracked-dir section:\n%s", out)
	}
	if !strings.Contains(out, "overlapping tracked dirs (1)") {
		t.Errorf("output missing the overlapping-tracked-dir section:\n%s", out)
	}
	if !strings.Contains(out, "more than once") {
		t.Errorf("output missing the duplicate explanation:\n%s", out)
	}
	if !strings.Contains(out, "1 missing tracked dir, 1 empty tracked dir, 1 overlapping tracked dir") {
		t.Errorf("output missing the count summary:\n%s", out)
	}
}
