package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(f.SkillsRepos()) != 0 {
		t.Errorf("SkillsRepos() = %q, want empty", f.SkillsRepos())
	}
	if f.AdoptTarget() != "" {
		t.Errorf("AdoptTarget() = %q, want empty", f.AdoptTarget())
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
	f.SetSkillsRepos([]string{repo})
	f.SetAdoptTarget(filepath.Join(repo, "skills"))
	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.SkillsRepos()) != 1 || got.SkillsRepos()[0] != repo {
		t.Errorf("SkillsRepos() = %q, want %q", got.SkillsRepos(), []string{repo})
	}
	if got.AdoptTarget() != filepath.Join(repo, "skills") {
		t.Errorf("AdoptTarget() = %q, want %q", got.AdoptTarget(), filepath.Join(repo, "skills"))
	}
	body, _ := os.ReadFile(path)
	want := "{\n  \"skillsRepos\": [\n    \"" + repo + "\"\n  ],\n  \"adoptTarget\": \"" + filepath.Join(repo, "skills") + "\"\n}\n"
	if string(body) != want {
		t.Errorf("file =\n%s\nwant\n%s", body, want)
	}
}

func TestUnknownFieldsSurviveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	src := `{
  "skillsRepos": ["/repo/one"],
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
	f.SetSkillsRepos([]string{"/repo/two"})
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
	if string(body) != "{\n  \"skillsRepos\": [\n    \"/repo/two\"\n  ],\n  \"alpha\": \"x\",\n  \"future\": 123\n}\n" {
		t.Errorf("canonical ordering wrong:\n%s", body)
	}
	// unset should keep unknowns but remove skillsRepos
	f2, _ := Load(path)
	f2.UnsetSkillsRepos()
	if err := Save(path, f2); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(path)
	if strings.Contains(string(body), "skillsRepos") {
		t.Errorf("unset should remove skillsRepos, got %s", body)
	}
	if !strings.Contains(string(body), "\"alpha\"") || !strings.Contains(string(body), "\"future\"") {
		t.Errorf("unset lost unknown fields: %s", body)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	f, _ := Load(path)
	f.SetSkillsRepos([]string{"/repo"})
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

// TestSupersededSinglePointerKeyBehavesAsUnset is the one place the
// retired file key (skillsRepo) still appears in fixtures: an old-key-only
// config loads without error, behaves as unset (empty list, empty target),
// and round-trips the key verbatim — preserved, never migrated, never
// interpreted. No command reads it.
func TestSupersededSinglePointerKeyBehavesAsUnset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"skillsRepo": "/repo/one"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(f.SkillsRepos()) != 0 {
		t.Errorf("SkillsRepos() = %q, want empty (old key behaves as unset)", f.SkillsRepos())
	}
	if f.AdoptTarget() != "" {
		t.Errorf("AdoptTarget() = %q, want empty (old key behaves as unset)", f.AdoptTarget())
	}
	if _, ok := f.Unknown()["skillsRepo"]; !ok {
		t.Errorf("old key should be preserved verbatim as unknown, got %v", f.Unknown())
	}
	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "{\n  \"skillsRepo\": \"/repo/one\"\n}\n" {
		t.Errorf("old key was migrated or dropped, want verbatim preservation:\n%s", body)
	}
	// Even a malformed old value is ignored, not an error: the key is
	// never interpreted.
	if err := os.WriteFile(path, []byte(`{"skillsRepo": 123}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err = Load(path)
	if err != nil {
		t.Fatalf("Load() with non-string old key error = %v (old key must never fail loading)", err)
	}
	if len(f.SkillsRepos()) != 0 || f.AdoptTarget() != "" {
		t.Errorf("non-string old key behaves as set: repos=%q target=%q", f.SkillsRepos(), f.AdoptTarget())
	}
	// The retired key is unknown to key normalization too.
	if got := NormalizeKey("skills-repo"); got != "" {
		t.Errorf("NormalizeKey(skills-repo) = %q, want unknown", got)
	}
	if got := NormalizeKey("skillsRepo"); got != "" {
		t.Errorf("NormalizeKey(skillsRepo) = %q, want unknown", got)
	}
}

func TestNormalizeKeyAcceptsKebabAndCamelPairs(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"skills-repo", ""},
		{"skillsRepo", ""},
		{"skills-repos", "skills-repos"},
		{"skillsRepos", "skills-repos"},
		{"adopt-target", "adopt-target"},
		{"adoptTarget", "adopt-target"},
		{"nope", ""},
		{"", ""},
		{"skills_repo", ""},
	}
	for _, c := range cases {
		if got := NormalizeKey(c.in); got != c.want {
			t.Errorf("NormalizeKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExpandPathAndAbsoluteValidation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := ExpandPath("~"); got != home {
		t.Errorf("ExpandPath(~) = %q, want %q", got, home)
	}
	if got := ExpandPath("~/repo"); got != filepath.Join(home, "repo") {
		t.Errorf("ExpandPath(~/repo) = %q, want %q", got, filepath.Join(home, "repo"))
	}
	if got := ExpandPath("/abs/path"); got != "/abs/path" {
		t.Errorf("ExpandPath(/abs/path) = %q, want unchanged", got)
	}
	if got := ExpandPath("relative/path"); got != "relative/path" {
		t.Errorf("ExpandPath(relative/path) = %q, want unchanged", got)
	}
	got, err := AbsolutePath("~/repo")
	if err != nil {
		t.Fatalf("AbsolutePath(~/repo) error = %v", err)
	}
	if got != filepath.Join(home, "repo") {
		t.Errorf("AbsolutePath(~/repo) = %q, want %q", got, filepath.Join(home, "repo"))
	}
	if _, err := AbsolutePath("relative/path"); err == nil {
		t.Error("AbsolutePath(relative/path) should fail")
	}
	got, err = AbsolutePath(filepath.Join(home, "a", "..", "b"))
	if err != nil {
		t.Fatalf("AbsolutePath with dotdot error = %v", err)
	}
	if got != filepath.Join(home, "b") {
		t.Errorf("AbsolutePath should clean, = %q want %q", got, filepath.Join(home, "b"))
	}
}

func TestLoadRejectsBadNewKeyTypes(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"skillsRepos not an array", `{"skillsRepos": "/repo/one"}`},
		{"skillsRepos with non-string entry", `{"skillsRepos": ["/repo/one", 123]}`},
		{"adoptTarget not a string", `{"adoptTarget": 123}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(c.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Errorf("Load(%s) should fail", c.body)
			}
		})
	}
}

func TestNewKeysPreserveUnknownsAndUnset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	src := `{
  "skillsRepos": ["/repo/a"],
  "adoptTarget": "/repo/one/skills",
  "future": 123,
  "legacy": "kept"
}`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	f.SetSkillsRepos([]string{"/repo/b"})
	f.UnsetAdoptTarget()
	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	body, _ := os.ReadFile(path)
	want := "{\n  \"skillsRepos\": [\n    \"/repo/b\"\n  ],\n  \"future\": 123,\n  \"legacy\": \"kept\"\n}\n"
	if string(body) != want {
		t.Errorf("file =\n%s\nwant\n%s", body, want)
	}
	// clearing the list omits the key but keeps unknowns
	f2, _ := Load(path)
	f2.UnsetSkillsRepos()
	if err := Save(path, f2); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(path)
	if string(body) != "{\n  \"future\": 123,\n  \"legacy\": \"kept\"\n}\n" {
		t.Errorf("cleared file =\n%s\nwant unknowns only", body)
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
	if len(f.SkillsRepos()) != 0 || f.AdoptTarget() != "" {
		t.Errorf("empty file should be empty config")
	}
}

func TestSkillsReposRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	f, _ := Load(path)
	f.SetSkillsRepos([]string{"/repo/b", "/repo/a"})
	f.SetAdoptTarget("/repo/one/skills")
	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	wantList := []string{"/repo/b", "/repo/a"}
	if len(got.SkillsRepos()) != 2 || got.SkillsRepos()[0] != wantList[0] || got.SkillsRepos()[1] != wantList[1] {
		t.Errorf("SkillsRepos() = %q, want order-preserved %q", got.SkillsRepos(), wantList)
	}
	if got.AdoptTarget() != "/repo/one/skills" {
		t.Errorf("AdoptTarget() = %q, want %q", got.AdoptTarget(), "/repo/one/skills")
	}
	body, _ := os.ReadFile(path)
	want := "{\n  \"skillsRepos\": [\n    \"/repo/b\",\n    \"/repo/a\"\n  ],\n  \"adoptTarget\": \"/repo/one/skills\"\n}\n"
	if string(body) != want {
		t.Errorf("file =\n%s\nwant\n%s", body, want)
	}
}
