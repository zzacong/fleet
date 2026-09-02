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
	out, _, err := runConfig(t, p, "get", "skills-repo")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("get when unset = %q, want empty", out)
	}
}

func TestConfigSetAndGetRoundTrip(t *testing.T) {
	p := configHome(t)
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runConfig(t, p, "set", "skills-repo", repo); err != nil {
		t.Fatalf("set: %v", err)
	}
	out, _, err := runConfig(t, p, "get", "skills-repo")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.TrimSpace(out) != repo {
		t.Errorf("get = %q, want %q", strings.TrimSpace(out), repo)
	}
	// file should be canonical
	body, _ := os.ReadFile(p.FleetConfigFile())
	if !strings.Contains(string(body), "\"skillsRepo\": \""+repo+"\"") {
		t.Errorf("config file missing skillsRepo:\n%s", body)
	}
}

func TestConfigSetWarnsWithoutGit(t *testing.T) {
	p := configHome(t)
	repo := filepath.Join(t.TempDir(), "repo-nogit")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	_, errOut, err := runConfig(t, p, "set", "skills-repo", repo)
	if err != nil {
		t.Fatalf("set without .git should still succeed: %v", err)
	}
	if !strings.Contains(errOut, "does not contain .git") {
		t.Errorf("expected warning about missing .git, got %q", errOut)
	}
}

func TestConfigSetRejectsRelativeAndMissing(t *testing.T) {
	p := configHome(t)
	if _, _, err := runConfig(t, p, "set", "skills-repo", "relative/path"); err == nil {
		t.Error("relative path should be rejected")
	}
	if _, _, err := runConfig(t, p, "set", "skills-repo", "/no/such/dir/xyz"); err == nil {
		t.Error("missing path should be rejected")
	}
}

func TestConfigUnsetClears(t *testing.T) {
	p := configHome(t)
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runConfig(t, p, "set", "skills-repo", repo); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runConfig(t, p, "unset", "skills-repo"); err != nil {
		t.Fatalf("unset: %v", err)
	}
	out, _, err := runConfig(t, p, "get", "skills-repo")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("after unset get = %q, want empty", out)
	}
}

func TestConfigListAndJSON(t *testing.T) {
	p := configHome(t)
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runConfig(t, p, "set", "skills-repo", repo); err != nil {
		t.Fatal(err)
	}
	out, _, err := runConfig(t, p, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, repo) {
		t.Errorf("list missing repo %q in %q", repo, out)
	}
	out, _, err = runConfig(t, p, "list", "--json")
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	var obj map[string]string
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatalf("list --json not JSON: %v\n%s", err, out)
	}
	if obj["skillsRepo"] != repo {
		t.Errorf("list --json skillsRepo = %q, want %q", obj["skillsRepo"], repo)
	}
}

func TestConfigGetRespectsEnvOverride(t *testing.T) {
	p := configHome(t)
	repoFile := filepath.Join(t.TempDir(), "repo-file")
	repoEnv := filepath.Join(t.TempDir(), "repo-env")
	for _, r := range []string{repoFile, repoEnv} {
		if err := os.MkdirAll(filepath.Join(r, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := runConfig(t, p, "set", "skills-repo", repoFile); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLEET_REPO", repoEnv)
	out, _, err := runConfig(t, p, "get", "skills-repo")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != repoEnv {
		t.Errorf("get with env = %q, want %q", strings.TrimSpace(out), repoEnv)
	}
	out, _, err = runConfig(t, p, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]string
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}
	if obj["skillsRepo"] != repoEnv {
		t.Errorf("list --json with env = %q, want %q", obj["skillsRepo"], repoEnv)
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
