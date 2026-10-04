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
	if len(f.SkillsDirs()) != 0 {
		t.Errorf("SkillsDirs() = %q, want empty", f.SkillsDirs())
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
	collection := filepath.Join(dir, "collection")
	f, _ := Load(path)
	f.SetSkillsDirs([]string{collection})
	f.SetAdoptTarget(filepath.Join(dir, "target"))
	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.SkillsDirs()) != 1 || got.SkillsDirs()[0] != collection {
		t.Errorf("SkillsDirs() = %q, want %q", got.SkillsDirs(), []string{collection})
	}
	if got.AdoptTarget() != filepath.Join(dir, "target") {
		t.Errorf("AdoptTarget() = %q, want %q", got.AdoptTarget(), filepath.Join(dir, "target"))
	}
	body, _ := os.ReadFile(path)
	want := "{\n  \"skillsDirs\": [\n    \"" + collection + "\"\n  ],\n  \"adoptTarget\": \"" + filepath.Join(dir, "target") + "\"\n}\n"
	if string(body) != want {
		t.Errorf("file =\n%s\nwant\n%s", body, want)
	}
}

func TestUnknownFieldsSurviveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	src := `{
  "skillsDirs": ["/dirs/one"],
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
	f.SetSkillsDirs([]string{"/dirs/two"})
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
	if string(body) != "{\n  \"skillsDirs\": [\n    \"/dirs/two\"\n  ],\n  \"alpha\": \"x\",\n  \"future\": 123\n}\n" {
		t.Errorf("canonical ordering wrong:\n%s", body)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	f, _ := Load(path)
	f.SetSkillsDirs([]string{"/dirs/one"})
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

// TestSupersededSinglePointerKeyBehavesAsUnset is the one place the older
// retired file key (skillsRepo) appears in fixtures: an old-key-only
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
	if len(f.SkillsDirs()) != 0 {
		t.Errorf("SkillsDirs() = %q, want empty (old key behaves as unset)", f.SkillsDirs())
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
	if len(f.SkillsDirs()) != 0 || f.AdoptTarget() != "" {
		t.Errorf("non-string old key behaves as set: dirs=%q target=%q", f.SkillsDirs(), f.AdoptTarget())
	}
	// The retired key is unknown to key normalization too.
	if got := NormalizeKey("skills-repo"); got != "" {
		t.Errorf("NormalizeKey(skills-repo) = %q, want unknown", got)
	}
	if got := NormalizeKey("skillsRepo"); got != "" {
		t.Errorf("NormalizeKey(skillsRepo) = %q, want unknown", got)
	}
}

// TestRetiredReposKeyPreservedAsUnknown is the same contract for the
// later-retired repo-root list: never read, preserved verbatim, and even a
// malformed value is not an error.
func TestRetiredReposKeyPreservedAsUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"skillsRepos": ["/repo/one", "/repo/two"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(f.SkillsDirs()) != 0 {
		t.Errorf("SkillsDirs() = %q, want empty (retired key behaves as unset)", f.SkillsDirs())
	}
	if _, ok := f.Unknown()["skillsRepos"]; !ok {
		t.Errorf("retired key should be preserved verbatim as unknown, got %v", f.Unknown())
	}
	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "{\n  \"skillsRepos\": [\"/repo/one\", \"/repo/two\"]\n}\n" {
		t.Errorf("retired key was migrated or dropped, want verbatim preservation:\n%s", body)
	}
	// Malformed retired values are ignored, not an error.
	if err := os.WriteFile(path, []byte(`{"skillsRepos": 123}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("Load() with malformed retired key error = %v (must never fail loading)", err)
	}
	if got := NormalizeKey("skills-repos"); got != "" {
		t.Errorf("NormalizeKey(skills-repos) = %q, want unknown", got)
	}
	if got := NormalizeKey("skillsRepos"); got != "" {
		t.Errorf("NormalizeKey(skillsRepos) = %q, want unknown", got)
	}
}

func TestNormalizeKeyAcceptsKebabAndCamelPairs(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"skills-repo", ""},
		{"skillsRepo", ""},
		{"skills-repos", ""},
		{"skillsRepos", ""},
		{"skills-dirs", "skills-dirs"},
		{"skillsDirs", "skills-dirs"},
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

func TestLoadRejectsBadKnownKeyTypes(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"skillsDirs not an array", `{"skillsDirs": "/dirs/one"}`},
		{"skillsDirs with non-string entry", `{"skillsDirs": ["/dirs/one", 123]}`},
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

func TestKnownKeysPreserveUnknownsAndUnset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	src := `{
  "skillsDirs": ["/dirs/a"],
  "adoptTarget": "/dirs/one/skills",
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
	f.SetSkillsDirs([]string{"/dirs/b"})
	f.UnsetAdoptTarget()
	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	body, _ := os.ReadFile(path)
	want := "{\n  \"skillsDirs\": [\n    \"/dirs/b\"\n  ],\n  \"future\": 123,\n  \"legacy\": \"kept\"\n}\n"
	if string(body) != want {
		t.Errorf("file =\n%s\nwant\n%s", body, want)
	}
	// clearing the list omits the key but keeps unknowns
	f2, _ := Load(path)
	f2.UnsetSkillsDirs()
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
	if len(f.SkillsDirs()) != 0 || f.AdoptTarget() != "" {
		t.Errorf("empty file should be empty config")
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSkillsDirsRoundTripPreservesOrderAndUnknowns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	src := `{
  "skillsRepos": ["/repo/legacy"],
  "future": 123
}`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(f.SkillsDirs()) != 0 {
		t.Fatalf("SkillsDirs() = %q, want empty for an absent key", f.SkillsDirs())
	}
	f.SetSkillsDirs([]string{"/dirs/b", "/dirs/a"})
	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if want := []string{"/dirs/b", "/dirs/a"}; !sameStrings(got.SkillsDirs(), want) {
		t.Errorf("SkillsDirs() = %q, want order-preserved %q", got.SkillsDirs(), want)
	}
	// The retired repo-root list is preserved verbatim as an unknown field.
	if _, ok := got.Unknown()["skillsRepos"]; !ok {
		t.Errorf("retired skillsRepos lost: %v", got.Unknown())
	}
	if _, ok := got.Unknown()["future"]; !ok {
		t.Errorf("round-trip lost unknown key future: %v", got.Unknown())
	}
	body, _ := os.ReadFile(path)
	want := "{\n  \"skillsDirs\": [\n    \"/dirs/b\",\n    \"/dirs/a\"\n  ],\n  \"future\": 123,\n  \"skillsRepos\": [\"/repo/legacy\"]\n}\n"
	if string(body) != want {
		t.Errorf("file =\n%s\nwant\n%s", body, want)
	}
}

func TestSkillsDirsEmptyMeansNoneAndUnsetOmitsKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	f, _ := Load(path)
	f.SetSkillsDirs(nil)
	if len(f.SkillsDirs()) != 0 {
		t.Errorf("SkillsDirs() = %q, want empty", f.SkillsDirs())
	}
	f.SetSkillsDirs([]string{"/dirs/one"})
	f.UnsetSkillsDirs()
	if len(f.SkillsDirs()) != 0 {
		t.Errorf("UnsetSkillsDirs() left %q", f.SkillsDirs())
	}
	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	body, _ := os.ReadFile(path)
	if strings.Contains(string(body), "skillsDirs") {
		t.Errorf("empty skillsDirs should not render the key:\n%s", body)
	}
}

func TestLoadRejectsMalformedSkillsDirs(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"not an array", `{"skillsDirs": "/dirs/one"}`},
		{"non-string entry", `{"skillsDirs": ["/dirs/one", 123]}`},
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

func TestNormalizeKeyIncludesSkillsDirs(t *testing.T) {
	if got := NormalizeKey("skills-dirs"); got != "skills-dirs" {
		t.Errorf("NormalizeKey(skills-dirs) = %q, want skills-dirs", got)
	}
	if got := NormalizeKey("skillsDirs"); got != "skills-dirs" {
		t.Errorf("NormalizeKey(skillsDirs) = %q, want skills-dirs", got)
	}
}
