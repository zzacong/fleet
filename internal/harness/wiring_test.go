package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zacong/fleet/internal/paths"
)

const repoSkillsDir = "/repo/skills"

// wiringHome builds a home with every harness installed and, when
// configs is non-empty, writes each named config file.
func wiringHome(t *testing.T, configs map[string]string) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.ClaudeDir(), p.CursorDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range configs {
		path := map[string]string{
			"opencode": p.OpenCodeConfig(),
			"pi":       p.PiSettings(),
		}[name]
		if path == "" {
			t.Fatalf("unknown config %q", name)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestWireSkillSourceCreatesTheV1ShapeForAFreshOpenCodeConfig(t *testing.T) {
	p := wiringHome(t, nil)

	res, err := WireSkillSource(p, repoSkillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("wired %d harnesses, want opencode and pi: %v", len(res), res)
	}

	body := readFileBody(t, p.OpenCodeConfig())
	if !strings.Contains(body, repoSkillsDir) {
		t.Errorf("opencode config missing the repo path:\n%s", body)
	}
	if !strings.Contains(body, `"paths"`) {
		t.Errorf("fresh opencode config should use the V1 skills.paths shape:\n%s", body)
	}
}

func TestWireSkillSourceFollowsTheShapeTheOpenCodeFileUses(t *testing.T) {
	t.Run("a V2 file gets the flat skills array", func(t *testing.T) {
		p := wiringHome(t, map[string]string{"opencode": `{
			"$schema": "https://opencode.ai/config.json",
			"permissions": [{ "action": "skill", "resource": "*", "effect": "allow" }],
		}`})

		if _, err := WireSkillSource(p, repoSkillsDir); err != nil {
			t.Fatal(err)
		}
		body := readFileBody(t, p.OpenCodeConfig())
		if !strings.Contains(body, `"`+repoSkillsDir+`"`) {
			t.Errorf("repo path not wired:\n%s", body)
		}
		if strings.Contains(body, `"paths"`) {
			t.Errorf("a V2 file must get the flat skills array, not skills.paths:\n%s", body)
		}
		// Everything fleet doesn't own survives.
		if !strings.Contains(body, `"resource": "*"`) || !strings.Contains(body, "$schema") {
			t.Errorf("existing config was clobbered:\n%s", body)
		}
	})

	t.Run("a V1 file gets skills.paths", func(t *testing.T) {
		p := wiringHome(t, map[string]string{"opencode": `{
			"permission": { "skill": { "git-*": "deny" } },
		}`})

		if _, err := WireSkillSource(p, repoSkillsDir); err != nil {
			t.Fatal(err)
		}
		body := readFileBody(t, p.OpenCodeConfig())
		if !strings.Contains(body, `"paths"`) || !strings.Contains(body, repoSkillsDir) {
			t.Errorf("a V1 file must get skills.paths:\n%s", body)
		}
		if !strings.Contains(body, `"git-*": "deny"`) {
			t.Errorf("existing permission rules were clobbered:\n%s", body)
		}
	})

	t.Run("an existing V1 skills object gains a paths entry", func(t *testing.T) {
		p := wiringHome(t, map[string]string{"opencode": `{
			"skills": { "paths": ["/other/skills"], "urls": ["https://example.com"] },
		}`})

		if _, err := WireSkillSource(p, repoSkillsDir); err != nil {
			t.Fatal(err)
		}
		body := readFileBody(t, p.OpenCodeConfig())
		for _, want := range []string{`"/other/skills"`, `"` + repoSkillsDir + `"`, `"https://example.com"`} {
			if !strings.Contains(body, want) {
				t.Errorf("config missing %q:\n%s", want, body)
			}
		}
	})

	t.Run("an existing V2 skills array gains an entry", func(t *testing.T) {
		p := wiringHome(t, map[string]string{"opencode": `{
			"skills": ["/other/skills"],
		}`})

		if _, err := WireSkillSource(p, repoSkillsDir); err != nil {
			t.Fatal(err)
		}
		body := readFileBody(t, p.OpenCodeConfig())
		if !strings.Contains(body, `"`+repoSkillsDir+`"`) || !strings.Contains(body, `"/other/skills"`) {
			t.Errorf("config missing the wired paths:\n%s", body)
		}
	})
}

func TestWireSkillSourceIsIdempotent(t *testing.T) {
	p := wiringHome(t, map[string]string{"opencode": `{"skills": ["` + repoSkillsDir + `"]}`})
	before := readFileBody(t, p.OpenCodeConfig())

	res, err := WireSkillSource(p, repoSkillsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Harness == OpenCode && r.Changed {
			t.Errorf("opencode rewired an already-wired path: %v", res)
		}
	}
	if after := readFileBody(t, p.OpenCodeConfig()); after != before {
		t.Errorf("an already-wired config must not be rewritten:\n%s\nwas\n%s", after, before)
	}
}

func TestWireSkillSourceRejectsUnusableShapes(t *testing.T) {
	p := wiringHome(t, map[string]string{"opencode": `{"skills": "just a string"}`})
	if _, err := WireSkillSource(p, repoSkillsDir); err == nil {
		t.Error("wiring into a string skills value should fail")
	}

	p = wiringHome(t, map[string]string{"opencode": `{"skills": {"paths": "not an array"}}`})
	if _, err := WireSkillSource(p, repoSkillsDir); err == nil {
		t.Error("wiring into a non-array skills.paths should fail")
	}

	p = wiringHome(t, map[string]string{"pi": `{"skills": {"paths": []}}`})
	if _, err := WireSkillSource(p, repoSkillsDir); err == nil {
		t.Error("pi wiring into a non-array skills should fail")
	}
}

func TestWireSkillSourceAppendsToThePiSkillsArray(t *testing.T) {
	p := wiringHome(t, map[string]string{"pi": `{"skills": ["-skills/tdd/SKILL.md"]}`})

	if _, err := WireSkillSource(p, repoSkillsDir); err != nil {
		t.Fatal(err)
	}
	body := readFileBody(t, p.PiSettings())
	if !strings.Contains(body, `"`+repoSkillsDir+`"`) {
		t.Errorf("pi settings missing the repo path:\n%s", body)
	}
	if !strings.Contains(body, "-skills/tdd/SKILL.md") {
		t.Errorf("pi exclusion entry was clobbered:\n%s", body)
	}

	// Fresh settings file: created with the wired path.
	p = wiringHome(t, nil)
	if _, err := WireSkillSource(p, repoSkillsDir); err != nil {
		t.Fatal(err)
	}
	if body := readFileBody(t, p.PiSettings()); !strings.Contains(body, repoSkillsDir) {
		t.Errorf("pi settings not created with the repo path:\n%s", body)
	}
}

func TestWireSkillSourceSkipsUninstalledHarnesses(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	if err := os.MkdirAll(p.OpenCodeDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := WireSkillSource(p, repoSkillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Harness != OpenCode {
		t.Errorf("results = %v, want only opencode", res)
	}
	if _, err := os.Stat(p.PiSettings()); !os.IsNotExist(err) {
		t.Error("pi settings created although pi is not installed")
	}
}

func readFileBody(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
