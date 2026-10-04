// The add-dir command: register an existing collection dir in the explicit
// tracked list and wire its skills into every installed harness. Every
// validation rule lives in the tracked-set module; this layer resolves
// nothing itself and consults no git.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/customs"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/trackedset"
)

func newSkillAddDirCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "add-dir <path>",
		Short: "Track an existing directory of skills and wire it into every harness",
		Long: "Register an existing collection dir with `fleet skill add-dir <path>`, appending it to the tracked list and linking every skill it holds into every installed harness.\n\n" +
			"A collection dir's immediate children are skill dirs, each holding a SKILL.md. `~` expands to the home directory and a relative path resolves against the working directory; the cleaned absolute path is stored verbatim, so re-running on the same directory is a no-op.\n\n" +
			"Validation is loud: the path must exist, be a directory holding at least one skill, and must not be the canonical store, the fleet-home fallback, or anywhere inside fleet home. It refuses an already tracked path's nested ancestor or descendant and a skill name that collides with another tracked dir or the fallback. A name collision with the canonical store is allowed — custom outranks canonical, and doctor reports the shadow.\n\n" +
			"Fleet never clones or deletes anything, and consults no git binary.",
		Example: "  fleet skill add-dir ~/Developer/projects/agent-skills/skills\n" +
			"  fleet skill add-dir ./customs",
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			pal := newPalette(stdoutIsTTY())
			res, err := trackedset.Add(p, args[0])
			if err != nil {
				return err
			}
			if !res.Added {
				_, err := fmt.Fprintf(out, "%s%q is %s\n", pal.dim("add-dir: "), res.Dir, pal.dim("already tracked"))
				return err
			}
			vis, err := customs.MakeVisible(p, res.Dir)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "%s%s %q\n", pal.dim("add-dir: "), pal.good("added"), res.Dir); err != nil {
				return err
			}
			for _, l := range vis.Linked {
				if _, err := fmt.Fprintln(out, pal.dim("add-dir: ")+formatLinkStyled(string(l.Harness), l, pal)); err != nil {
					return err
				}
			}
			return runSyncTo(out, p)
		},
	}
}
