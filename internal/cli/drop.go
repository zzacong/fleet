// The drop command: remove a versioned customs home from the tracked
// set. The inverse of pull, not of adopt: an explicit repo (outside fleet
// home) is unlisted from the `skillsRepos` config list with the disk
// untouched; a fleet-home checkout is deleted from disk (presence is
// tracked, so deletion is the untrack). Git runs behind the injected
// pull.Runner seam (tests inject a stub). After the drop the collection
// is unwired from the config-path harnesses and its links are unlinked,
// then sync runs, as on every command.
package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/drop"
	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
)

func newSkillDropCmd(p *paths.Paths) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "drop <path-or-name>",
		Short: "Remove a tracked customs repo from the tracked set",
		Long: "Remove a versioned customs home from the tracked set with `fleet skill drop <path-or-name>`, resolved against the tracked repos.\n\n" +
			"The single arg is a repo-root path or a fleet-home slot name (no bare/prompt mode: this verb deletes). An explicit repo outside fleet home is removed from the tracked list with the disk untouched; a fleet-home checkout is deleted from disk.\n\n" +
			"A dirty working tree fails surfacing `git status --porcelain` output — fleet never stashes — unless --force is given. A missing git binary skips the dirty check with a warning instead. An adopt target pointing inside the dropped repo always fails with a re-point hint, even with --force.\n\n" +
			"After the drop the collection (<repo>/skills) is unwired from the config-path harnesses (opencode, pi) and its managed links unlinked (codex, claude code, Cursor, Bob), then sync runs.",
		Example: "  fleet skill drop my-customs\n" +
			"  fleet skill drop ~/Developer/team-customs\n" +
			"  fleet skill drop my-customs --force",
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return nil, cobra.ShellCompDirectiveFilterDirs
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			res, err := drop.Drop(p, newPullRunner(), expandPath(args[0]), force)
			if err != nil {
				return err
			}
			if res.Warning != "" {
				fmt.Fprintf(errOut, "warning: %s\n", res.Warning) //nolint:errcheck
			}
			unwired, err := harness.UnwireSkillSource(p, res.Collection)
			if err != nil {
				return err
			}
			unlinked, err := harness.RemoveCustomLinks(p, res.Collection)
			if err != nil {
				return err
			}
			if err := runSyncTo(out, p); err != nil {
				return err
			}
			return printDropResult(out, res, unwired, unlinked)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "drop even with uncommitted changes (adopt-target conflicts still fail)")
	return cmd
}

// printDropResult reports a completed drop in the pull:/adopt: palette
// style: the removed headline first, then one line per unwired harness
// and per unlinked managed link.
func printDropResult(out io.Writer, res *drop.Result, unwired []harness.UnwireResult, unlinked []harness.UnlinkResult) error {
	pal := newPalette(stdoutIsTTY())
	if _, err := fmt.Fprintf(out, "%s%s %q\n", pal.dim("drop: "), pal.good("removed"), res.Repo); err != nil {
		return err
	}
	for _, w := range unwired {
		if _, err := fmt.Fprintln(out, pal.dim("drop: ")+formatUnwired(string(w.Harness), res.Collection, w.Where, pal)); err != nil {
			return err
		}
	}
	for _, l := range unlinked {
		if _, err := fmt.Fprintln(out, pal.dim("drop: ")+formatUnlinked(string(l.Harness), l, pal)); err != nil {
			return err
		}
	}
	return nil
}

// formatUnwired renders one harness's skill-source removal, mirroring
// adopt's wired line: 'opencode: unwired "<dir>" as a skill source
// (skills.paths)'.
func formatUnwired(harnessName, dir, where string, pal palette) string {
	return fmt.Sprintf("%s: %s %q as a skill source %s", pal.info(harnessName), pal.good("unwired"), dir, pal.dim("("+where+")"))
}

// formatUnlinked renders one removed managed link, mirroring adopt's
// linked line: 'codex: unlinked "my-notes" → /repo/skills/my-notes'.
func formatUnlinked(harnessName string, l harness.UnlinkResult, pal palette) string {
	return fmt.Sprintf("%s: %s %q → %s", pal.info(harnessName), pal.good("unlinked"), l.Name, l.Target)
}
