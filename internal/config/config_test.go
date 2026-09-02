package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
)

func TestMissingFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if f.SkillsRepo() != "" {
		t.Errorf("SkillsRepo() = %q, want empty", f.SkillsRepo())
	}
	if len(f.Unknown()) != 0 {
		t.Errorf("Unknown() = %v, want empty", f.Unknown())
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".config", "fleet", "config.json")
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	f, _ := Load(path)
	f.SetSkillsRepo(repo)
	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.SkillsRepo() != repo {
		t.Errorf("SkillsRepo() = %q, want %q", got.SkillsRepo(), repo)
	}
	body, _ := os.ReadFile(path)
	want := "{\n  \"skillsRepo\": \"" + repo + "\"\n}\n"
	if string(body) != want {
		t.Errorf("file =\n%s\nwant\n%s", body, want)
	}
}

func TestUnknownFieldsSurviveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	src := `{
  "skillsRepo": "/repo/one",
  "future": 123,
  "alpha": "x"
}`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	// mutate known field, preserve unknown
	f.SetSkillsRepo("/repo/two")
	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var out map[string]json.RawMessage
	body, _ := os.ReadFile(path)
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("round-tripped file is not JSON: %v", err)
	}
	if _, ok := out["future"]; !ok {
		t.Error("round-trip lost unknown key future")
	}
	if _, ok := out["alpha"]; !ok {
		t.Error("round-trip lost unknown key alpha")
	}
	if string(body) != "{\n  \"skillsRepo\": \"/repo/two\",\n  \"alpha\": \"x\",\n  \"future\": 123\n}\n" {
		t.Errorf("canonical ordering wrong:\n%s", body)
	}
	// unset should keep unknowns but remove skillsRepo
	f2, _ := Load(path)
	f2.UnsetSkillsRepo()
	if err := Save(path, f2); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(path)
	if strings.Contains(string(body), "skillsRepo") {
		t.Errorf("unset should remove skillsRepo, got %s", body)
	}
	if !strings.Contains(string(body), "\"alpha\"") || !strings.Contains(string(body), "\"future\"") {
		t.Errorf("unset lost unknown fields: %s", body)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	f, _ := Load(path)
	f.SetSkillsRepo("/repo")
	if err := Save(path, f); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file %s left behind", e.Name())
		}
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("saved file does not load: %v", err)
	}
}

func TestEffectiveRepoPrecedenceEnvOverFile(t *testing.T) {
	home := t.TempDir()
	p := paths.New(home)
	repoFile := filepath.Join(home, "repo-file")
	repoEnv := filepath.Join(home, "repo-env")
	for _, r := range []string{repoFile, repoEnv} {
		if err := os.MkdirAll(r, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f, _ := Load(p.FleetConfigFile())
	f.SetSkillsRepo(repoFile)
	if err := Save(p.FleetConfigFile(), f); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLEET_REPO", repoEnv)
	got, err := EffectiveRepo(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != repoEnv {
		t.Errorf("EffectiveRepo = %q, want env %q", got, repoEnv)
	}
	t.Setenv("FLEET_REPO", "")
	got, err = EffectiveRepo(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != repoFile {
		t.Errorf("EffectiveRepo = %q, want file %q", got, repoFile)
	}
	// missing file means empty
	if err := os.Remove(p.FleetConfigFile()); err != nil {
		t.Fatal(err)
	}
	got, err = EffectiveRepo(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("EffectiveRepo = %q, want empty when no file and no env", got)
	}
}

func TestLoadRejectsNonStringSkillsRepo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"skillsRepo": 123}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("Load should fail when skillsRepo is not a string")
	}
}

func TestEmptyFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if f.SkillsRepo() != "" {
		t.Errorf("empty file should be empty config")
	}
}
