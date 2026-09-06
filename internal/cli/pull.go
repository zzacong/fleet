// The pull command: clone a customs repo (defaulting into the
// auto-tracked fleet-home slot) and fast-forward every tracked repo on a
// bare pull. Git runs behind the injected pull.Runner seam (real shells
// out with inherited stdio so SSH agent and credential helpers just work;
// tests inject a stub). After clone or pull the collection dir is ensured
// (created with a warning on the empty-repo first run), the home is
// wired/linked, sync runs, and a per-repo report prints.
package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/pull"
)

// newPullRunner builds the git runner: the real exec runner. Tests swap
// it for a scripted stub — real git is absent in sandboxes.
var newPullRunner = func() pull.Runner { return pull.Exec{} }

func newSkillPullCmd(p *paths.Paths) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "pull [git-url] [path]",
		Short: "Clone a customs repo or fast-forward every tracked repo",
		Long: "Clone a customs repo with `fleet skill pull <git-url> [path]` and fast-forward every tracked repo with bare `fleet skill pull`.\n\n" +
			"With no path, the checkout lands in the auto-tracked fleet-home slot derived from the URL (final path or colon segment, trailing slashes and .git stripped) with no config write. An explicit path inside fleet home stays auto-tracked with no config write; an explicit path outside fleet home is appended once to the tracked list (no duplicates, no reordering). Re-pulling an existing checkout never reorders the list.\n\n" +
			"An existing checkout with the same remote fast-forwards only (`git pull --ff-only`): dirty or diverged trees fail with their state surfaced — fleet never stashes, merges, rebases, or resets. A checkout pointing at a different remote fails unless --force is given. A missing git binary fails cleanly. Bare pull skips non-git entries with a warning instead of failing the run.\n\n" +
			"After clone or pull the collection dir (<repo>/skills) is ensured (created with a warning on the empty-repo first run), the home is wired into the config-path harnesses and linked for the link-based harnesses, and sync runs. Auth inherits your stdio; fleet adds no credential flags.",
		Example: "  fleet skill pull git@github.com:me/my-customs.git\n" +
			"  fleet skill pull https://github.com/me/team.git ~/Developer/team-customs\n" +
			"  fleet skill pull\n" +
			"  fleet skill pull --force https://github.com/me/team.git ~/Developer/team-customs",
		Args: cobra.MaximumNArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 1 {
				return nil, cobra.ShellCompDirectiveFilterDirs
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			runner := newPullRunner()
			if len(args) == 0 {
				return runPullAll(cmd, p, runner, out, errOut)
			}
			url := strings.TrimSpace(args[0])
			if url == "" {
				return fmt.Errorf("git-url must not be empty")
			}
			var dest string
			if len(args) == 2 {
				expanded := expandPath(args[1])
				if !filepath.IsAbs(expanded) {
					abs, err := filepath.Abs(expanded)
					if err != nil {
						return err
					}
					expanded = abs
				}
				dest = filepath.Clean(expanded)
			} else {
				if pull.DeriveDirName(url) == "" {
					return fmt.Errorf("cannot derive a directory name from %q: pass an explicit path", url)
				}
				dest = pull.DefaultPath(p, url)
			}
			res, err := pull.PullOne(p, runner, url, dest, force)
			if err != nil {
				return err
			}
			if res.CollectionCreated {
				fmt.Fprintf(errOut, "warning: created missing collection dir %s — empty remote first run\n", filepath.Join(res.Repo, "skills")) //nolint:errcheck
			}
			if err := runSyncTo(out, p); err != nil {
				return err
			}
			return printPullResult(out, res, "")
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "pull even when the existing checkout points at a different remote")
	return cmd
}

func runPullAll(cmd *cobra.Command, p *paths.Paths, runner pull.Runner, out io.Writer, errOut io.Writer) error {
	results, err := pull.PullAll(p, runner)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		if err := runSyncTo(out, p); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, "pull: no tracked repos")
		return err
	}
	for _, r := range results {
		switch r.Outcome {
		case pull.OutcomeSkipped:
			fmt.Fprintf(errOut, "warning: skipped %s — %s\n", r.Repo, r.Detail) //nolint:errcheck
		default:
			if r.CollectionCreated {
				fmt.Fprintf(errOut, "warning: created missing collection dir %s — empty remote first run\n", filepath.Join(r.Repo, "skills")) //nolint:errcheck
			}
		}
	}
	if err := runSyncTo(out, p); err != nil {
		return err
	}
	failed := 0
	for _, r := range results {
		if r.Outcome == pull.OutcomeFailed {
			failed++
		}
		if err := printPullResult(out, &r, ""); err != nil {
			return err
		}
	}
	if failed > 0 {
		return fmt.Errorf("pull: %d of %d repos failed", failed, len(results))
	}
	return nil
}

func printPullResult(out io.Writer, res *pull.Result, _ string) error {
	pal := newPalette(stdoutIsTTY())
	switch res.Outcome {
	case pull.OutcomeCloned:
		_, err := fmt.Fprintf(out, "%s%s %q\n", pal.dim("pull: "), pal.good("cloned"), res.Repo)
		return err
	case pull.OutcomeUpdated:
		_, err := fmt.Fprintf(out, "%s%s %q\n", pal.dim("pull: "), pal.good("updated"), res.Repo)
		return err
	case pull.OutcomeCurrent:
		_, err := fmt.Fprintf(out, "%s%s %q\n", pal.dim("pull: "), pal.dim("current"), res.Repo)
		return err
	case pull.OutcomeSkipped:
		_, err := fmt.Fprintf(out, "%s%s %q — %s\n", pal.dim("pull: "), pal.warn("skipped"), res.Repo, res.Detail)
		return err
	case pull.OutcomeFailed:
		_, err := fmt.Fprintf(out, "%s%s %q — %s\n", pal.dim("pull: "), pal.broken("failed"), res.Repo, res.Detail)
		return err
	default:
		_, err := fmt.Fprintf(out, "pull: %s %q\n", res.Outcome, res.Repo)
		return err
	}
}
