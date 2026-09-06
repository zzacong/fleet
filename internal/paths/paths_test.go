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

func TestFromEnvIgnoresUnknownFileKeys(t *testing.T) {
	home := t.TempDir()
	// A config carrying only unknown keys behaves as unset: no tracked
	// repos come from it. (The retired single-pointer key is covered at
	// the config seam; here the read path must simply not choke.)
	if err := os.MkdirAll(filepath.Join(home, ".config", "fleet"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "fleet", "config.json"), []byte(`{"future": 123}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLEET_HOME", home)
	t.Setenv("FLEET_REPO", "")

	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if p.Home != home {
		t.Errorf("Home = %q, want %q", p.Home, home)
	}
	tracked, err := p.TrackedRepos()
	if err != nil {
		t.Fatalf("TrackedRepos() error = %v", err)
	}
	if len(tracked) != 0 {
		t.Errorf("TrackedRepos() = %q, want empty (old file key behaves as unset)", tracked)
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
	if p.Home != home {
		t.Errorf("Home = %q, want %q (an unrelated checkout must not leak in)", p.Home, home)
	}
}

func TestTrackedReposResolvesRelativeEnvToAbsolute(t *testing.T) {
	home := t.TempDir()
	p := New(home)
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

	got, err := p.TrackedRepos()
	if err != nil {
		t.Fatalf("TrackedRepos() error = %v", err)
	}
	absWant, _ := filepath.Abs(rel)
	if len(got) != 1 || got[0] != absWant {
		t.Errorf("TrackedRepos() = %q, want absolute [%q]", got, absWant)
	}
	if !filepath.IsAbs(got[0]) {
		t.Errorf("TrackedRepos()[0] %q is not absolute", got[0])
	}
}

func TestFleetReposDirDerivesFromInjectedHome(t *testing.T) {
	p := New("/home/fake")
	if got := p.FleetReposDir(); got != "/home/fake/.config/fleet/repos" {
		t.Errorf("FleetReposDir() = %q, want /home/fake/.config/fleet/repos", got)
	}
}

func TestInsideFleetHomeClassifiesWithFakeHomeOnly(t *testing.T) {
	home := t.TempDir()
	p := New(home)
	fleetDir := filepath.Join(home, ".config", "fleet")
	inside := []string{
		filepath.Join(fleetDir, "repos", "customs"),
		filepath.Join(fleetDir, "repos", "customs", "skills"),
		fleetDir,
		fleetDir + string(filepath.Separator) + ".",
		filepath.Join(fleetDir, "repos", "..", "repos", "team"),
	}
	for _, path := range inside {
		if !p.InsideFleetHome(path) {
			t.Errorf("InsideFleetHome(%q) = false, want true", path)
		}
	}
	outside := []string{
		home,
		filepath.Join(home, "Developer", "fleet"),
		filepath.Join(t.TempDir(), "elsewhere"),
		"relative/path",
		filepath.Join(fleetDir, "..", "opencode"),
	}
	for _, path := range outside {
		if p.InsideFleetHome(path) {
			t.Errorf("InsideFleetHome(%q) = true, want false", path)
		}
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

func writeTrackedConfig(t *testing.T, p *Paths, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p.FleetConfigFile()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.FleetConfigFile(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTrackedReposEmptyWhenNothingSet(t *testing.T) {
	p := New(t.TempDir())
	t.Setenv("FLEET_REPO", "")
	got, err := p.TrackedRepos()
	if err != nil {
		t.Fatalf("TrackedRepos() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("TrackedRepos() = %q, want empty", got)
	}
}

func TestTrackedReposKeepsExplicitOrderThenScansFleetHomeAlphabetically(t *testing.T) {
	home := t.TempDir()
	p := New(home)
	t.Setenv("FLEET_REPO", "")
	outsideB := filepath.Join(t.TempDir(), "explicit-b")
	outsideA := filepath.Join(t.TempDir(), "explicit-a")
	writeTrackedConfig(t, p, `{"skillsRepos": ["`+outsideB+`", "`+outsideA+`"]}`)
	for _, name := range []string{"zeta", "alpha", "mid"} {
		if err := os.MkdirAll(filepath.Join(p.FleetReposDir(), name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A stray file in the checkout parent is not a slot.
	if err := os.WriteFile(filepath.Join(p.FleetReposDir(), "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := p.TrackedRepos()
	if err != nil {
		t.Fatalf("TrackedRepos() error = %v", err)
	}
	want := []string{
		outsideB, outsideA,
		filepath.Join(p.FleetReposDir(), "alpha"),
		filepath.Join(p.FleetReposDir(), "mid"),
		filepath.Join(p.FleetReposDir(), "zeta"),
	}
	if len(got) != len(want) {
		t.Fatalf("TrackedRepos() = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TrackedRepos() = %q, want %q", got, want)
		}
	}
}

func TestTrackedReposPrependsEnvAndDedupes(t *testing.T) {
	home := t.TempDir()
	p := New(home)
	envRepo := filepath.Join(t.TempDir(), "env-repo")
	explicit := filepath.Join(t.TempDir(), "explicit")
	writeTrackedConfig(t, p, `{"skillsRepos": ["`+envRepo+`", "`+explicit+`"]}`)
	if err := os.MkdirAll(filepath.Join(p.FleetReposDir(), "auto"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLEET_REPO", envRepo)
	got, err := p.TrackedRepos()
	if err != nil {
		t.Fatalf("TrackedRepos() error = %v", err)
	}
	want := []string{envRepo, explicit, filepath.Join(p.FleetReposDir(), "auto")}
	if len(got) != len(want) {
		t.Fatalf("TrackedRepos() = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TrackedRepos() = %q, want %q", got, want)
		}
	}
}
