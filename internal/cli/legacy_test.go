// The one-time legacy cleanup: on sync, fleet removes the collection paths
// and custom off-entries the old wiring model left behind, reports each by
// name, leaves everything outside its scope alone, and is silent on the
// second run.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeHarnessConfig writes a harness config file, creating its directory.
func writeHarnessConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSyncRemovesLegacyOpenCodeV1Entries(t *testing.T) {
	// Both V1 shapes at once: the fleet-wired collection path in
	// skills.paths and a fleet-shape custom deny in permission.skill. A
	// user's own source path and untracked deny must survive.
	p, collection := adoptHome(t)
	writeSkillDir(t, collection, "my-notes", "Personal notes.")
	writeHarnessConfig(t, p.OpenCodeConfig(), `{
  "skills": {"paths": ["`+collection+`", "~/.claude/skills"]},
  "permission": {"skill": {"my-notes": "deny", "manual": "deny"}}
}
`)

	out, _, err := runSync(t, p)
	if err != nil {
		t.Fatalf("fleet skill sync: %v", err)
	}

	got := readFile(t, p.OpenCodeConfig())
	if strings.Contains(got, collection) {
		t.Errorf("legacy collection path survived:\n%s", got)
	}
	if !strings.Contains(got, "~/.claude/skills") {
		t.Errorf("user source path was removed:\n%s", got)
	}
	if strings.Contains(got, `"my-notes"`) {
		t.Errorf("legacy custom deny survived:\n%s", got)
	}
	if !strings.Contains(got, `"manual": "deny"`) {
		t.Errorf("user's untracked deny was removed:\n%s", got)
	}

	for _, want := range []string{
		`sync: opencode: removed legacy collection path "` + collection + `"`,
		`sync: opencode: removed legacy disable entry "my-notes"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestSyncRemovesLegacyOpenCodeV2Entries(t *testing.T) {
	// The V2 flat skills array and fleet-shape permissions rules. A user
	// source entry and an untracked deny survive.
	p, collection := adoptHome(t)
	writeSkillDir(t, collection, "my-notes", "Personal notes.")
	writeHarnessConfig(t, p.OpenCodeConfig(), `{
  "skills": ["`+collection+`", "~/.claude/skills"],
  "permissions": [
    {"action": "skill", "resource": "my-notes", "effect": "deny"},
    {"action": "skill", "resource": "manual", "effect": "deny"}
  ]
}
`)

	out, _, err := runSync(t, p)
	if err != nil {
		t.Fatalf("fleet skill sync: %v", err)
	}

	got := readFile(t, p.OpenCodeConfig())
	if strings.Contains(got, collection) {
		t.Errorf("legacy collection path survived:\n%s", got)
	}
	if !strings.Contains(got, "~/.claude/skills") {
		t.Errorf("user source path was removed:\n%s", got)
	}
	if strings.Contains(got, `"resource": "my-notes"`) {
		t.Errorf("legacy custom deny survived:\n%s", got)
	}
	if !strings.Contains(got, `"resource": "manual"`) {
		t.Errorf("user's untracked deny was removed:\n%s", got)
	}

	for _, want := range []string{
		`sync: opencode: removed legacy collection path "` + collection + `"`,
		`sync: opencode: removed legacy disable entry "my-notes"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestSyncRemovesLegacyPiEntries(t *testing.T) {
	// The fleet-wired collection path and the -skills/<name>/SKILL.md
	// exact exclusion for a custom. A user source path and an untracked
	// exclusion survive.
	p, collection := adoptHome(t)
	writeSkillDir(t, collection, "my-notes", "Personal notes.")
	writeHarnessConfig(t, p.PiSettings(),
		`{"skills": ["`+collection+`", "~/.claude/skills", "-skills/my-notes/SKILL.md", "-skills/manual/SKILL.md"]}`)

	out, _, err := runSync(t, p)
	if err != nil {
		t.Fatalf("fleet skill sync: %v", err)
	}

	got := readFile(t, p.PiSettings())
	if strings.Contains(got, collection) {
		t.Errorf("legacy collection path survived:\n%s", got)
	}
	if !strings.Contains(got, "~/.claude/skills") {
		t.Errorf("user source path was removed:\n%s", got)
	}
	if strings.Contains(got, "-skills/my-notes/SKILL.md") {
		t.Errorf("legacy custom exclusion survived:\n%s", got)
	}
	if !strings.Contains(got, "-skills/manual/SKILL.md") {
		t.Errorf("user's untracked exclusion was removed:\n%s", got)
	}

	for _, want := range []string{
		`sync: pi: removed legacy collection path "` + collection + `"`,
		`sync: pi: removed legacy disable entry "my-notes"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestSyncRemovesLegacyCodexBlocks(t *testing.T) {
	// The fleet-shape [[skills.config]] off-entry for a custom goes; an
	// untracked one stays, and the rest of the file is untouched.
	p, collection := adoptHome(t)
	writeSkillDir(t, collection, "my-notes", "Personal notes.")
	writeHarnessConfig(t, p.CodexConfig(), `# codex
[[skills.config]]
name = "my-notes"
enabled = false

[[skills.config]]
name = "manual"
enabled = false
`)

	out, _, err := runSync(t, p)
	if err != nil {
		t.Fatalf("fleet skill sync: %v", err)
	}

	got := readFile(t, p.CodexConfig())
	if strings.Contains(got, `name = "my-notes"`) {
		t.Errorf("legacy custom block survived:\n%s", got)
	}
	if !strings.Contains(got, `name = "manual"`) {
		t.Errorf("user's untracked block was removed:\n%s", got)
	}
	if !strings.Contains(got, "# codex") {
		t.Errorf("the file's other content was touched:\n%s", got)
	}
	if !strings.Contains(out, `sync: codex: removed legacy disable entry "my-notes"`) {
		t.Errorf("output missing the codex removal:\n%s", out)
	}
}

func TestSyncLeavesUnrecognizedLegacyEntriesAlone(t *testing.T) {
	// Entries fleet cannot identify as its own are out of scope even when
	// they name a custom skill: a codex block with a path selector and an
	// extra key, and an opencode V2 rule with extra keys.
	p, collection := adoptHome(t)
	writeSkillDir(t, collection, "my-notes", "Personal notes.")
	writeHarnessConfig(t, p.CodexConfig(), `[[skills.config]]
name = "my-notes"
path = "/elsewhere/skills/my-notes/SKILL.md"
enabled = false
extra = true
`)
	writeHarnessConfig(t, p.OpenCodeConfig(),
		`{"permissions": [{"action": "skill", "resource": "my-notes", "effect": "deny", "note": "mine"}]}`)

	if _, _, err := runSync(t, p); err != nil {
		t.Fatalf("fleet skill sync: %v", err)
	}

	if got := readFile(t, p.CodexConfig()); !strings.Contains(got, `name = "my-notes"`) {
		t.Errorf("unrecognized codex block was removed:\n%s", got)
	}
	if got := readFile(t, p.OpenCodeConfig()); !strings.Contains(got, `"resource": "my-notes"`) {
		t.Errorf("unrecognized opencode rule was removed:\n%s", got)
	}
}

func TestSyncLegacyCleanupIsIdempotent(t *testing.T) {
	// Every legacy family at once, all fleet-owned. The first sync removes
	// and reports them; the second has nothing left to do.
	p, collection := adoptHome(t)
	writeSkillDir(t, collection, "my-notes", "Personal notes.")
	writeHarnessConfig(t, p.OpenCodeConfig(), `{"skills": {"paths": ["`+collection+`"]}}`)
	writeHarnessConfig(t, p.PiSettings(), `{"skills": ["`+collection+`"]}`)
	writeHarnessConfig(t, p.CodexConfig(), "[[skills.config]]\nname = \"my-notes\"\nenabled = false\n")

	out, _, err := runSync(t, p)
	if err != nil {
		t.Fatalf("first fleet skill sync: %v", err)
	}
	for _, want := range []string{
		`sync: opencode: removed legacy collection path "` + collection + `"`,
		`sync: pi: removed legacy collection path "` + collection + `"`,
		`sync: codex: removed legacy disable entry "my-notes"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("first sync output missing %q:\n%s", want, out)
		}
	}

	out, _, err = runSync(t, p)
	if err != nil {
		t.Fatalf("second fleet skill sync: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("second sync reported work it did not do:\n%s", out)
	}
}
