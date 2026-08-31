package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMissingFileIsEmptyState(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(f.Disabled("opencode")) != 0 {
		t.Errorf("Disabled() = %v, want empty", f.Disabled("opencode"))
	}
	if f.IsDisabled("tdd", "opencode") {
		t.Error("IsDisabled = true, want false")
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "fleet", "state.json")
	f, _ := Load(path)
	f.SetDisabled("tdd", "opencode")
	f.SetDisabled("tdd", "pi")
	f.SetDisabled("git-helper", "codex")

	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	wantOpenCode := []string{"tdd"}
	if !reflect.DeepEqual(got.Disabled("opencode"), wantOpenCode) {
		t.Errorf("Disabled(opencode) = %v, want %v", got.Disabled("opencode"), wantOpenCode)
	}
	wantPi := []string{"tdd"}
	if !reflect.DeepEqual(got.Disabled("pi"), wantPi) {
		t.Errorf("Disabled(pi) = %v, want %v", got.Disabled("pi"), wantPi)
	}
	if got.IsDisabled("tdd", "claude") {
		t.Error("tdd/claude disabled, want on (not toggled)")
	}
}

func TestSaveCreatesParentDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "config", "fleet", "state.json")
	f, _ := Load(path)
	f.SetDisabled("tdd", "bob")
	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file not written: %v", err)
	}
}

func TestUnknownFieldsSurviveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	src := `{
  "version": 1,
  "skills": {
    "tdd": {
      "harnesses": {
        "opencode": "off"
      },
      "sourceRepo": "github.com/example/repo"
    },
    "future": {
      "harnesses": {
        "opencode": {"paused": true}
      }
    }
  },
  "migratedFrom": "skillctl",
  "flags": [1, 2]
}`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	f.SetEnabled("tdd", "opencode") // toggling must not disturb the rest
	if err := Save(path, f); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var out map[string]json.RawMessage
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("round-tripped file is not JSON: %v", err)
	}
	for _, key := range []string{"version", "migratedFrom", "flags", "skills"} {
		if _, ok := out[key]; !ok {
			t.Errorf("round-trip lost top-level key %q", key)
		}
	}

	var skills map[string]json.RawMessage
	if err := json.Unmarshal(out["skills"], &skills); err != nil {
		t.Fatal(err)
	}
	var tdd struct {
		Harnesses  map[string]json.RawMessage `json:"harnesses"`
		SourceRepo string                     `json:"sourceRepo"`
	}
	if err := json.Unmarshal(skills["tdd"], &tdd); err != nil {
		t.Fatal(err)
	}
	if tdd.SourceRepo != "github.com/example/repo" {
		t.Errorf("round-trip lost skills.tdd.sourceRepo = %q", tdd.SourceRepo)
	}
	if _, stillOff := tdd.Harnesses["opencode"]; stillOff {
		t.Error("skills.tdd.harnesses.opencode still \"off\" after SetEnabled")
	}
	var future struct {
		Harnesses map[string]json.RawMessage `json:"harnesses"`
	}
	if err := json.Unmarshal(skills["future"], &future); err != nil {
		t.Fatal(err)
	}
	var paused struct {
		Paused bool `json:"paused"`
	}
	if err := json.Unmarshal(future.Harnesses["opencode"], &paused); err != nil || !paused.Paused {
		t.Errorf("unknown harness value not preserved verbatim: %s (%v)", future.Harnesses["opencode"], err)
	}
}

func TestDisabledOnlyCountsOffValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	src := `{"version": 1, "skills": {"tdd": {"harnesses": {"opencode": "off", "pi": {"paused": true}}}}}`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if want := []string{"tdd"}; !reflect.DeepEqual(f.Disabled("opencode"), want) {
		t.Errorf("Disabled(opencode) = %v, want %v", f.Disabled("opencode"), want)
	}
	if len(f.Disabled("pi")) != 0 {
		t.Errorf("Disabled(pi) = %v, want empty (unknown value is not fleet's)", f.Disabled("pi"))
	}
	if f.IsDisabled("tdd", "pi") {
		t.Error("IsDisabled(tdd, pi) = true for an unknown value, want false")
	}
}

func TestSetEnabledLeavesUnknownHarnessValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	src := `{"version": 1, "skills": {"tdd": {"harnesses": {"pi": {"paused": true}}}}}`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	f.SetEnabled("tdd", "pi")

	var out struct {
		Skills map[string]struct {
			Harnesses map[string]json.RawMessage `json:"harnesses"`
		} `json:"skills"`
	}
	if err := Save(path, f); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out.Skills["tdd"]; !ok {
		t.Fatal("SetEnabled dropped a skill entry holding unknown values")
	}
	if _, ok := out.Skills["tdd"].Harnesses["pi"]; !ok {
		t.Error("SetEnabled removed an unknown harness value")
	}
}

func TestEmptyStateRendersMinimalSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	f, _ := Load(path)
	if err := Save(path, f); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	want := "{\n  \"version\": 1,\n  \"skills\": {}\n}\n"
	if got := string(body); got != want {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
}

func TestDisabledPairRendersSorted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	f, _ := Load(path)
	f.SetDisabled("tdd", "pi")
	f.SetDisabled("tdd", "opencode")
	f.SetDisabled("a-skill", "opencode")
	if err := Save(path, f); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	want := `{
  "version": 1,
  "skills": {
    "a-skill": {
      "harnesses": {
        "opencode": "off"
      }
    },
    "tdd": {
      "harnesses": {
        "opencode": "off",
        "pi": "off"
      }
    }
  }
}
`
	if got := string(body); got != want {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
}

func TestLoadRejectsForeignOrNewerFiles(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"no version", `{"skills": {}}`},
		{"newer version", `{"version": 2}`},
		{"version not a number", `{"version": "one"}`},
		{"not an object", `[1]`},
		{"skills not an object", `{"version": 1, "skills": []}`},
		{"skill entry not an object", `{"version": 1, "skills": {"tdd": "off"}}`},
		{"harnesses not an object", `{"version": 1, "skills": {"tdd": {"harnesses": "off"}}}`},
		{"broken json", `{"version":`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(path, []byte(c.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Errorf("Load(%s) succeeded, want error", c.body)
			}
		})
	}
}

func TestVersionOneFileWithNoSkillsLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"version": 1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(f.skills) != 0 {
		t.Errorf("skills = %v, want empty", f.skills)
	}
}

func TestSaveIsAtomicEnoughForCrashes(t *testing.T) {
	// Save writes to a temp file and renames; the temp file must be gone
	// and the target complete afterwards.
	path := filepath.Join(t.TempDir(), "state.json")
	f, _ := Load(path)
	f.SetDisabled("tdd", "claude")
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
