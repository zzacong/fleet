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

func TestFromEnvPrecedenceEnvOverConfigOverEmpty(t *testing.T) {
	home := t.TempDir()
	repoFile := filepath.Join(home, "repo-file")
	repoEnv := filepath.Join(home, "repo-env")
	for _, r := range []string{repoFile, repoEnv} {
		if err := os.MkdirAll(filepath.Join(r, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Write config file pointing at repoFile.
	if err := os.MkdirAll(filepath.Join(home, ".config", "fleet"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "fleet", "config.json"), []byte(`{"skillsRepo": "`+repoFile+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLEET_HOME", home)

	// env overrides config
	t.Setenv("FLEET_REPO", repoEnv)
	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if p.Repo != repoEnv {
		t.Errorf("Repo = %q, want env %q", p.Repo, repoEnv)
	}
	if got := p.RepoSkills(); got != filepath.Join(repoEnv, "skills") {
		t.Errorf("RepoSkills() = %q, want %q", got, filepath.Join(repoEnv, "skills"))
	}

	// without env, falls back to config
	t.Setenv("FLEET_REPO", "")
	p, err = FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if p.Repo != repoFile {
		t.Errorf("Repo = %q, want config %q", p.Repo, repoFile)
	}
	if got := p.RepoSkills(); got != filepath.Join(repoFile, "skills") {
		t.Errorf("RepoSkills() = %q, want %q", got, filepath.Join(repoFile, "skills"))
	}

	// without env and without file, empty
	if err := os.Remove(filepath.Join(home, ".config", "fleet", "config.json")); err != nil {
		t.Fatal(err)
	}
	p, err = FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if p.Repo != "" {
		t.Errorf("Repo = %q, want empty when no env and no config", p.Repo)
	}
	if got := p.RepoSkills(); got != "" {
		t.Errorf("RepoSkills() = %q, want empty when no repo", got)
	}
}

func TestFromEnvDoesNotPickUpUnrelatedGitCheckout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLEET_HOME", home)
	t.Setenv("FLEET_REPO", "")

	// No config file -> empty
	repo := filepath.Join(t.TempDir(), "unrelated")
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
	if p.Repo != "" {
		t.Errorf("Repo = %q, want empty when in unrelated checkout with no config", p.Repo)
	}
	if got := p.RepoSkills(); got != "" {
		t.Errorf("RepoSkills() = %q, want empty", got)
	}
}

func TestFromEnvResolvesRelativeFleetRepoToAbsolute(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLEET_HOME", home)
	repoDir := filepath.Join(t.TempDir(), "myrepo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a temp wd and use a relative path to repoDir
	wd := t.TempDir()
	t.Chdir(wd)
	rel, err := filepath.Rel(wd, repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(rel) {
		t.Fatalf("expected relative path, got %q", rel)
	}
	t.Setenv("FLEET_REPO", rel)

	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	absWant, _ := filepath.Abs(rel)
	if p.Repo != absWant {
		t.Errorf("Repo = %q, want absolute %q", p.Repo, absWant)
	}
	if !filepath.IsAbs(p.Repo) {
		t.Errorf("Repo %q is not absolute", p.Repo)
	}
	if got := p.RepoSkills(); got != filepath.Join(absWant, "skills") {
		t.Errorf("RepoSkills() = %q, want %q", got, filepath.Join(absWant, "skills"))
	}
}

func TestFleetConfigFileAndFleetHomeSkillsAreFleetHomeAware(t *testing.T) {
	p := New("/home/fake")
	if got := p.FleetConfigFile(); got != "/home/fake/.config/fleet/config.json" {
		t.Errorf("FleetConfigFile() = %q, want /home/fake/.config/fleet/config.json", got)
	}
	if got := p.FleetHomeSkills(); got != "/home/fake/.config/fleet/skills" {
		t.Errorf("FleetHomeSkills() = %q, want /home/fake/.config/fleet/skills", got)
	}
	sandbox := filepath.Join(t.TempDir(), "home2")
	t.Setenv("FLEET_HOME", sandbox)
	t.Setenv("FLEET_REPO", "")
	p2, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if p2.FleetConfigFile() != filepath.Join(sandbox, ".config", "fleet", "config.json") {
		t.Errorf("FleetConfigFile() = %q, want derived from FLEET_HOME", p2.FleetConfigFile())
	}
	if p2.FleetHomeSkills() != filepath.Join(sandbox, ".config", "fleet", "skills") {
		t.Errorf("FleetHomeSkills() = %q, want derived from FLEET_HOME", p2.FleetHomeSkills())
	}
}
