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

func configHome(t *testing.T) *paths.Paths {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	return paths.New(home)
}

func runConfig(t *testing.T, p *paths.Paths, args ...string) (string, string, error) {
	t.Helper()
	t.Cleanup(func() { stdoutTTY = func() bool { return false } })
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root := NewRoot(p)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(append([]string{"config"}, args...))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func TestConfigGetEmptyWhenUnset(t *testing.T) {
	p := configHome(t)
	out, _, err := runConfig(t, p, "get", "adopt-target")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("get when unset = %q, want empty", out)
	}
}

func TestConfigSetAndGetRoundTrip(t *testing.T) {
	p := configHome(t)
	target := filepath.Join(t.TempDir(), "customs", "skills")
	if _, _, err := runConfig(t, p, "set", "adopt-target", target); err != nil {
		t.Fatalf("set: %v", err)
	}
	out, _, err := runConfig(t, p, "get", "adopt-target")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.TrimSpace(out) != target {
		t.Errorf("get = %q, want %q", strings.TrimSpace(out), target)
	}
	// file should be canonical
	body, _ := os.ReadFile(p.FleetConfigFile())
	if !strings.Contains(string(body), "\"adoptTarget\": \""+target+"\"") {
		t.Errorf("config file missing adoptTarget:\n%s", body)
	}
}

func TestConfigSetAcceptsMissingDirAndExpandsHome(t *testing.T) {
	p := configHome(t)
	// adopt creates the target on demand, so set must not require it.
	target := filepath.Join(t.TempDir(), "not-yet", "skills")
	if _, _, err := runConfig(t, p, "set", "adopt-target", target); err != nil {
		t.Fatalf("set with missing dir should succeed: %v", err)
	}
	out, _, err := runConfig(t, p, "get", "adopt-target")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != target {
		t.Errorf("get = %q, want %q", strings.TrimSpace(out), target)
	}
	// ~/ expands to the home directory.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runConfig(t, p, "set", "adopt-target", "~/customs/skills"); err != nil {
		t.Fatalf("set with ~ should succeed: %v", err)
	}
	out, _, err = runConfig(t, p, "get", "adopt-target")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != filepath.Join(home, "customs", "skills") {
		t.Errorf("get = %q, want home-expanded", strings.TrimSpace(out))
	}
}

func TestConfigSetRejectsRelative(t *testing.T) {
	p := configHome(t)
	if _, _, err := runConfig(t, p, "set", "adopt-target", "relative/path"); err == nil {
		t.Error("relative path should be rejected")
	}
}

func TestConfigUnsetClears(t *testing.T) {
	p := configHome(t)
	target := filepath.Join(t.TempDir(), "customs", "skills")
	if _, _, err := runConfig(t, p, "set", "adopt-target", target); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runConfig(t, p, "unset", "adopt-target"); err != nil {
		t.Fatalf("unset: %v", err)
	}
	out, _, err := runConfig(t, p, "get", "adopt-target")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("after unset get = %q, want empty", out)
	}
}

func TestConfigRetiredSinglePointerKeyFailsWithHint(t *testing.T) {
	p := configHome(t)
	for _, key := range []string{"skills-repo", "skillsRepo"} {
		if _, _, err := runConfig(t, p, "get", key); err == nil || !strings.Contains(err.Error(), "retired") {
			t.Errorf("get %s err = %v, want the retired-key hint", key, err)
		}
		if _, _, err := runConfig(t, p, "set", key, "/tmp/x"); err == nil || !strings.Contains(err.Error(), "retired") {
			t.Errorf("set %s err = %v, want the retired-key hint", key, err)
		}
		if _, _, err := runConfig(t, p, "unset", key); err == nil || !strings.Contains(err.Error(), "retired") {
			t.Errorf("unset %s err = %v, want the retired-key hint", key, err)
		}
	}
}

func TestConfigListShowsTrackedListAndTarget(t *testing.T) {
	p := configHome(t)
	repoA := filepath.Join(t.TempDir(), "repo-a")
	repoB := filepath.Join(t.TempDir(), "repo-b")
	target := filepath.Join(t.TempDir(), "customs", "skills")
	body, _ := json.Marshal(map[string]any{
		"skillsRepos": []string{repoA, repoB},
		"adoptTarget": target,
	})
	if err := os.MkdirAll(filepath.Dir(p.FleetConfigFile()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.FleetConfigFile(), body, 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := runConfig(t, p, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"skills-repos = " + repoA, "skills-repos = " + repoB, "adopt-target = " + target} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing %q in %q", want, out)
		}
	}
	out, _, err = runConfig(t, p, "list", "--json")
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	var obj struct {
		Repos  []string `json:"skillsRepos"`
		Target string   `json:"adoptTarget"`
	}
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatalf("list --json not JSON: %v\n%s", err, out)
	}
	if len(obj.Repos) != 2 || obj.Repos[0] != repoA || obj.Repos[1] != repoB {
		t.Errorf("list --json skillsRepos = %q, want order-preserved [%q %q]", obj.Repos, repoA, repoB)
	}
	if obj.Target != target {
		t.Errorf("list --json adoptTarget = %q, want %q", obj.Target, target)
	}
}

func TestConfigListEmpty(t *testing.T) {
	p := configHome(t)
	out, _, err := runConfig(t, p, "list")
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("list when empty = %q, want empty", out)
	}
	out, _, err = runConfig(t, p, "list", "--json")
	if err != nil {
		t.Fatalf("list --json empty: %v", err)
	}
	var obj map[string]string
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatalf("list --json empty not JSON: %v", err)
	}
	if len(obj) != 0 {
		t.Errorf("list --json when empty = %v, want empty object", obj)
	}
}
