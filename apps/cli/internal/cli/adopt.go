// The adopt command: migrate a custom skill from the canonical store into
// the resolved custom home (the designated skills repo's skills/ when a
// repo is set, otherwise ~/.config/fleet/skills), wire that home into the
// config-path harnesses (opencode, pi), and manage the link-based
// harnesses' symlinks (codex, claude code, Cursor, Bob). The state file is
// untouched — custom is defined by living in the resolved home — and sync
// runs after, as on every command.

package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/customs"
	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	fleetsync "github.com/zzacong/fleet/internal/sync"
)

func newSkillAdoptCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "adopt <name>",
		Short: "Move a custom skill from the canonical store into the resolved custom home",
		Long: "Move a custom skill from the canonical store (~/.agents/skills) into the resolved custom home (the designated skills repo's skills/ when a skills repo is set, otherwise ~/.config/fleet/skills), where it stays versioned.\n\n" +
			"The resolved home is wired into opencode's and pi's skill-path config, and every link-based harness (codex, claude code, Cursor, Bob) gets a managed symlink to the skill. Managed links point into the resolved home, never into the canonical store — a link into ~/.agents/skills would make opencode and pi see the skill twice, so custom skills get none.\n\n" +
			"Adoption is reversible by hand: move the directory back into ~/.agents/skills and fleet keeps working (doctor reports the leftovers). Adopting an already-adopted skill moves nothing but re-ensures the wiring and links.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			rep, err := customs.Adopt(p, args[0])
			if err != nil {
				return err
			}

			pal := newPalette(stdoutIsTTY())
			if rep.Moved {
				if _, err := fmt.Fprintf(out, "%s%s %q\n", pal.dim("adopt: "), pal.good("adopted"), rep.Skill); err != nil {
					return err
				}
				if _, err := fmt.Fprintf(out, "  %s%s\n", pal.dim("from "), rep.From); err != nil {
					return err
				}
				if _, err := fmt.Fprintf(out, "    %s %s\n", pal.dim("→"), rep.To); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(out, "%s%q is %s\n", pal.dim("adopt: "), rep.Skill, pal.good("already adopted")); err != nil {
					return err
				}
				if _, err := fmt.Fprintf(out, "  %s %s\n", pal.dim("→"), rep.To); err != nil {
					return err
				}
			}
			target := p.RepoSkills()
			if target == "" {
				target = p.FleetHomeSkills()
			}
			for _, w := range rep.Wired {
				if _, err := fmt.Fprintln(out, pal.dim("adopt: ")+formatWired(string(w.Harness), target, w.Where, pal)); err != nil {
					return err
				}
			}
			for _, l := range rep.Linked {
				if _, err := fmt.Fprintln(out, pal.dim("adopt: ")+formatLinkStyled(string(l.Harness), l, pal)); err != nil {
					return err
				}
			}

			// Sync: repair any other drift while we are here, but keep
			// the report quiet — only findings about the adopted skill
			// belong to this command. Everything else is sync's and
			// doctor's business.
			return printAdoptSync(out, p, rep.Skill)
		},
	}
}

// formatLink renders one managed-link action: 'codex: linked "my-notes" →
// /repo/skills/my-notes'.
//
//nolint:unused // kept for reference; styled variant is used in output
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

func formatWired(harness, dir, where string, pal palette) string {
	return fmt.Sprintf("%s: %s %q as a skill source %s", pal.info(harness), pal.good("wired"), dir, pal.dim("("+where+")"))
}

func formatLinkStyled(harnessName string, l harness.LinkResult, pal palette) string {
	switch l.Change.Action {
	case harness.LinkRepointed:
		return fmt.Sprintf("%s: %s %q %s → %s", pal.info(harnessName), pal.warn("repointed"), l.Name, pal.dim("(was "+l.Change.From+")"), l.Target)
	case harness.LinkSkipped:
		return fmt.Sprintf("%s: %s", pal.info(harnessName), pal.dim(l.Change.Note))
	default:
		return fmt.Sprintf("%s: %s %q → %s", pal.info(harnessName), pal.good("linked"), l.Name, l.Target)
	}
}

// printAdoptSync runs sync and reports only findings about the adopted
// skill. Ambient findings about other skills — the untracked config
// disables, foreign rules, and redundant links that belong to other
// skills — stay out of the adopt report; `fleet skill sync` and
// `fleet skill doctor` are where they are listed.
func printAdoptSync(out io.Writer, p *paths.Paths, skill string) error {
	reports, err := fleetsync.Run(p)
	if err != nil {
		return err
	}
	pal := newPalette(stdoutIsTTY())
	for _, r := range reports {
		for _, e := range r.Removed {
			if e.Name != skill {
				continue
			}
			if _, err := fmt.Fprintln(out, styleSyncLine(formatRemoved(r.Harness, e), pal)); err != nil {
				return err
			}
		}
		for _, c := range r.Changed {
			if c.Skill != skill {
				continue
			}
			if _, err := fmt.Fprintln(out, styleChange(formatChange(r.Harness, c), pal)); err != nil {
				return err
			}
		}
		var relevant []harness.Flag
		for _, f := range r.Flags {
			if f.Skill == skill {
				relevant = append(relevant, f)
			}
		}
		for _, g := range groupFlags(relevant) {
			line := ""
			if len(g.skills) == 1 {
				line = formatFlag(r.Harness, harness.Flag{Skill: g.skills[0], Message: g.message})
			} else {
				line = formatFlagGroup(r.Harness, g)
			}
			if _, err := fmt.Fprintln(out, styleSyncLine(line, pal)); err != nil {
				return err
			}
		}
	}
	return nil
}
