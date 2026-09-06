package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
)

// unwireHome builds a home with the opencode and pi harness dirs installed
// and, when configs is non-empty, writes each named config file.
func unwireHome(t *testing.T, configs map[string]string) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	for _, dir := range []string{p.OpenCodeDir(), p.PiDir()} {
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

func TestUnwireSkillSourceRemovesFromV1Paths(t *testing.T) {
	p := unwireHome(t, map[string]string{"opencode": `{
		"skills": { "paths": ["/other/skills", "` + repoSkillsDir + `"], "urls": ["https://example.com"] },
	}`})

	res, err := UnwireSkillSource(p, repoSkillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Harness != OpenCode || !res[0].Changed {
		t.Fatalf("results = %v, want one changed opencode", res)
	}
	if res[0].Where != "skills.paths" {
		t.Errorf("Where = %q, want skills.paths", res[0].Where)
	}
	body := readFileBody(t, p.OpenCodeConfig())
	if strings.Contains(body, `"`+repoSkillsDir+`"`) {
		t.Errorf("repo path still wired:\n%s", body)
	}
	for _, want := range []string{`"/other/skills"`, `"https://example.com"`} {
		if !strings.Contains(body, want) {
			t.Errorf("config missing %q:\n%s", want, body)
		}
	}
}

func TestUnwireSkillSourceRemovesFromV2Array(t *testing.T) {
	p := unwireHome(t, map[string]string{"opencode": `{
		"skills": ["/other/skills", "` + repoSkillsDir + `"],
	}`})

	res, err := UnwireSkillSource(p, repoSkillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Harness != OpenCode || !res[0].Changed {
		t.Fatalf("results = %v, want one changed opencode", res)
	}
	if res[0].Where != "skills" {
		t.Errorf("Where = %q, want skills", res[0].Where)
	}
	body := readFileBody(t, p.OpenCodeConfig())
	if strings.Contains(body, `"`+repoSkillsDir+`"`) {
		t.Errorf("repo path still wired:\n%s", body)
	}
	if !strings.Contains(body, `"/other/skills"`) {
		t.Errorf("sibling entry was clobbered:\n%s", body)
	}
}

func TestUnwireSkillSourceRemovesFromPiArray(t *testing.T) {
	p := unwireHome(t, map[string]string{"pi": `{"skills": ["-skills/tdd/SKILL.md", "` + repoSkillsDir + `"]}`})

	res, err := UnwireSkillSource(p, repoSkillsDir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range res {
		if r.Harness == Pi && r.Changed {
			found = true
		}
	}
	if !found {
		t.Fatalf("results = %v, want a changed pi", res)
	}
	body := readFileBody(t, p.PiSettings())
	if strings.Contains(body, `"`+repoSkillsDir+`"`) {
		t.Errorf("repo path still wired:\n%s", body)
	}
	if !strings.Contains(body, "-skills/tdd/SKILL.md") {
		t.Errorf("pi exclusion entry was clobbered:\n%s", body)
	}
}

func TestUnwireSkillSourceIsIdempotentNoOpWhenAbsent(t *testing.T) {
	p := unwireHome(t, map[string]string{
		"opencode": `{"skills": ["/other/skills"]}`,
		"pi":       `{"skills": ["-skills/tdd/SKILL.md"]}`,
	})
	beforeOpenCode := readFileBody(t, p.OpenCodeConfig())
	beforePi := readFileBody(t, p.PiSettings())

	res, err := UnwireSkillSource(p, repoSkillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Errorf("results = %v, want no changes for an absent dir", res)
	}
	if after := readFileBody(t, p.OpenCodeConfig()); after != beforeOpenCode {
		t.Errorf("opencode config rewritten on no-op:\n%s\nwas\n%s", after, beforeOpenCode)
	}
	if after := readFileBody(t, p.PiSettings()); after != beforePi {
		t.Errorf("pi settings rewritten on no-op:\n%s\nwas\n%s", after, beforePi)
	}
}

func TestUnwireSkillSourceEmptyRemovalLeavesValidConfig(t *testing.T) {
	p := unwireHome(t, map[string]string{
		"opencode": `{"skills": ["` + repoSkillsDir + `"]}`,
		"pi":       `{"skills": ["` + repoSkillsDir + `"]}`,
	})

	if _, err := UnwireSkillSource(p, repoSkillsDir); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"opencode": p.OpenCodeConfig(), "pi": p.PiSettings()} {
		var v map[string]json.RawMessage
		if body, err := os.ReadFile(path); err != nil {
			t.Fatalf("%s: %v", name, err)
		} else if err := json.Unmarshal(body, &v); err != nil {
			t.Errorf("%s config invalid after empty removal: %v\n%s", name, err, body)
		}
	}
}

func TestUnwireSkillSourceSkipsUninstalledHarnesses(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	if err := os.MkdirAll(p.OpenCodeDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := UnwireSkillSource(p, repoSkillsDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = res
	if _, err := os.Stat(p.PiSettings()); !os.IsNotExist(err) {
		t.Error("pi settings created although pi is not installed")
	}
}

func TestUnwireSkillSourceNeverCreatesMissingConfigs(t *testing.T) {
	p := unwireHome(t, nil)

	res, err := UnwireSkillSource(p, repoSkillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Errorf("results = %v, want no changes when configs are missing", res)
	}
	if _, err := os.Stat(p.OpenCodeConfig()); !os.IsNotExist(err) {
		t.Error("opencode config created although it was missing")
	}
	if _, err := os.Stat(p.PiSettings()); !os.IsNotExist(err) {
		t.Error("pi settings created although they were missing")
	}
}

func TestUnwireSkillSourceRejectsUnusableShapes(t *testing.T) {
	p := unwireHome(t, map[string]string{"opencode": `{"skills": "just a string"}`})
	if _, err := UnwireSkillSource(p, repoSkillsDir); err == nil {
		t.Error("unwiring from a string skills value should fail")
	}

	p = unwireHome(t, map[string]string{"opencode": `{"skills": {"paths": "not an array"}}`})
	if _, err := UnwireSkillSource(p, repoSkillsDir); err == nil {
		t.Error("unwiring from a non-array skills.paths should fail")
	}

	p = unwireHome(t, map[string]string{"pi": `{"skills": {"paths": []}}`})
	if _, err := UnwireSkillSource(p, repoSkillsDir); err == nil {
		t.Error("pi unwiring from a non-array skills should fail")
	}
}
