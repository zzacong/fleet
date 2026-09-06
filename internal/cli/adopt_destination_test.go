package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/paths"
)

// adoptDestHome builds a fake home with no legacy repo pointer, harness
// dirs installed, and the given store skills. Callers add tracked slots or
// config keys on top.
func adoptDestHome(t *testing.T, storeSkills ...string) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	t.Setenv("FLEET_REPO", "")
	for _, name := range storeSkills {
		writeSkillDir(t, p.SkillsStore(), name, "Does "+name+" things.")
	}
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func writeAdoptConfig(t *testing.T, p *paths.Paths, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p.FleetConfigFile()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.FleetConfigFile(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runAdoptFull(t *testing.T, p *paths.Paths, stdin string, args ...string) (string, error) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	if stdin != "" {
		root.SetIn(strings.NewReader(stdin))
	}
	root.SetArgs(append([]string{"skill", "adopt"}, args...))
	err := root.Execute()
	return out.String(), err
}

func stubTTY(t *testing.T, stdout, stdin bool) {
	t.Helper()
	stdoutTTY = func() bool { return stdout }
	stdinTTY = func() bool { return stdin }
	t.Cleanup(func() {
		stdoutTTY = func() bool { return false }
		stdinTTY = func() bool { return false }
	})
}

func TestAdoptIntoFlagWinsWithNoPromptNoConfigWrite(t *testing.T) {
	stubTTY(t, false, false)
	tracked := filepath.Join(t.TempDir(), "tracked")
	configured := filepath.Join(t.TempDir(), "configured", "skills")
	p := adoptDestHome(t, "my-notes")
	writeAdoptConfig(t, p, `{"skillsRepos": ["`+tracked+`"], "adoptTarget": "`+configured+`"}`)
	into := filepath.Join(t.TempDir(), "one-off", "skills")

	out, err := runAdoptFull(t, p, "", "my-notes", "--into", into)
	if err != nil {
		t.Fatalf("adopt --into: %v", err)
	}
	if _, err := os.Stat(filepath.Join(into, "my-notes", "SKILL.md")); err != nil {
		t.Errorf("skill not moved into --into dir: %v", err)
	}
	if strings.Contains(out, "choose") || strings.Contains(out, "[1]") {
		t.Errorf("flag run must not prompt:\n%s", out)
	}
	// No config write: the configured target is untouched.
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	if f.AdoptTarget() != configured {
		t.Errorf("AdoptTarget() = %q, want configured %q untouched", f.AdoptTarget(), configured)
	}
}

func TestAdoptConfiguredTargetWinsWithNoPrompt(t *testing.T) {
	stubTTY(t, false, false)
	tracked := filepath.Join(t.TempDir(), "tracked")
	configured := filepath.Join(t.TempDir(), "configured", "skills")
	p := adoptDestHome(t, "my-notes")
	writeAdoptConfig(t, p, `{"skillsRepos": ["`+tracked+`"], "adoptTarget": "`+configured+`"}`)

	out, err := runAdoptFull(t, p, "", "my-notes")
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configured, "my-notes", "SKILL.md")); err != nil {
		t.Errorf("skill not moved into configured target: %v", err)
	}
	if strings.Contains(out, "choose") || strings.Contains(out, "[1]") {
		t.Errorf("configured-target run must not prompt:\n%s", out)
	}
}

func TestAdoptZeroTrackedUsesFallbackNoPrompt(t *testing.T) {
	stubTTY(t, false, false)
	p := adoptDestHome(t, "my-notes")

	out, err := runAdoptFull(t, p, "", "my-notes")
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.FleetHomeSkills(), "my-notes", "SKILL.md")); err != nil {
		t.Errorf("skill not moved into fallback: %v", err)
	}
	if strings.Contains(out, "choose") {
		t.Errorf("fallback run must not prompt:\n%s", out)
	}
}

func TestAdoptPromptAlwaysIncludesFallback(t *testing.T) {
	stubTTY(t, false, true)
	tracked := filepath.Join(t.TempDir(), "tracked")
	p := adoptDestHome(t, "my-notes")
	writeAdoptConfig(t, p, `{"skillsRepos": ["`+tracked+`"]}`)
	trackedSkills := filepath.Join(tracked, "skills")

	// One tracked collection means two options; pick 2 (the fallback),
	// then answer the save-back with empty (default No).
	out, err := runAdoptFull(t, p, "2\n\n", "my-notes")
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.FleetHomeSkills(), "my-notes", "SKILL.md")); err != nil {
		t.Errorf("skill not moved into chosen fallback: %v", err)
	}
	for _, want := range []string{trackedSkills, p.FleetHomeSkills()} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt missing candidate %q:\n%s", want, out)
		}
	}
	// Default-No save-back: nothing persisted.
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	if f.AdoptTarget() != "" {
		t.Errorf("AdoptTarget() = %q, want empty after default-No save-back", f.AdoptTarget())
	}
}

func TestAdoptPromptChoiceIsOneShotAndSavebackYesPersists(t *testing.T) {
	stubTTY(t, false, true)
	tracked := filepath.Join(t.TempDir(), "tracked")
	p := adoptDestHome(t, "my-notes")
	writeAdoptConfig(t, p, `{"skillsRepos": ["`+tracked+`"]}`)
	trackedSkills := filepath.Join(tracked, "skills")

	out, err := runAdoptFull(t, p, "1\ny\n", "my-notes")
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(trackedSkills, "my-notes", "SKILL.md")); err != nil {
		t.Errorf("skill not moved into chosen tracked collection: %v", err)
	}
	if !strings.Contains(out, trackedSkills) {
		t.Errorf("output should mention the chosen destination:\n%s", out)
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	if f.AdoptTarget() != trackedSkills {
		t.Errorf("AdoptTarget() = %q, want persisted %q after yes save-back", f.AdoptTarget(), trackedSkills)
	}
}

func TestAdoptPipedAmbiguousFailsWithCandidatesAndFlagHint(t *testing.T) {
	stubTTY(t, false, false)
	tracked := filepath.Join(t.TempDir(), "tracked")
	p := adoptDestHome(t, "my-notes")
	writeAdoptConfig(t, p, `{"skillsRepos": ["`+tracked+`"]}`)

	_, err := runAdoptFull(t, p, "", "my-notes")
	if err == nil {
		t.Fatal("piped ambiguous adopt should fail instead of blocking")
	}
	msg := err.Error()
	if !strings.Contains(msg, filepath.Join(tracked, "skills")) || !strings.Contains(msg, p.FleetHomeSkills()) {
		t.Errorf("error should list candidates, got: %v", err)
	}
	if !strings.Contains(msg, "--into") {
		t.Errorf("error should hint the flag, got: %v", err)
	}
	// Nothing moved on failure.
	if _, statErr := os.Stat(filepath.Join(p.FleetHomeSkills(), "my-notes")); !os.IsNotExist(statErr) {
		t.Error("failed adopt must not move the skill")
	}
}

func TestConfigAdoptTargetGetSetUnsetList(t *testing.T) {
	stubTTY(t, false, false)
	p := configHome(t)
	target := filepath.Join(t.TempDir(), "custom", "skills")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	// Get when unset is empty.
	out, _, err := runConfig(t, p, "get", "adopt-target")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("get when unset = %q, want empty", out)
	}

	// Set then get round-trips (camel alias on set, kebab on get).
	if _, _, err := runConfig(t, p, "set", "adoptTarget", target); err != nil {
		t.Fatalf("set: %v", err)
	}
	out, _, err = runConfig(t, p, "get", "adopt-target")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.TrimSpace(out) != target {
		t.Errorf("get = %q, want %q", strings.TrimSpace(out), target)
	}

	// List shows the key in text and JSON.
	out, _, err = runConfig(t, p, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "adopt-target = "+target) {
		t.Errorf("list missing adopt-target line:\n%s", out)
	}
	out, _, err = runConfig(t, p, "list", "--json")
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	var obj map[string]string
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatalf("list --json not JSON: %v\n%s", err, out)
	}
	if obj["adoptTarget"] != target {
		t.Errorf("list --json adoptTarget = %q, want %q", obj["adoptTarget"], target)
	}

	// Unset clears.
	if _, _, err := runConfig(t, p, "unset", "adopt-target"); err != nil {
		t.Fatalf("unset: %v", err)
	}
	out, _, err = runConfig(t, p, "get", "adoptTarget")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("after unset get = %q, want empty", out)
	}
}

func TestAdoptPromptInvalidChoiceFailsWithoutMoving(t *testing.T) {
	stubTTY(t, false, true)
	tracked := filepath.Join(t.TempDir(), "tracked")
	p := adoptDestHome(t, "my-notes")
	writeAdoptConfig(t, p, `{"skillsRepos": ["`+tracked+`"]}`)

	_, err := runAdoptFull(t, p, "9\n", "my-notes")
	if err == nil || !strings.Contains(err.Error(), "invalid choice") {
		t.Fatalf("out-of-range choice should fail one-shot, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(p.SkillsStore(), "my-notes")); statErr != nil {
		t.Errorf("failed choice must not move the skill: %v", statErr)
	}
}

func TestConfigSetAdoptTargetExpandsHome(t *testing.T) {
	stubTTY(t, false, false)
	home := t.TempDir()
	t.Setenv("HOME", home)
	p := configHome(t)
	if _, _, err := runConfig(t, p, "set", "adopt-target", "~/customs/skills"); err != nil {
		t.Fatalf("set: %v", err)
	}
	out, _, err := runConfig(t, p, "get", "adopt-target")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != filepath.Join(home, "customs", "skills") {
		t.Errorf("get = %q, want home-expanded path", strings.TrimSpace(out))
	}
}

func TestConfigSetAdoptTargetRejectsRelative(t *testing.T) {
	stubTTY(t, false, false)
	p := configHome(t)
	if _, _, err := runConfig(t, p, "set", "adopt-target", "relative/path"); err == nil {
		t.Error("relative adopt-target should be rejected")
	}
}
