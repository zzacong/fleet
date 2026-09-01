// Shell completions come straight from Cobra's generator: `fleet
// completion <shell>` must emit a script that names fleet for every shell
// the generator supports, without fleet shipping its own generator.

package cli

import (
	"strings"
	"testing"

	"github.com/zzacong/fleet/internal/paths"
)

func TestCompletionEmitsScriptsNamingFleet(t *testing.T) {
	for shell := range map[string]bool{"bash": true, "zsh": true, "fish": true, "powershell": true} {
		t.Run(shell, func(t *testing.T) {
			out, err := runBare(t, paths.New(t.TempDir()), "completion", shell)
			if err != nil {
				t.Fatalf("fleet completion %s: %v", shell, err)
			}

			if !strings.Contains(out, "fleet") {
				t.Errorf("completion script never mentions fleet:\n%s", out)
			}
		})
	}
}

func TestCompletionZshDeclaresItself(t *testing.T) {
	// The zsh script is autoloaded, so it must carry the #compdef header.
	out, err := runBare(t, paths.New(t.TempDir()), "completion", "zsh")
	if err != nil {
		t.Fatalf("fleet completion zsh: %v", err)
	}

	if !strings.Contains(out, "compdef") {
		t.Errorf("zsh script missing #compdef header:\n%s", out)
	}
}

func TestCompletionUnknownShellYieldsNoScript(t *testing.T) {
	// Cobra's completion parent command answers an unsupported shell with
	// help, not a script — nothing shell-executable may leak out.
	out, err := runBare(t, paths.New(t.TempDir()), "completion", "tcsh")
	if err != nil {
		t.Fatalf("fleet completion tcsh: %v", err)
	}

	if !strings.Contains(out, "Usage:") {
		t.Errorf("unsupported shell did not fall back to help:\n%s", out)
	}
	if strings.Contains(out, "compdef") {
		t.Errorf("unsupported shell produced a completion script:\n%s", out)
	}
}
