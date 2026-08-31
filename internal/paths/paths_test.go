package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewDerivesEveryPathFromInjectedHomeRoot(t *testing.T) {
	p := New("/home/fake")

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"agents dir", p.AgentsDir(), "/home/fake/.agents"},
		{"canonical store", p.SkillsStore(), "/home/fake/.agents/skills"},
		{"skills lockfile", p.SkillLock(), "/home/fake/.agents/.skill-lock.json"},
		{"opencode dir", p.OpenCodeDir(), "/home/fake/.config/opencode"},
		{"opencode config", p.OpenCodeConfig(), "/home/fake/.config/opencode/opencode.jsonc"},
		{"pi dir", p.PiDir(), "/home/fake/.pi"},
		{"pi settings", p.PiSettings(), "/home/fake/.pi/agent/settings.json"},
		{"codex dir", p.CodexDir(), "/home/fake/.codex"},
		{"codex config", p.CodexConfig(), "/home/fake/.codex/config.toml"},
		{"codex skills", p.CodexSkills(), "/home/fake/.codex/skills"},
		{"claude dir", p.ClaudeDir(), "/home/fake/.claude"},
		{"claude skills", p.ClaudeSkills(), "/home/fake/.claude/skills"},
		{"claude settings", p.ClaudeSettings(), "/home/fake/.claude/settings.json"},
		{"cursor dir", p.CursorDir(), "/home/fake/.cursor"},
		{"cursor skills", p.CursorSkills(), "/home/fake/.cursor/skills"},
		{"bob dir", p.BobDir(), "/home/fake/.bob"},
		{"bob skills", p.BobSkills(), "/home/fake/.bob/skills"},
		{"fleet config dir", p.FleetConfigDir(), "/home/fake/.config/fleet"},
		{"fleet state file", p.FleetStateFile(), "/home/fake/.config/fleet/state.json"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

func TestFromEnvPrefersFleetHomeOverUserHome(t *testing.T) {
	t.Setenv("FLEET_HOME", "/home/sandbox")
	t.Setenv("HOME", "/home/real")

	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if p.Home != "/home/sandbox" {
		t.Errorf("Home = %q, want the FLEET_HOME override /home/sandbox", p.Home)
	}
	if got := p.SkillsStore(); got != "/home/sandbox/.agents/skills" {
		t.Errorf("SkillsStore() = %q, want it derived from FLEET_HOME", got)
	}
}

func TestFromEnvFallsBackToUserHomeWithoutFleetHome(t *testing.T) {
	t.Setenv("FLEET_HOME", "")

	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if p.Home == "" {
		t.Fatal("Home is empty without FLEET_HOME; want the user home fallback")
	}
}

func TestRepoSkillsDerivesFromTheRepoRoot(t *testing.T) {
	p := WithRepo("/home/fake", "/repo")
	if got := p.RepoSkills(); got != "/repo/skills" {
		t.Errorf("RepoSkills() = %q, want /repo/skills", got)
	}

	// No repo: RepoSkills is empty and callers must guard, so the rest of
	// fleet keeps working outside a checkout.
	homeOnly := New("/home/fake")
	if got := homeOnly.RepoSkills(); got != "" {
		t.Errorf("RepoSkills() without a repo = %q, want empty", got)
	}
}

func TestDiscoverRepoWalksUpToTheNearestGitRoot(t *testing.T) {
	root := t.TempDir()

	t.Run("a .git directory marks the repo root", func(t *testing.T) {
		gitDir := filepath.Join(root, "checkout", ".git")
		deep := filepath.Join(root, "checkout", "internal", "deep")
		if err := os.MkdirAll(gitDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := DiscoverRepo(deep); got != filepath.Join(root, "checkout") {
			t.Errorf("DiscoverRepo(%q) = %q, want the checkout root", deep, got)
		}
	})

	t.Run("a .git file marks the worktree root", func(t *testing.T) {
		gitFile := filepath.Join(root, "worktree", ".git")
		deep := filepath.Join(root, "worktree", "cmd", "fleet")
		if err := os.MkdirAll(filepath.Dir(gitFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(gitFile, []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := DiscoverRepo(deep); got != filepath.Join(root, "worktree") {
			t.Errorf("DiscoverRepo(%q) = %q, want the worktree root", deep, got)
		}
	})

	t.Run("no .git anywhere yields empty", func(t *testing.T) {
		lonely := filepath.Join(root, "lonely", "deeper")
		if err := os.MkdirAll(lonely, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := DiscoverRepo(lonely); got != "" {
			t.Errorf("DiscoverRepo(%q) = %q, want empty", lonely, got)
		}
	})
}

func TestFromEnvPrefersFleetRepoOverDiscovery(t *testing.T) {
	t.Setenv("FLEET_HOME", "/home/sandbox")
	t.Setenv("FLEET_REPO", "/repo/override")

	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if p.Repo != "/repo/override" {
		t.Errorf("Repo = %q, want the FLEET_REPO override", p.Repo)
	}
	if got := p.RepoSkills(); got != "/repo/override/skills" {
		t.Errorf("RepoSkills() = %q, want it derived from FLEET_REPO", got)
	}
}

func TestFromEnvDiscoversTheRepoFromTheWorkingDirectory(t *testing.T) {
	t.Setenv("FLEET_HOME", "/home/sandbox")
	t.Setenv("FLEET_REPO", "")

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(repo, "internal", "x")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Chdir(deep)
	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if p.Repo != repo {
		t.Errorf("Repo = %q, want the discovered root %q", p.Repo, repo)
	}
}

func TestFromEnvWithoutARepoLeavesRepoEmpty(t *testing.T) {
	t.Setenv("FLEET_HOME", "/home/sandbox")
	t.Setenv("FLEET_REPO", "")

	// Somewhere with no .git up the tree.
	lonely := filepath.Join(t.TempDir(), "lonely")
	if err := os.MkdirAll(lonely, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(lonely)

	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if p.Repo != "" {
		t.Errorf("Repo = %q, want empty outside a checkout", p.Repo)
	}
}
