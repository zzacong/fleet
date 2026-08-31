package paths

import "testing"

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
