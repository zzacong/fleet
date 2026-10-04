// The remove-dir command: unregister an explicit collection dir and unlink
// its managed skills from every installed harness. The inverse of add-dir,
// not of adopt: the disk is never touched. Every rule lives in the
// tracked-set module; this layer unlinks, prints, and syncs.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/customs"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/trackedset"
)

func newSkillRemoveDirCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "remove-dir <path>",
		Short: "Untrack a directory of skills and unlink it from every harness",
		Long: "Unregister a collection dir with `fleet skill remove-dir <path>`, removing it from the tracked list and unlinking its managed skills from every installed harness. The inverse of `add-dir`, not of `adopt`: fleet never deletes the directory or any file in it.\n\n" +
			"`~` expands to the home directory and a relative path resolves against the working directory, exactly like `add-dir`. A tracked path that is missing from disk still unlists cleanly; an untracked path is an error that lists the tracked dirs.\n\n" +
			"After the unlink, sync runs as on every command. Fleet consults no git binary.",
		Example: "  fleet skill remove-dir ~/Developer/projects/agent-skills/skills\n" +
			"  fleet skill remove-dir ./customs",
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			pal := newPalette(stdoutIsTTY())
			res, err := trackedset.Remove(p, args[0])
			if err != nil {
				return err
			}
			wd, err := customs.Withdraw(p, res.Dir)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "%s%s %q\n", pal.dim("remove-dir: "), pal.good("removed"), res.Dir); err != nil {
				return err
			}
			for _, l := range wd.Unlinked {
				if _, err := fmt.Fprintln(out, pal.dim("remove-dir: ")+formatUnlinked(string(l.Harness), l, pal)); err != nil {
					return err
				}
			}
			return runSyncTo(out, p)
		},
	}
}
