package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
)

// runOpenCodeProject writes fixture (when non-empty), runs Project, and
// returns the report plus the file's exact content afterwards.
func runOpenCodeProject(t *testing.T, home, fixture string, writes []SkillWrite) (WriteReport, string) {
	t.Helper()
	p := paths.New(home)
	if fixture != "" {
		if err := os.MkdirAll(p.OpenCodeDir(), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p.OpenCodeConfig(), []byte(fixture), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := NewOpenCode(p).Project(writes)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	body, err := os.ReadFile(p.OpenCodeConfig())
	if err != nil {
		t.Fatal(err)
	}
	return rep, string(body)
}

func TestOpenCodeProjectV1GoldenDenyAppended(t *testing.T) {
	fixture := `{
		"$schema": "https://opencode.ai/config.json", // pin the schema
		"model": "gpt-5",
		"permission": {
			// skills policy
			"skill": {
				"*": "allow",
				"git-*": "deny",
			},
		},
	}`
	want := `{
		"$schema": "https://opencode.ai/config.json", // pin the schema
		"model": "gpt-5",
		"permission": {
			// skills policy
			"skill": {
				"*": "allow",
				"git-*": "deny",
				"tdd": "deny",
			},
		},
	}`
	rep, got := runOpenCodeProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config =\n%s\nwant\n%s", got, want)
	}
	wantChanges := []Change{{Skill: "tdd", From: StateOn, To: StateOff}}
	if !reflect.DeepEqual(rep.Changed, wantChanges) {
		t.Errorf("Changed = %v, want %v", rep.Changed, wantChanges)
	}
	// The user's git-* pattern deny covers no managed skill: foreign, and
	// flagged every run until the user or a later ticket resolves it.
	wantFlags := []Flag{{Message: `permission.skill rule "git-*" (effect deny) is not fleet's — left alone`}}
	if !reflect.DeepEqual(rep.Flags, wantFlags) {
		t.Errorf("Flags = %v, want %v", rep.Flags, wantFlags)
	}
}

func TestOpenCodeProjectV1ReplacesExactEntry(t *testing.T) {
	// An exact allow entry is part of the pair fleet manages: it goes away
	// and the deny is appended last so last-match-wins resolves to it.
	fixture := `{"permission": {"skill": {"tdd": "allow", "git-*": "deny"}}}`
	want := `{"permission": {"skill": { "git-*": "deny", "tdd": "deny"}}}`

	rep, got := runOpenCodeProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config = %s, want %s", got, want)
	}
	if len(rep.Changed) != 1 || rep.Changed[0].To != StateOff {
		t.Errorf("Changed = %v, want one flip to off", rep.Changed)
	}
}

func TestOpenCodeProjectV1ConvertsShorthand(t *testing.T) {
	fixture := `{"permission": {"skill": "ask"}}`
	want := `{"permission": {"skill": {"*": "ask", "tdd": "deny"}}}`

	rep, got := runOpenCodeProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config = %s, want %s", got, want)
	}
	if len(rep.Changed) != 1 || rep.Changed[0].From != StateOn || rep.Changed[0].To != StateOff {
		t.Errorf("Changed = %v, want one on->off flip", rep.Changed)
	}
}

func TestOpenCodeProjectCreatesMissingFileV1(t *testing.T) {
	want := `{"permission": {"skill": {"tdd": "deny"}}}` + "\n"

	rep, got := runOpenCodeProject(t, filepath.Join(t.TempDir(), "home"), "",
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config = %q, want %q", got, want)
	}
	if len(rep.Changed) != 1 || rep.Changed[0].To != StateOff {
		t.Errorf("Changed = %v, want one flip to off", rep.Changed)
	}
}

func TestOpenCodeProjectMarkerFreeFileDefaultsToV1(t *testing.T) {
	// V1 is the safe default for files without V2 markers: V2 auto-migrates
	// V1 keys, but V1 silently drops V2-only keys.
	fixture := `{"model": "gpt-5"}`
	want := `{"model": "gpt-5", "permission": {"skill": {"tdd": "deny"}}}`

	_, got := runOpenCodeProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config = %s, want %s", got, want)
	}
}

func TestOpenCodeProjectSkillsObjectOnlyIsWrittenAsV2(t *testing.T) {
	// The dialect-detection guard: a file whose only marker is the
	// V1-shaped skills object must receive V2 rules, which its decoder
	// honors; V1 permission there would be silently skipped.
	fixture := `{"skills": {"paths": ["./team-skills"], "urls": []}}`
	want := `{"skills": {"paths": ["./team-skills"], "urls": []}, "permissions": [{"action": "skill", "resource": "tdd", "effect": "deny"}]}`

	rep, got := runOpenCodeProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config = %s, want %s", got, want)
	}
	if len(rep.Changed) != 1 || rep.Changed[0].To != StateOff {
		t.Errorf("Changed = %v, want one flip to off", rep.Changed)
	}
}

func TestOpenCodeProjectV2GoldenDenyAppended(t *testing.T) {
	fixture := `{
  "$schema": "https://opencode.ai/config.json",
  // v2 beta rules
  "permissions": [
    { "action": "skill", "resource": "*", "effect": "allow" },
    { "action": "bash", "resource": "rm -rf *", "effect": "deny" },
  ],
}`
	want := `{
  "$schema": "https://opencode.ai/config.json",
  // v2 beta rules
  "permissions": [
    { "action": "skill", "resource": "*", "effect": "allow" },
    { "action": "bash", "resource": "rm -rf *", "effect": "deny" },
    {"action": "skill", "resource": "tdd", "effect": "deny"},
  ],
}`

	rep, got := runOpenCodeProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config =\n%s\nwant\n%s", got, want)
	}
	if len(rep.Changed) != 1 || rep.Changed[0].To != StateOff {
		t.Errorf("Changed = %v, want one flip to off", rep.Changed)
	}
}

func TestOpenCodeProjectV2EnableRemovesFleetRuleOnly(t *testing.T) {
	fixture := `{
  "permissions": [
    { "action": "skill", "resource": "*", "effect": "allow" },
    { "action": "skill", "resource": "tdd", "effect": "deny" }, // fleet
    { "action": "skill", "resource": "internal-*", "effect": "deny" },
  ],
}`
	want := `{
  "permissions": [
    { "action": "skill", "resource": "*", "effect": "allow" }, // fleet
    { "action": "skill", "resource": "internal-*", "effect": "deny" },
  ],
}`

	rep, got := runOpenCodeProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOn}})
	if got != want {
		t.Errorf("config =\n%s\nwant\n%s", got, want)
	}
	wantChanges := []Change{{Skill: "tdd", From: StateOff, To: StateOn}}
	if !reflect.DeepEqual(rep.Changed, wantChanges) {
		t.Errorf("Changed = %v, want %v", rep.Changed, wantChanges)
	}
	// The internal-* pattern deny affects no managed skill; it is foreign
	// and gets flagged.
	wantFlags := []Flag{{Message: `permissions rule for "internal-*" (action "skill", effect deny) is not fleet's — left alone`}}
	if !reflect.DeepEqual(rep.Flags, wantFlags) {
		t.Errorf("Flags = %v, want %v", rep.Flags, wantFlags)
	}
}

func TestOpenCodeProjectV2EnableFlagsPatternStillDenying(t *testing.T) {
	fixture := `{
  "permissions": [
    { "action": "skill", "resource": "tdd", "effect": "deny" },
    { "action": "skill", "resource": "t*", "effect": "deny" },
  ],
}`
	want := `{
  "permissions": [
    { "action": "skill", "resource": "t*", "effect": "deny" },
  ],
}`

	rep, got := runOpenCodeProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOn}})
	if got != want {
		t.Errorf("config =\n%s\nwant\n%s", got, want)
	}
	if len(rep.Changed) != 0 {
		t.Errorf("Changed = %v, want none (the pattern still denies)", rep.Changed)
	}
	wantFlags := []Flag{{Skill: "tdd", Message: `still disabled by a permissions rule for "t*" — left alone`}}
	if !reflect.DeepEqual(rep.Flags, wantFlags) {
		t.Errorf("Flags = %v, want %v", rep.Flags, wantFlags)
	}
}

func TestOpenCodeProjectSatisfiedOffQuietWhenFleetsOwn(t *testing.T) {
	// State says off, config already denies exactly: nothing to do, no
	// noise.
	fixture := `{"permissions": [{"action": "skill", "resource": "tdd", "effect": "deny"}]}`
	rep, got := runOpenCodeProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != fixture {
		t.Errorf("config =\n%s\nwant untouched\n%s", got, fixture)
	}
	if !rep.Empty() {
		t.Errorf("report = %+v, want empty", rep)
	}
}

func TestOpenCodeProjectManualEditDriftIsFlaggedNotTouched(t *testing.T) {
	// The manual-edit drift scenario: denies for skills fleet isn't
	// tracking stay exactly as written and are flagged. The managed skill
	// itself still gets its deny (git-* doesn't match tdd, so the read
	// state was on).
	fixture := `{
  "permission": {
    "skill": {
      "git-*": "deny",
      "manual-skill": "deny",
    },
  },
}`
	want := `{
  "permission": {
    "skill": {
      "git-*": "deny",
      "manual-skill": "deny",
      "tdd": "deny",
    },
  },
}`
	rep, got := runOpenCodeProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != want {
		t.Errorf("config =\n%s\nwant\n%s", got, want)
	}
	wantFlags := []Flag{
		{Message: `permission.skill rule "git-*" (effect deny) is not fleet's — left alone`},
		{Skill: "manual-skill", Message: "disabled in config but not tracked by fleet's state — left alone"},
	}
	if !reflect.DeepEqual(rep.Flags, wantFlags) {
		t.Errorf("Flags =\n%v\nwant\n%v", rep.Flags, wantFlags)
	}
	if len(rep.Changed) != 1 || rep.Changed[0].To != StateOff {
		t.Errorf("Changed = %v, want the tdd deny flip", rep.Changed)
	}
}

func TestOpenCodeProjectManualEditDriftNoWritesStillFlags(t *testing.T) {
	// Ambient sync with an empty state file must flag manual denies and
	// touch nothing.
	fixture := `{"permission": {"skill": {"manual-skill": "deny"}}}`
	rep, got := runOpenCodeProject(t, filepath.Join(t.TempDir(), "home"), fixture, nil)
	if got != fixture {
		t.Errorf("config =\n%s\nwant untouched\n%s", got, fixture)
	}
	wantFlags := []Flag{{Skill: "manual-skill", Message: "disabled in config but not tracked by fleet's state — left alone"}}
	if !reflect.DeepEqual(rep.Flags, wantFlags) {
		t.Errorf("Flags = %v, want %v", rep.Flags, wantFlags)
	}
}

func TestOpenCodeProjectV2BlanketDenyFlaggedAndRespected(t *testing.T) {
	// A wildcard-action deny covers every skill: the off write is
	// satisfied by it and the rule is flagged as foreign.
	fixture := `{"permissions": [{"action": "*", "resource": "*", "effect": "deny"}]}`
	rep, got := runOpenCodeProject(t, filepath.Join(t.TempDir(), "home"), fixture,
		[]SkillWrite{{Name: "tdd", State: StateOff}})
	if got != fixture {
		t.Errorf("config =\n%s\nwant untouched\n%s", got, fixture)
	}
	if len(rep.Changed) != 0 {
		t.Errorf("Changed = %v, want none", rep.Changed)
	}
	wantFlags := []Flag{{Skill: "tdd", Message: `still disabled by a permissions rule for "*" — left alone`}}
	if !reflect.DeepEqual(rep.Flags, wantFlags) {
		t.Errorf("Flags = %v, want %v", rep.Flags, wantFlags)
	}
}

func TestOpenCodeProjectOnWhenNothingToEnable(t *testing.T) {
	// Enabling a skill that isn't disabled: no changes, no writes.
	rep, err := NewOpenCode(paths.New(filepath.Join(t.TempDir(), "home"))).
		Project([]SkillWrite{{Name: "tdd", State: StateOn}})
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if !rep.Empty() {
		t.Errorf("report = %+v, want empty", rep)
	}
	if _, err := os.Stat(paths.New(filepath.Join(t.TempDir(), "home")).OpenCodeConfig()); !os.IsNotExist(err) {
		t.Errorf("config file created by an enable no-op: %v", err)
	}
}

func TestOpenCodeProjectPreservesModeOnRewrite(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)
	if err := os.MkdirAll(p.OpenCodeDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.OpenCodeConfig(), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewOpenCode(p).Project([]SkillWrite{{Name: "tdd", State: StateOff}}); err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	info, err := os.Stat(p.OpenCodeConfig())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want %v (existing mode preserved)", info.Mode().Perm(), os.FileMode(0o600))
	}
}

func TestOpenCodeProjectMalformedConfigIsAnError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if _, _, err := runOpenCodeProjectErr(t, home, `{"permission": {`); err == nil {
		t.Fatal("Project() on malformed config should fail")
	}
}

func runOpenCodeProjectErr(t *testing.T, home, fixture string) (WriteReport, string, error) {
	t.Helper()
	p := paths.New(home)
	if err := os.MkdirAll(p.OpenCodeDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := NewOpenCode(p).Project([]SkillWrite{{Name: "tdd", State: StateOff}})
	return rep, "", err
}
