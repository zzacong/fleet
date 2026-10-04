// The prune command: remove the stale disable rules and state entries for
// skills installed nowhere. With no flags it lists them and changes
// nothing; --yes applies. The removals run through internal/prune, which
// reuses the enable write paths, so only fleet's own exact disable shapes
// go and a state entry disappears once nothing remains. An incomplete scan
// removes nothing and names the home that blocked it. Sync runs after a
// successful apply, as on every command.

package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/prune"
)

func newSkillPruneCmd(p *paths.Paths) *cobra.Command {
	var yes bool
	var harnessFlags []string
	var configOnly, stateOnly bool

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove stale disable rules and state entries for skills installed nowhere",
		Long: "Remove the leftover disable rules and state entries for skills that are installed nowhere.\n\n" +
			"A harness config can still deny a skill you uninstalled, and the state file can still carry its dormant disable. Prune clears both. With no flags it lists what it would remove and changes nothing; --yes applies. --harness (repeatable) limits the work to named harnesses; --config-only and --state-only select one axis.\n\n" +
			"Removals go through the same enable write path the on verb uses, so only fleet's own exact disable shapes are removed: opencode's exact permission deny, pi's exact exclusion entry, codex's simple disabled block, and claude's \"off\" override. Pattern rules, blanket rules, extra-key shapes, and your own values survive untouched. A config rule for a skill the state does not track is doctor's manual-edit business and is left alone.\n\n" +
			"When a skill home is missing or unreadable the scan is incomplete, so prune removes nothing and names the home that blocked it. Sync runs after a successful prune, so the configs and the state file agree.",
		Example: "  fleet skill prune\n" +
			"  fleet skill prune --yes\n" +
			"  fleet skill prune --harness opencode --yes\n" +
			"  fleet skill prune --state-only --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			names, err := pruneHarnessNames(p, harnessFlags)
			if err != nil {
				return err
			}
			rep, err := prune.Run(p, prune.Options{
				Harnesses:  names,
				ConfigOnly: configOnly,
				StateOnly:  stateOnly,
				Apply:      yes,
			})
			if err != nil {
				return err
			}
			if len(rep.Blocked) > 0 {
				return printPruneBlocked(out, rep.Blocked)
			}
			if err := printPruneReport(out, rep, yes); err != nil {
				return err
			}
			if yes {
				return runSyncTo(out, p)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply the removals (without it, prune only lists them)")
	cmd.Flags().StringArrayVar(&harnessFlags, "harness", nil, "limit to this harness (repeatable); default: every harness")
	cmd.Flags().BoolVar(&configOnly, "config-only", false, "prune only harness config rules")
	cmd.Flags().BoolVar(&stateOnly, "state-only", false, "prune only state entries")
	cmd.MarkFlagsMutuallyExclusive("config-only", "state-only")
	return cmd
}

// pruneHarnessNames validates every --harness value against the supported
// harnesses and dedupes in flag order. Unlike resolveTargets it does not
// require the harness to be installed: prune can still clear its state
// entries, and a missing config is simply nothing to remove.
func pruneHarnessNames(p *paths.Paths, flags []string) ([]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	all := harness.All(p)
	known := map[string]bool{}
	for _, a := range all {
		known[string(a.Harness())] = true
	}
	seen := map[string]bool{}
	var names []string
	for _, f := range flags {
		if !known[f] {
			return nil, fmt.Errorf("unknown harness %q (want one of: %s)", f, harnessList(all))
		}
		if seen[f] {
			continue
		}
		seen[f] = true
		names = append(names, f)
	}
	return names, nil
}

// printPruneReport renders what prune removed (or would remove) and what
// it left alone, in the drop:/adopt: palette: dim command prefix, cyan
// harness, green verb, and a faint axis annotation.
func printPruneReport(out io.Writer, rep prune.Report, apply bool) error {
	pal := newPalette(stdoutIsTTY())
	if rep.Empty() && len(rep.Skipped) == 0 {
		_, err := fmt.Fprintf(out, "%s%s\n", pal.dim("prune: "), pal.good("nothing to prune"))
		return err
	}
	verb := "would remove"
	if apply {
		verb = "removed"
	}
	for _, c := range rep.Config {
		if _, err := fmt.Fprintf(out, "%s%s: %s %q %s\n", pal.dim("prune: "), pal.info(c.Harness), pal.good(verb), c.Skill, pal.dim("(config)")); err != nil {
			return err
		}
	}
	for _, s := range rep.State {
		if _, err := fmt.Fprintf(out, "%s%s: %s %q %s\n", pal.dim("prune: "), pal.info(s.Harness), pal.good(verb), s.Skill, pal.dim("(state)")); err != nil {
			return err
		}
	}
	for _, s := range rep.Skipped {
		if _, err := fmt.Fprintf(out, "%s%s: %s %q — %s\n", pal.dim("prune: "), pal.info(s.Harness), pal.dim("skipped"), s.Skill, pal.dim(s.Reason)); err != nil {
			return err
		}
	}
	if !apply && !rep.Empty() {
		_, err := fmt.Fprintf(out, "%s%s\n", pal.dim("prune: "), pal.dim("re-run with --yes to remove"))
		return err
	}
	return nil
}

// printPruneBlocked reports an incomplete scan: prune removed nothing
// because a home is missing or unreadable, and names each blocker.
func printPruneBlocked(out io.Writer, blocked []string) error {
	pal := newPalette(stdoutIsTTY())
	if _, err := fmt.Fprintf(out, "%s%s %s\n", pal.dim("prune: "), pal.warn("nothing removed"), pal.dim("— the skill scan is incomplete")); err != nil {
		return err
	}
	for _, home := range blocked {
		if _, err := fmt.Fprintf(out, "%s%s\n", pal.dim("prune: "), pal.info(home)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(out, "%s%s\n", pal.dim("prune: "), pal.dim("fix the home above, then re-run; `fleet skill doctor` reports the same blocker"))
	return err
}
