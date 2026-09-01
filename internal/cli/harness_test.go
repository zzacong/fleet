package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
)

// runHarnessLs runs `fleet harness ls` with the given args, returning
// stdout. fakeHome gives a realistic mix: opencode, pi, codex, and Bob
// installed; claude and cursor not.
func runHarnessLs(t *testing.T, p *paths.Paths, args ...string) string {
	t.Helper()
	out, _, err := runHarnessLsCapture(t, p, args...)
	if err != nil {
		t.Fatalf("fleet harness ls %v: error = %v", args, err)
	}
	return out
}

func runHarnessLsCapture(t *testing.T, p *paths.Paths, args ...string) (string, string, error) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"harness", "ls"}, args...))
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func TestHarnessLsTable(t *testing.T) {
	out := runHarnessLs(t, fakeHome(t))

	// All six supported harnesses appear.
	for _, name := range []string{"opencode", "pi", "codex", "claude", "cursor", "bob"} {
		if fieldAfterName(out, name, 0) == "" {
			t.Errorf("expected row for %q in:\n%s", name, out)
		}
	}
	// The INSTALLED column is tabwriter-padded, so installed markers are
	// checked through the parsed fields rather than tab matches.
	for _, name := range []string{"claude", "cursor"} {
		if fieldAfterName(out, name, 0) != "-" {
			t.Errorf("expected %q marked not installed (-) in:\n%s", name, out)
		}
	}
	for _, name := range []string{"opencode", "pi", "codex", "bob"} {
		if fieldAfterName(out, name, 0) != "✓" {
			t.Errorf("expected %q marked installed (✓) in:\n%s", name, out)
		}
	}
	// Off-switch honesty: the writable harnesses say yes, cursor and bob no.
	yesNames := map[string]bool{"opencode": true, "pi": true, "codex": true, "claude": true}
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "NAME" {
			continue
		}
		want := "no"
		if yesNames[fields[0]] {
			want = "yes"
		}
		if fields[len(fields)-1] != want {
			t.Errorf("expected %q's off switch to be %q in row %q", fields[0], want, line)
		}
	}
	// The config dirs are the ones the paths package derives.
	if !strings.Contains(out, filepath.Join(".config", "opencode")) {
		t.Errorf("expected opencode's config dir in:\n%s", out)
	}
}

func TestHarnessLsEmptyHome(t *testing.T) {
	// No harness dirs at all: every row is present and marked absent, and
	// the command still succeeds.
	out := runHarnessLs(t, paths.New(t.TempDir()))
	for _, name := range []string{"opencode", "pi", "codex", "claude", "cursor", "bob"} {
		if fieldAfterName(out, name, 0) != "-" {
			t.Errorf("expected %q marked not installed in:\n%s", name, out)
		}
	}
}

// fieldAfterName finds the table row beginning with name and returns its
// index'th field after the name, or "" when the row is missing.
func fieldAfterName(out, name string, index int) string {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1+index && fields[0] == name {
			return fields[1+index]
		}
	}
	return ""
}

func TestHarnessLsJSON(t *testing.T) {
	p := fakeHome(t)
	out := runHarnessLs(t, p, "--json")

	var report struct {
		Harnesses []struct {
			Name      string `json:"name"`
			Installed bool   `json:"installed"`
			ConfigDir string `json:"configDir"`
			CanOff    bool   `json:"canDisable"`
		} `json:"harnesses"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(report.Harnesses) != 6 {
		t.Fatalf("expected 6 harnesses, got %d", len(report.Harnesses))
	}

	byName := map[string]bool{}
	for _, h := range report.Harnesses {
		byName[h.Name] = h.Installed
	}
	for _, name := range []string{"opencode", "pi", "codex", "bob"} {
		if !byName[name] {
			t.Errorf("%s should be installed", name)
		}
	}
	for _, name := range []string{"claude", "cursor"} {
		if byName[name] {
			t.Errorf("%s should not be installed", name)
		}
	}

	// canDisable is honest: exactly the four adapters with a write side.
	off := map[string]bool{}
	for _, h := range report.Harnesses {
		off[h.Name] = h.CanOff
	}
	for _, name := range []string{"opencode", "pi", "codex", "claude"} {
		if !off[name] {
			t.Errorf("%s should report canDisable", name)
		}
	}
	for _, name := range []string{"cursor", "bob"} {
		if off[name] {
			t.Errorf("%s should not report canDisable", name)
		}
	}

	// configDir points at the probed directory for every harness.
	for _, h := range report.Harnesses {
		if h.ConfigDir == "" {
			t.Errorf("%s: empty configDir", h.Name)
		}
		if h.Installed {
			if info, err := os.Stat(h.ConfigDir); err != nil || !info.IsDir() {
				t.Errorf("%s: installed but configDir %q is not a directory", h.Name, h.ConfigDir)
			}
		}
	}
}

func TestHarnessLsSummaryOnlyOnTTY(t *testing.T) {
	p := fakeHome(t)

	// Piped: no summary line — scripts get bare table rows.
	out := runHarnessLs(t, p)
	if strings.Contains(out, "harnesses installed") {
		t.Errorf("piped output should omit the summary line:\n%s", out)
	}

	// TTY: the summary leads.
	stdoutTTY = func() bool { return true }
	defer func() { stdoutTTY = func() bool { return false } }()
	out = runHarnessLs(t, p)
	if !strings.HasPrefix(out, "fleet · 4 of 6 harnesses installed\n") {
		t.Errorf("expected TTY summary \"fleet · 4 of 6 harnesses installed\", got:\n%s", out)
	}

	// --json never carries the summary, even on a TTY.
	out = runHarnessLs(t, p, "--json")
	if strings.Contains(out, "harnesses installed") {
		t.Errorf("--json should omit the summary line:\n%s", out)
	}
}
