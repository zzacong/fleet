// The adopt command: migrate a custom skill from the canonical store into
// the fleet repo's skills/ directory, wire the repo path into the
// config-path harnesses (opencode, pi), and manage the link-based
// harnesses' symlinks (codex, claude code, Cursor, Bob). The state file is
// untouched — custom is defined by living in the repo — and sync runs
// after, as on every command.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zacong/fleet/internal/customs"
	"github.com/zacong/fleet/internal/harness"
	"github.com/zacong/fleet/internal/paths"
)

func newSkillAdoptCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "adopt <name>",
		Short: "Move a custom skill from the canonical store into the fleet repo",
		Long: "Move a hand-written skill from the canonical store (~/.agents/skills) into the fleet repo's skills/ directory, where it stays versioned.\n\n" +
			"The repo path is wired into opencode's and pi's skill-path config, and every link-based harness (codex, claude code, Cursor, Bob) gets a managed symlink to the skill. Managed links point at the repo, never at the canonical store — a link into ~/.agents/skills would make opencode and pi see the skill twice, so custom skills get none.\n\n" +
			"Adoption is reversible by hand: move the directory back into ~/.agents/skills and fleet keeps working (doctor reports the leftovers). Adopting an already-adopted skill moves nothing but re-ensures the wiring and links.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			rep, err := customs.Adopt(p, args[0])
			if err != nil {
				return err
			}

			if rep.Moved {
				if _, err := fmt.Fprintf(out, "moved %s → %s\n", rep.From, rep.To); err != nil {
					return err
				}
			} else if _, err := fmt.Fprintf(out, "%s is already in the repo — nothing to move\n", rep.Skill); err != nil {
				return err
			}
			for _, w := range rep.Wired {
				if _, err := fmt.Fprintf(out, "%s: wired %q as a skill source (%s)\n", w.Harness, p.RepoSkills(), w.Where); err != nil {
					return err
				}
			}
			for _, l := range rep.Linked {
				if _, err := fmt.Fprintln(out, formatLink(string(l.Harness), l)); err != nil {
					return err
				}
			}

			// Sync: repair any other drift while we are here.
			return runSyncTo(out, p)
		},
	}
}

// formatLink renders one managed-link action: 'codex: linked "my-notes" →
// /repo/skills/my-notes'.
func formatLink(harnessName string, l harness.LinkResult) string {
	switch l.Change.Action {
	case harness.LinkRepointed:
		return fmt.Sprintf("%s: repointed %q (was %s) → %s", harnessName, l.Name, l.Change.From, l.Target)
	case harness.LinkSkipped:
		return fmt.Sprintf("%s: %s", harnessName, l.Change.Note)
	default:
		return fmt.Sprintf("%s: linked %q → %s", harnessName, l.Name, l.Target)
	}
}
