// The update command tests: the wrapped skills CLI run goes through the
// injected runner seam (the binary does not exist here), the post-run
// sync re-applies what the run disturbed, and the report is built from
// fleet's own state — never from the CLI's prose.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/skillscli"
	"github.com/zzacong/fleet/internal/state"
)

// updateHome is toggleHome's home plus the skills CLI lockfile: tdd has
// provenance, so the update report can count installed vs custom.
func updateHome(t *testing.T) *paths.Paths {
	t.Helper()
	p := toggleHome(t)
	lock := `{"version": 3, "skills": {"tdd": {
		"source": "mattpocock/skills",
		"sourceType": "github",
		"skillFolderHash": "aaa111"
	}}}`
	if err := os.WriteFile(p.SkillLock(), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// swapRunner replaces the command-runner seam for one test.
func swapRunner(t *testing.T, r skillscli.Runner) {
	t.Helper()
	prev := newSkillsRunner
	newSkillsRunner = func() skillscli.Runner { return r }
	t.Cleanup(func() { newSkillsRunner = prev })
}

func runUpdate(t *testing.T, p *paths.Paths) string {
	t.Helper()
	out, _, err := runUpdateCapture(t, p)
	if err != nil {
		t.Fatalf("fleet skill update: error = %v", err)
	}
	return out
}

func runUpdateCapture(t *testing.T, p *paths.Paths) (string, string, error) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"skill", "update"})
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	err := root.Execute()
	return out.String(), errOut.String(), err
}

// resurrect simulates the wrapped skills CLI run's side effects: it
// re-creates opencode's redundant per-agent link and claude's load-bearing
// one, and strips pi's exclusion — the drift fleet's post-run sync undoes.
func resurrect(t *testing.T, p *paths.Paths) {
	t.Helper()
	store := filepath.Join(p.SkillsStore(), "tdd")
	for _, dir := range []string{p.OpenCodeSkills(), p.ClaudeSkills()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(store, filepath.Join(dir, "tdd")); err != nil {
			t.Fatal(err)
		}
	}
	stripPiExclusion(t, p.PiSettings(), "tdd")
}

// stripPiExclusion rewrites pi's settings without the skill's exclusion,
// as a config rewrite during the wrapped run would.
func stripPiExclusion(t *testing.T, path, name string) {
	t.Helper()
	var settings struct {
		Skills []string `json:"skills"`
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &settings); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	needle := "-skills/" + name + "/SKILL.md"
	var kept []string
	for _, entry := range settings.Skills {
		if entry != needle {
			kept = append(kept, entry)
		}
	}
	settings.Skills = kept
	if settings.Skills == nil {
		settings.Skills = []string{} // a real rewrite writes [], not null
	}
	out, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateReconcilesAfterTheWrappedRun(t *testing.T) {
	p := updateHome(t)

	// The user disabled tdd before updating; the markers are in place.
	runToggle(t, p, "off", "tdd")
	if body := readFile(t, p.PiSettings()); !strings.Contains(body, "-skills/tdd/SKILL.md") {
		t.Fatalf("setup: pi exclusion missing:\n%s", body)
	}
	lockBefore := readFile(t, p.SkillLock())

	// The wrapped run re-creates links, loses a marker, and claims
	// success — exactly the mess sync exists to clean up.
	var inv skillscli.Invocation
	swapRunner(t, skillscli.RunnerFunc(func(i skillscli.Invocation) (skillscli.Result, error) {
		inv = i
		resurrect(t, p)
		return skillscli.Result{Stdout: "✔ updated 1 skill"}, nil
	}))

	out := runUpdate(t, p)

	// The wrapped call went through the seam with the explicit flags.
	if inv.Exe != "skills" || !reflect.DeepEqual(inv.Args, []string{"update", "-g", "-y"}) {
		t.Errorf("wrapped call = %s %v, want skills update -g -y", inv.Exe, inv.Args)
	}

	// Disabled stays disabled: the markers are (back) in place.
	if body := readFile(t, p.PiSettings()); !strings.Contains(body, "-skills/tdd/SKILL.md") {
		t.Errorf("pi exclusion not re-applied:\n%s", body)
	}
	if body := readFile(t, p.ClaudeSettings()); !strings.Contains(body, `"tdd": "off"`) {
		t.Errorf("claude override not re-applied:\n%s", body)
	}
	// Cleaned links stay clean.
	if _, err := os.Lstat(filepath.Join(p.OpenCodeSkills(), "tdd")); !os.IsNotExist(err) {
		t.Error("the re-created redundant link was not removed")
	}

	// The state file is untouched: still the single source of truth.
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"opencode", "pi", "codex", "claude"} {
		if !st.IsDisabled("tdd", h) {
			t.Errorf("state: tdd/%s no longer disabled", h)
		}
	}
	// Fleet never writes the skills CLI's lockfile.
	if after := readFile(t, p.SkillLock()); after != lockBefore {
		t.Errorf("lockfile changed:\n%s\nwas\n%s", after, lockBefore)
	}

	// Results are fleet's own post-run state: what sync did, and the
	// verified disable matrix — never the CLI's prose.
	for _, want := range []string{
		`sync: claude: disabled "tdd" (was on)`,
		`sync: pi: disabled "tdd" (was on)`,
		`sync: opencode: removed redundant link "tdd" — opencode scans the canonical store natively`,
		"1 skill in " + p.SkillsStore() + " (1 installed, 0 custom)",
		`disabled: "tdd" for opencode, pi, codex, claude`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "✔ updated") {
		t.Errorf("the skills CLI's prose leaked into fleet's report:\n%s", out)
	}
}

func TestUpdateReportsADisableThatOutlivedItsSkill(t *testing.T) {
	// tdd was disabled for pi, then the skill was uninstalled by hand.
	// The state entry and pi's exclusion remain, sync keeps them, and the
	// report counts the disable as held — not as lost.
	p := updateHome(t)
	runToggle(t, p, "off", "tdd", "--harness", "pi")
	if err := os.RemoveAll(filepath.Join(p.SkillsStore(), "tdd")); err != nil {
		t.Fatal(err)
	}
	swapRunner(t, skillscli.RunnerFunc(func(skillscli.Invocation) (skillscli.Result, error) {
		return skillscli.Result{}, nil
	}))

	out := runUpdate(t, p)

	if !strings.Contains(out, `disabled: "tdd" for pi`) {
		t.Errorf("output missing the stale-but-held disable:\n%s", out)
	}
	if strings.Contains(out, "did not stay disabled") {
		t.Errorf("a disable sync keeps was reported as lost:\n%s", out)
	}
	if body := readFile(t, p.PiSettings()); !strings.Contains(body, "-skills/tdd/SKILL.md") {
		t.Errorf("pi exclusion not re-applied:\n%s", body)
	}
	if !strings.Contains(out, "0 skills in "+p.SkillsStore()+" (0 installed, 0 custom)") {
		t.Errorf("output missing the emptied-store summary:\n%s", out)
	}
}

func TestUpdateShowsRawOutputWhenTheWrappedRunFails(t *testing.T) {
	p := updateHome(t)
	swapRunner(t, skillscli.RunnerFunc(func(skillscli.Invocation) (skillscli.Result, error) {
		return skillscli.Result{Stdout: "updating tdd...", Stderr: "? unexpected prompt"}, errors.New("exit status 1")
	}))

	out, errOut, err := runUpdateCapture(t, p)

	if err == nil {
		t.Fatal("update succeeded on a failed wrapped run, want error")
	}
	if !strings.Contains(errOut, "updating tdd...") || !strings.Contains(errOut, "? unexpected prompt") {
		t.Errorf("stderr missing the raw skills CLI output:\n%s", errOut)
	}
	if !strings.Contains(err.Error(), "skills update -g -y") {
		t.Errorf("error = %v, want the wrapped invocation named", err)
	}
	if out != "" {
		t.Errorf("stdout not empty on failure:\n%s", out)
	}
}

func TestUpdateWithNothingToDoIsClean(t *testing.T) {
	// Upstream didn't change anything: no drift, no findings, no error —
	// just fleet's own summary.
	p := updateHome(t)
	swapRunner(t, skillscli.RunnerFunc(func(skillscli.Invocation) (skillscli.Result, error) {
		return skillscli.Result{Stdout: "everything up to date"}, nil
	}))

	out, errOut, err := runUpdateCapture(t, p)
	if err != nil {
		t.Fatalf("fleet skill update: error = %v", err)
	}
	if strings.Contains(out+errOut, "sync:") {
		t.Errorf("a no-op run reported sync findings:\n%s\n--\n%s", out, errOut)
	}
	if !strings.Contains(out, "1 skill in "+p.SkillsStore()+" (1 installed, 0 custom)") {
		t.Errorf("output missing the store summary:\n%s", out)
	}
	if strings.Contains(out, "disabled:") {
		t.Errorf("nothing was disabled, yet the report lists disables:\n%s", out)
	}
}
