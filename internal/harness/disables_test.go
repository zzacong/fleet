package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zacong/fleet/internal/paths"
)

// The doctor seam on the read side: Disables enumerates the exact-name
// disable entries in a harness's own config, whether or not the skills are
// in the canonical store, so doctor can compare config against state
// without writing anything. Patterns and blanket rules are excluded — they
// surface as flags, never as keep/restore conflicts.

func disablesFor(t *testing.T, p *paths.Paths, a Adapter) []string {
	t.Helper()
	res, err := a.Read(nil)
	if err != nil {
		t.Fatalf("%s Read() error = %v", a.Harness(), err)
	}
	return res.Disables
}

// emptyHome builds a home with every config dir present and nothing else.
func emptyHome(t *testing.T) *paths.Paths {
	t.Helper()
	home := t.TempDir()
	p := paths.New(home)
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := mkdir(dir); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := mkdir(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDisablesOpenCodeExactDeniesOnly(t *testing.T) {
	p := emptyHome(t)
	writeConfig(t, p.OpenCodeConfig(), `{
		// comment
		"permission": { "skill": { "exact-deny": "deny", "allowed": "allow", "t*": "deny" } },
		"permissions": [
			{ "action": "skill", "resource": "v2-deny", "effect": "deny" },
			{ "action": "skill", "resource": "v2-glob*", "effect": "deny" },
			{ "action": "*", "resource": "blanket", "effect": "deny" }
		]
	}`)

	got := disablesFor(t, p, NewOpenCode(p))
	if want := []string{"exact-deny", "v2-deny"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Disables = %v, want %v (patterns and blankets are not exact entries)", got, want)
	}
}

func TestDisablesPiExactExclusionsOnly(t *testing.T) {
	p := emptyHome(t)
	writeConfig(t, p.PiSettings(), `{"skills": ["-skills/exact/SKILL.md", "!glob*", "/some/path"]}`)

	got := disablesFor(t, p, NewPi(p))
	if want := []string{"exact"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Disables = %v, want %v", got, want)
	}
}

func TestDisablesCodexDisabledEntries(t *testing.T) {
	p := emptyHome(t)
	writeConfig(t, p.CodexConfig(), `
[[skills.config]]
name = "off-one"
enabled = false

[[skills.config]]
name = "explicitly-on"
enabled = true

[[skills.config]]
path = "/agents/skills/off-path/SKILL.md"
enabled = false
`)

	got := disablesFor(t, p, NewCodex(p))
	if want := []string{"off-one", "off-path"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Disables = %v, want %v", got, want)
	}
}

func TestDisablesClaudeOffOverridesOnly(t *testing.T) {
	p := emptyHome(t)
	writeConfig(t, p.ClaudeSettings(), `{"skillOverrides": {"off-one": "off", "partial": "user-invocable-only"}}`)

	got := disablesFor(t, p, NewClaude(p))
	if want := []string{"off-one"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Disables = %v, want %v", got, want)
	}
}

func TestDisablesEmptyWithoutConfigLever(t *testing.T) {
	p := emptyHome(t)

	for _, a := range All(p) {
		if got := disablesFor(t, p, a); got != nil {
			t.Errorf("%s Disables with no config = %v, want none", a.Harness(), got)
		}
	}
}
