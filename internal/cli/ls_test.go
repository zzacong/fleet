package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zacong/fleet/internal/outdated"
	"github.com/zacong/fleet/internal/paths"
)

// fakeHome builds a home directory with a realistic mix: an installed
// skill, another installed skill from a second source, and a custom skill;
// opencode denying the git helpers (V2), pi excluding tdd, codex disabling
// deploy-vercel, Bob installed with a link, and no claude or cursor.
func fakeHome(t *testing.T) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	p := paths.New(home)

	store := p.SkillsStore()
	writeSkillDir(t, store, "tdd", "Red-green-refactor workflow for strict TDD.")
	writeSkillDir(t, store, "git-helper", "Wraps common git flows.")
	writeSkillDir(t, store, "deploy-vercel", "Deploy preview and production to Vercel.")
	writeSkillDir(t, store, "my-notes", "Personal note-taking conventions.")

	lock := `{"version": 3, "skills": {
		"tdd": {
			"source": "mattpocock/skills",
			"sourceType": "github",
			"skillFolderHash": "aaa111",
			"installedAt": "2026-08-08T04:30:28.403Z",
			"updatedAt": "2026-08-21T06:17:32.238Z"
		},
		"git-helper": {
			"source": "mattpocock/skills",
			"sourceType": "github",
			"skillFolderHash": "bbb222"
		},
		"deploy-vercel": {
			"source": "vercel/agent-skills",
			"sourceType": "github",
			"skillFolderHash": "ccc333"
		}
	}}`
	if err := os.WriteFile(p.SkillLock(), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{p.OpenCodeDir(), p.PiDir(), p.CodexDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	opencode := `{
		"$schema": "https://opencode.ai/config.json",
		"permissions": [
			{ "action": "skill", "resource": "git-*", "effect": "deny" },
		],
	}`
	if err := os.WriteFile(p.OpenCodeConfig(), []byte(opencode), 0o644); err != nil {
		t.Fatal(err)
	}

	pi := `{"skills": ["-skills/tdd/SKILL.md"]}`
	if err := os.MkdirAll(filepath.Dir(p.PiSettings()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.PiSettings(), []byte(pi), 0o644); err != nil {
		t.Fatal(err)
	}

	codex := `
[[skills.config]]
name = "deploy-vercel"
enabled = false
`
	if err := os.WriteFile(p.CodexConfig(), []byte(codex), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(p.BobSkills(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(store+"/tdd", filepath.Join(p.BobSkills(), "tdd")); err != nil {
		t.Fatal(err)
	}

	return p
}

func writeSkillDir(t *testing.T, store, dir, description string) {
	t.Helper()
	path := filepath.Join(store, dir, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + dir + "\ndescription: " + description + "\n---\n\n# " + dir + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runLs(t *testing.T, p *paths.Paths, args ...string) string {
	t.Helper()
	out, _, err := runLsCapture(t, p, &fakeTrees{}, args...)
	if err != nil {
		t.Fatalf("fleet skill ls %v: error = %v", args, err)
	}
	return out
}

// runLsCheck runs ls with a scripted tree client, returning stdout and
// stderr separately so tests can assert on warnings.
func runLsCapture(t *testing.T, p *paths.Paths, trees *fakeTrees, args ...string) (string, string, error) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	prev := newTreeClient
	newTreeClient = func(*paths.Paths) outdated.TreeClient { return trees }
	t.Cleanup(func() { newTreeClient = prev })
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"skill", "ls"}, args...))
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	err := root.Execute()
	return out.String(), errOut.String(), err
}

// fakeTrees is the stubbed GitHub API seam for the ls command. It records
// every call and answers from a scripted map keyed "owner/repo@ref"; an
// unscripted repo answers an empty tree.
type fakeTrees struct {
	calls []string
	trees map[string]outdated.TreeResponse
	errs  map[string]error
}

func (f *fakeTrees) FetchTree(_ context.Context, owner, repo, ref, _ string) (outdated.TreeResponse, error) {
	key := owner + "/" + repo + "@" + ref
	f.calls = append(f.calls, key)
	if err := f.errs[key]; err != nil {
		return outdated.TreeResponse{}, err
	}
	return f.trees[key], nil
}

// scriptTree answers owner/repo@ref with the given folder tree SHA.
func (f *fakeTrees) scriptTree(key string, entries ...outdated.TreeEntry) {
	if f.trees == nil {
		f.trees = map[string]outdated.TreeResponse{}
	}
	f.trees[key] = outdated.TreeResponse{Entries: entries}
}

// writeLockWithSkillPaths replaces fakeHome's lockfile with one whose
// GitHub entries record skillPath, the field the update check needs.
func writeLockWithSkillPaths(t *testing.T, p *paths.Paths) {
	t.Helper()
	lock := `{"version": 3, "skills": {
		"tdd": {
			"source": "mattpocock/skills",
			"sourceType": "github",
			"skillPath": "skills/engineering/tdd/SKILL.md",
			"skillFolderHash": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		"git-helper": {
			"source": "mattpocock/skills",
			"sourceType": "github",
			"skillPath": "skills/engineering/git-helper/SKILL.md",
			"skillFolderHash": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		},
		"deploy-vercel": {
			"source": "vercel/agent-skills",
			"sourceType": "github",
			"skillPath": "skills/deploy-vercel/SKILL.md",
			"skillFolderHash": "cccccccccccccccccccccccccccccccccccccccc"
		}
	}}`
	if err := os.WriteFile(p.SkillLock(), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLsTableShowsTheUpdateMarker(t *testing.T) {
	p := fakeHome(t)
	writeLockWithSkillPaths(t, p)
	trees := &fakeTrees{}
	// tdd is current, git-helper has moved upstream, deploy-vercel is
	// current; the custom skill my-notes is never checked.
	trees.scriptTree("mattpocock/skills@",
		outdated.TreeEntry{Path: "skills/engineering/tdd", Type: "tree", SHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		outdated.TreeEntry{Path: "skills/engineering/git-helper", Type: "tree", SHA: "dddddddddddddddddddddddddddddddddddddddd"},
	)
	trees.scriptTree("vercel/agent-skills@",
		outdated.TreeEntry{Path: "skills/deploy-vercel", Type: "tree", SHA: "cccccccccccccccccccccccccccccccccccccccc"},
	)

	out, _, _ := runLsCapture(t, p, trees)

	if !strings.Contains(out, "UPDATE") {
		t.Errorf("table missing the UPDATE column:\n%s", out)
	}
	if row := tableRow(t, out, "git-helper"); !strings.Contains(row, "↑") {
		t.Errorf("git-helper row should carry the update-available marker: %q", row)
	}
	if row := tableRow(t, out, "tdd"); !strings.Contains(row, "✓") {
		t.Errorf("tdd row should be marked current: %q", row)
	}
	if row := tableRow(t, out, "deploy-vercel"); !strings.Contains(row, "✓") {
		t.Errorf("deploy-vercel row should be marked current: %q", row)
	}
	if row := tableRow(t, out, "my-notes"); !strings.Contains(row, "?") {
		t.Errorf("custom skills are unknown, not checked: %q", row)
	}
	if len(trees.calls) != 2 {
		t.Errorf("two source repos, two API calls, calls = %v", trees.calls)
	}
}

func TestLsJSONCarriesTriStateOutdated(t *testing.T) {
	p := fakeHome(t)
	writeLockWithSkillPaths(t, p)
	trees := &fakeTrees{}
	trees.scriptTree("mattpocock/skills@",
		outdated.TreeEntry{Path: "skills/engineering/tdd", Type: "tree", SHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		outdated.TreeEntry{Path: "skills/engineering/git-helper", Type: "tree", SHA: "dddddddddddddddddddddddddddddddddddddddd"},
	)
	trees.scriptTree("vercel/agent-skills@",
		outdated.TreeEntry{Path: "skills/deploy-vercel", Type: "tree", SHA: "cccccccccccccccccccccccccccccccccccccccc"},
	)

	out, _, _ := runLsCapture(t, p, trees, "--json")

	var report struct {
		Skills []map[string]json.RawMessage `json:"skills"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}

	// The tri-state is structurally clean: true = outdated, false =
	// current, null = unknown (custom, non-GitHub, or failed check). The
	// field is present for every skill, never omitted.
	want := map[string]string{
		"my-notes":      "null",
		"git-helper":    "true",
		"tdd":           "false",
		"deploy-vercel": "false",
	}
	seen := map[string]bool{}
	for _, s := range report.Skills {
		var name string
		if err := json.Unmarshal(s["name"], &name); err != nil {
			t.Fatalf("skill without a name: %v", err)
		}
		raw, ok := s["outdated"]
		if !ok {
			t.Errorf("skill %s has no outdated field; every skill must carry the tri-state", name)
			continue
		}
		if got := strings.TrimSpace(string(raw)); got != want[name] {
			t.Errorf("%s outdated = %s, want %s", name, got, want[name])
		}
		seen[name] = true
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("skill %s missing from JSON", name)
		}
	}
}

func TestLsDegradesGracefullyWhenTheAPICannotBeReached(t *testing.T) {
	p := fakeHome(t)
	writeLockWithSkillPaths(t, p)
	trees := &fakeTrees{errs: map[string]error{
		"mattpocock/skills@":   context.DeadlineExceeded,
		"vercel/agent-skills@": context.DeadlineExceeded,
	}}

	out, errOut, err := runLsCapture(t, p, trees)

	// The failure is visible on stderr but never fatal: the command exits
	// 0 and the table still renders with every badge unknown.
	if err != nil {
		t.Fatalf("a failed update check must not fail the command: %v", err)
	}
	for _, repo := range []string{"mattpocock/skills", "vercel/agent-skills"} {
		if !strings.Contains(errOut, repo) {
			t.Errorf("stderr should name the failed repo %s:\n%s", repo, errOut)
		}
	}
	for _, name := range []string{"tdd", "git-helper", "deploy-vercel"} {
		if row := tableRow(t, out, name); !strings.Contains(row, "?") {
			t.Errorf("%s should show the unknown badge on API failure: %q", name, row)
		}
	}

	jsonOut, _, _ := runLsCapture(t, p, trees, "--json")
	if !strings.Contains(jsonOut, `"outdated": null`) {
		t.Errorf("--json should mark every skill unknown on API failure:\n%s", jsonOut)
	}
	if strings.Contains(jsonOut, `"outdated": true`) {
		t.Errorf("--json must not claim any skill is outdated when the check failed:\n%s", jsonOut)
	}
}

func TestLsSkipsTheCheckWhenNothingIsCheckable(t *testing.T) {
	p := fakeHome(t)
	if err := os.Remove(p.SkillLock()); err != nil {
		t.Fatal(err)
	}
	trees := &fakeTrees{}

	_, _, _ = runLsCapture(t, p, trees)

	if len(trees.calls) != 0 {
		t.Errorf("a home with only custom skills must not touch the API, calls = %v", trees.calls)
	}
}

func TestLsTableShowsTheFullPicture(t *testing.T) {
	p := fakeHome(t)
	out := runLs(t, p)

	// Only installed harnesses get a column; claude and cursor are absent
	// from the fake home.
	for _, col := range []string{"NAME", "ORIGIN", "SOURCE", "DESCRIPTION", "OPENCODE", "PI", "CODEX", "BOB"} {
		if !strings.Contains(out, col) {
			t.Errorf("table missing column %q:\n%s", col, out)
		}
	}
	for _, absent := range []string{"CLAUDE", "CURSOR"} {
		if strings.Contains(out, absent) {
			t.Errorf("table shows column for uninstalled harness %q:\n%s", absent, out)
		}
	}

	// Custom first, then installed grouped by source repo.
	names := []string{"my-notes", "git-helper", "tdd", "deploy-vercel"}
	positions := make([]int, len(names))
	for i, name := range names {
		idx := strings.Index(out, tableRow(t, out, name))
		if idx < 0 {
			t.Fatalf("row for %s missing:\n%s", name, out)
		}
		positions[i] = idx
		if i > 0 && positions[i] < positions[i-1] {
			t.Errorf("row %s appears before %s; want custom first, then by source:\n%s", name, names[i-1], out)
		}
	}

	// The custom skill is marked custom with no source.
	row := tableRow(t, out, "my-notes")
	if !strings.Contains(row, "custom") {
		t.Errorf("my-notes row not marked custom: %q", row)
	}
	if strings.Count(row, "-") < 1 {
		t.Errorf("my-notes row should show a dash for source: %q", row)
	}

	// Per-harness states: pi disabled tdd, opencode disabled the git
	// helpers, codex disabled deploy-vercel.
	if row = tableRow(t, out, "tdd"); !strings.Contains(row, "off") {
		t.Errorf("tdd row should show pi off: %q", row)
	}
	if row = tableRow(t, out, "git-helper"); !strings.Contains(row, "off") {
		t.Errorf("git-helper row should show opencode off: %q", row)
	}
	if row = tableRow(t, out, "deploy-vercel"); !strings.Contains(row, "off") {
		t.Errorf("deploy-vercel row should show codex off: %q", row)
	}

	// Provenance shows up as the source repo.
	if row = tableRow(t, out, "tdd"); !strings.Contains(row, "mattpocock/skills") {
		t.Errorf("tdd row should show its source repo: %q", row)
	}
}

func TestLsJSONEmitsTheSameDataStructurally(t *testing.T) {
	p := fakeHome(t)
	out := runLs(t, p, "--json")

	var report struct {
		Harnesses []string `json:"harnesses"`
		Skills    []struct {
			Name        string            `json:"name"`
			Custom      bool              `json:"custom"`
			Source      string            `json:"source"`
			SourceType  string            `json:"sourceType"`
			Hash        string            `json:"hash"`
			InstalledAt string            `json:"installedAt"`
			UpdatedAt   string            `json:"updatedAt"`
			Description string            `json:"description"`
			States      map[string]string `json:"states"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}

	wantHarnesses := []string{"opencode", "pi", "codex", "bob"}
	if len(report.Harnesses) != len(wantHarnesses) {
		t.Fatalf("harnesses = %v, want %v", report.Harnesses, wantHarnesses)
	}
	for i, h := range wantHarnesses {
		if report.Harnesses[i] != h {
			t.Errorf("harnesses[%d] = %q, want %q", i, report.Harnesses[i], h)
		}
	}

	if len(report.Skills) != 4 {
		t.Fatalf("skills = %d entries, want 4:\n%s", len(report.Skills), out)
	}

	wantOrder := []string{"my-notes", "git-helper", "tdd", "deploy-vercel"}
	for i, name := range wantOrder {
		if report.Skills[i].Name != name {
			t.Errorf("skills[%d].Name = %q, want %q (custom first, then by source)", i, report.Skills[i].Name, name)
		}
	}

	byName := map[string]map[string]string{}
	for i, s := range report.Skills {
		m := map[string]string{
			"custom":      jsonBool(s.Custom),
			"source":      s.Source,
			"description": s.Description,
		}
		for h, state := range s.States {
			m["state:"+h] = state
		}
		byName[s.Name] = m
		_ = i
	}

	custom := byName["my-notes"]
	if custom["custom"] != "true" || custom["source"] != "" {
		t.Errorf("my-notes = %v, want custom with no source", custom)
	}
	tdd := byName["tdd"]
	if tdd["custom"] != "false" || tdd["source"] != "mattpocock/skills" || tdd["description"] != "Red-green-refactor workflow for strict TDD." {
		t.Errorf("tdd row = %v", tdd)
	}
	if tdd["state:pi"] != "off" || tdd["state:opencode"] != "on" {
		t.Errorf("tdd states = %v", tdd)
	}
	if byName["git-helper"]["state:opencode"] != "off" {
		t.Errorf("git-helper opencode state = %q, want off", byName["git-helper"]["state:opencode"])
	}
	if byName["deploy-vercel"]["state:codex"] != "off" {
		t.Errorf("deploy-vercel codex state = %q, want off", byName["deploy-vercel"]["state:codex"])
	}
	if byName["my-notes"]["state:bob"] != "on" {
		t.Errorf("my-notes bob state = %q, want on", byName["my-notes"]["state:bob"])
	}
}

func TestLsJSONCarriesLockfileProvenance(t *testing.T) {
	p := fakeHome(t)
	out := runLs(t, p, "--json")

	var report struct {
		Skills []struct {
			Name        string `json:"name"`
			Hash        string `json:"hash"`
			InstalledAt string `json:"installedAt"`
			UpdatedAt   string `json:"updatedAt"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	for _, s := range report.Skills {
		if s.Name != "tdd" {
			continue
		}
		if s.Hash != "aaa111" || s.InstalledAt != "2026-08-08T04:30:28.403Z" || s.UpdatedAt != "2026-08-21T06:17:32.238Z" {
			t.Errorf("tdd provenance = hash %q, installedAt %q, updatedAt %q", s.Hash, s.InstalledAt, s.UpdatedAt)
		}
		return
	}
	t.Fatal("tdd missing from JSON")
}

func TestLsSummaryLineFollowsBannerRules(t *testing.T) {
	t.Run("piped output has no summary line", func(t *testing.T) {
		p := fakeHome(t)
		if out := runLs(t, p); strings.Contains(out, "fleet ·") {
			t.Errorf("piped output should not print the summary line:\n%s", out)
		}
	})

	t.Run("terminal output has a summary line", func(t *testing.T) {
		p := fakeHome(t)
		stdoutTTY = func() bool { return true }
		out := runLs(t, p)
		want := "fleet · 4 skills · 3 installed · 1 custom"
		if !strings.Contains(out, want) {
			t.Errorf("summary line = missing, want %q in:\n%s", want, out)
		}
	})

	t.Run("quiet suppresses the summary line even on a terminal", func(t *testing.T) {
		p := fakeHome(t)
		stdoutTTY = func() bool { return true }
		if out := runLs(t, p, "--quiet"); strings.Contains(out, "fleet ·") {
			t.Errorf("--quiet should suppress the summary line:\n%s", out)
		}
	})

	t.Run("json suppresses the summary line even on a terminal", func(t *testing.T) {
		p := fakeHome(t)
		stdoutTTY = func() bool { return true }
		if out := runLs(t, p, "--json"); strings.Contains(out, "fleet ·") {
			t.Errorf("--json should suppress the summary line:\n%s", out)
		}
	})
}

func TestShouldPrintHeader(t *testing.T) {
	cases := []struct {
		json  bool
		quiet bool
		tty   bool
		want  bool
	}{
		{false, false, true, true},
		{false, false, false, false},
		{false, true, true, false},
		{true, false, true, false},
		{true, true, true, false},
	}
	for _, c := range cases {
		if got := shouldPrintHeader(c.json, c.quiet, c.tty); got != c.want {
			t.Errorf("shouldPrintHeader(json=%v, quiet=%v, tty=%v) = %v, want %v", c.json, c.quiet, c.tty, got, c.want)
		}
	}
}

func TestLsWithEmptyHomeReportsNoSkills(t *testing.T) {
	p := paths.New(filepath.Join(t.TempDir(), "home"))
	if out := runLs(t, p); !strings.Contains(out, "no skills found") {
		t.Errorf("output = %q, want a no-skills message", out)
	}

	out := runLs(t, p, "--json")
	var report struct {
		Harnesses []string       `json:"harnesses"`
		Skills    []skillRowJSON `json:"skills"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if len(report.Skills) != 0 {
		t.Errorf("skills = %v, want empty", report.Skills)
	}
}

func TestLsMarksRepoCustomsAndPrefersTheStoreOnNameClashes(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	repo := filepath.Join(t.TempDir(), "repo")
	p := paths.WithRepo(home, repo)

	// A repo skill that was never adopted by fleet (hand-placed), and an
	// adopted skill whose lockfile entry lingers from its pre-adoption
	// install (the store copy moved away with it).
	writeSkillDir(t, p.RepoSkills(), "my-notes", "Personal note-taking conventions.")
	writeSkillDir(t, p.RepoSkills(), "tdd", "Forked and adopted.")
	if err := os.MkdirAll(p.AgentsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.SkillLock(), []byte(`{"version": 3, "skills": {
		"tdd": {"source": "mattpocock/skills", "sourceType": "github", "skillFolderHash": "aaa111"}
	}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{p.OpenCodeDir(), p.BobDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	out, _, _ := runLsCapture(t, p, &fakeTrees{}, "--json")
	var report struct {
		Skills []struct {
			Name     string            `json:"name"`
			Custom   bool              `json:"custom"`
			Source   string            `json:"source"`
			States   map[string]string `json:"states"`
			Outdated *bool             `json:"outdated"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}

	byName := map[string]struct {
		custom bool
		source string
	}{}
	names := []string{}
	for _, s := range report.Skills {
		byName[s.Name] = struct {
			custom bool
			source string
		}{s.Custom, s.Source}
		names = append(names, s.Name)
	}
	if len(names) != 2 {
		t.Fatalf("skills = %v, want one row per name (clash deduped)", names)
	}
	if c := byName["my-notes"]; !c.custom || c.source != "" {
		t.Errorf("my-notes = %+v, want custom with no source", c)
	}
	// The lingering lock entry is stale provenance: the repo copy is
	// custom, and no API call is spent on it.
	if c := byName["tdd"]; !c.custom || c.source != "" {
		t.Errorf("tdd = %+v, want custom with no source despite the stale lock entry", c)
	}

	// A name present in both places reports once, from the canonical store.
	writeSkillDir(t, p.SkillsStore(), "clash", "The live copy lives here.")
	writeSkillDir(t, p.RepoSkills(), "clash", "The live copy lives here.")
	out, _, _ = runLsCapture(t, p, &fakeTrees{}, "--json")
	if strings.Count(out, `"name": "clash"`) != 1 {
		t.Errorf("a name in both places must report once:\n%s", out)
	}
}

func TestLsSkipsRepoScanOutsideARepo(t *testing.T) {
	p := fakeHome(t) // built with paths.New: no repo bound
	out := runLs(t, p)
	if !strings.Contains(out, "my-notes") {
		t.Errorf("ls outside a repo should still list the store:\n%s", out)
	}
}

func TestLsFailsOnBrokenHarnessConfig(t *testing.T) {
	p := fakeHome(t)
	if err := os.WriteFile(p.OpenCodeConfig(), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRoot(p)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"skill", "ls"})
	if err := root.Execute(); err == nil {
		t.Fatal("ls on a broken harness config should fail")
	}
}

func jsonBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func tableRow(t *testing.T, out, name string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == name {
			return line
		}
	}
	t.Fatalf("no table row for %q in:\n%s", name, out)
	return ""
}

type skillRowJSON struct {
	Name string `json:"name"`
}
