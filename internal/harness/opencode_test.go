package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zacong/fleet/internal/paths"
)

func writeOpenCodeConfig(t *testing.T, home, body string) {
	t.Helper()
	p := paths.New(home)
	if err := os.MkdirAll(p.OpenCodeDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenCodeReadsV1PermissionSkillMap(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeOpenCodeConfig(t, home, `{
		"$schema": "https://opencode.ai/config.json",
		"permission": {
			// skills policy: allow all, deny the git helpers, un-deny one
			"skill": {
				"*": "allow",
				"git-*": "deny",
				"git-secret": "allow",
			},
		},
	}`)

	a := NewOpenCode(paths.New(home))
	res, err := a.Read([]string{"tdd", "git-helper", "git-secret"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	want := map[string]State{"tdd": StateOn, "git-helper": StateOff, "git-secret": StateOn}
	if !reflect.DeepEqual(res.States, want) {
		t.Errorf("States = %v, want %v (last matching rule must win)", res.States, want)
	}
	if res.Dialect != "v1" {
		t.Errorf("Dialect = %q, want v1", res.Dialect)
	}
	if res.Linked != nil {
		t.Errorf("Linked = %v, want nil for opencode", res.Linked)
	}
}

func TestOpenCodeReadsV1ShorthandString(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeOpenCodeConfig(t, home, `{"permission": {"skill": "deny"}}`)

	a := NewOpenCode(paths.New(home))
	res, err := a.Read([]string{"tdd"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if res.States["tdd"] != StateOff {
		t.Errorf("state = %q, want off (shorthand denies every skill)", res.States["tdd"])
	}
}

func TestOpenCodeReadsV2PermissionsRules(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeOpenCodeConfig(t, home, `{
		"permissions": [
			{ "action": "skill", "resource": "*", "effect": "allow" },
			{ "action": "skill", "resource": "internal-*", "effect": "deny" },
		],
	}`)

	a := NewOpenCode(paths.New(home))
	res, err := a.Read([]string{"tdd", "internal-tools"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	want := map[string]State{"tdd": StateOn, "internal-tools": StateOff}
	if !reflect.DeepEqual(res.States, want) {
		t.Errorf("States = %v, want %v (last matching rule must win)", res.States, want)
	}
	if res.Dialect != "v2" {
		t.Errorf("Dialect = %q, want v2", res.Dialect)
	}
}

func TestOpenCodeV2WildcardActionMatchesSkills(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeOpenCodeConfig(t, home, `{
		"permissions": [
			{ "action": "*", "resource": "*", "effect": "deny" },
		],
	}`)

	a := NewOpenCode(paths.New(home))
	res, err := a.Read([]string{"tdd"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if res.States["tdd"] != StateOff {
		t.Errorf("state = %q, want off (action * matches the skill action)", res.States["tdd"])
	}
}

func TestOpenCodeMixedFileV2RulesOverrideV1(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeOpenCodeConfig(t, home, `{
		"permission": {
			"skill": { "tdd": "deny" },
		},
		"permissions": [
			{ "action": "skill", "resource": "tdd", "effect": "allow" },
		],
	}`)

	a := NewOpenCode(paths.New(home))
	res, err := a.Read([]string{"tdd"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if res.States["tdd"] != StateOn {
		t.Errorf("state = %q, want on (V2 auto-migrates V1 keys, so V2 decides)", res.States["tdd"])
	}
	if res.Dialect != "v2" {
		t.Errorf("Dialect = %q, want v2", res.Dialect)
	}
}

func TestOpenCodeAskEffectLeavesSkillOn(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeOpenCodeConfig(t, home, `{"permissions": [{"action": "skill", "resource": "tdd", "effect": "ask"}]}`)

	a := NewOpenCode(paths.New(home))
	res, err := a.Read([]string{"tdd"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if res.States["tdd"] != StateOn {
		t.Errorf("state = %q, want on (ask advertises the skill, only deny hides it)", res.States["tdd"])
	}
}

func TestOpenCodeDialectDetection(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"v1 permission object", `{"permission": {"edit": "ask"}}`, "v1"},
		{"v2 permissions array", `{"permissions": []}`, "v2"},
		{"skills object only is never v1", `{"skills": {"paths": ["./x"], "urls": []}}`, "v2"},
		{"skills array only", `{"skills": ["./x"]}`, "v2"},
		{"no markers at all", `{"model": "gpt-5"}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "home")
			writeOpenCodeConfig(t, home, c.body)
			res, err := NewOpenCode(paths.New(home)).Read([]string{"tdd"})
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
			if res.Dialect != c.want {
				t.Errorf("Dialect = %q, want %q", res.Dialect, c.want)
			}
		})
	}
}

func TestOpenCodeReadsSkillSourcesInBothDialects(t *testing.T) {
	t.Run("v1 object form", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "home")
		writeOpenCodeConfig(t, home, `{"skills": {"paths": ["./team-skills", "~/shared"], "urls": ["https://example.com/skills/"]}}`)

		res, err := NewOpenCode(paths.New(home)).Read([]string{"tdd"})
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		want := []string{"./team-skills", "~/shared", "https://example.com/skills/"}
		if !reflect.DeepEqual(res.SkillSources, want) {
			t.Errorf("SkillSources = %v, want %v", res.SkillSources, want)
		}
	})

	t.Run("v2 flat array form", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "home")
		writeOpenCodeConfig(t, home, `{"skills": ["./team-skills", "https://example.com/skills/"]}`)

		res, err := NewOpenCode(paths.New(home)).Read([]string{"tdd"})
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		want := []string{"./team-skills", "https://example.com/skills/"}
		if !reflect.DeepEqual(res.SkillSources, want) {
			t.Errorf("SkillSources = %v, want %v", res.SkillSources, want)
		}
	})

	t.Run("no skills key", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "home")
		writeOpenCodeConfig(t, home, `{}`)

		res, err := NewOpenCode(paths.New(home)).Read([]string{"tdd"})
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if res.SkillSources != nil {
			t.Errorf("SkillSources = %v, want nil", res.SkillSources)
		}
	})
}

func TestOpenCodeMalformedConfigIsAnError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	writeOpenCodeConfig(t, home, `{"permission": {`)

	_, err := NewOpenCode(paths.New(home)).Read([]string{"tdd"})
	if err == nil {
		t.Fatal("Read() on malformed JSONC should fail")
	}
}
