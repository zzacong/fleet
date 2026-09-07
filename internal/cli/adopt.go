// The adopt command: migrate a custom skill from the canonical store into
// the resolved adopt destination, wire that home into the config-path
// harnesses (opencode, pi), and manage the link-based harnesses' symlinks
// (codex, claude code, Cursor, Bob). The state file is untouched — custom is
// defined by living in a tracked collection or the fleet-home fallback — and
// sync runs after, as on every command.

package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/customs"
	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	fleetsync "github.com/zzacong/fleet/internal/sync"
)

func newSkillAdoptCmd(p *paths.Paths) *cobra.Command {
	var into string
	cmd := &cobra.Command{
		Use:   "adopt <name>",
		Short: "Move a custom skill from the canonical store into the resolved custom home",
		Long: "Move a custom skill from the canonical store (~/.agents/skills) into the resolved adopt destination, where it stays versioned.\n\n" +
			"The destination resolves as: --into <skills-dir> for this run, else the configured adopt target (`fleet config set adopt-target <skills-dir>`), else a numbered choice over the tracked collections plus the fleet-home fallback (~/.config/fleet/skills). With no tracked collections the fallback wins with no prompt.\n\n" +
			"The --into directory is a collection dir (not a repo root): it is created on demand and never saved. A choice from the prompt offers a yes/no follow-up (default No) to save it as the adopt target. Without a terminal an ambiguous adopt fails listing the candidates and the --into hint instead of blocking.\n\n" +
			"The resolved home is wired into OpenCode's and Pi's skill-path config, and every link-based harness (Codex, Claude Code, Cursor, Bob) gets a managed symlink to the skill. Managed links point into the resolved home, never into the canonical store — a link into ~/.agents/skills would make OpenCode and Pi see the skill twice, so custom skills get none.\n\n" +
			"Adoption is reversible by hand: move the directory back into ~/.agents/skills and fleet keeps working (doctor reports the leftovers). Adopting an already-adopted skill moves nothing but re-ensures the wiring and links.",
		Example: "  fleet skill adopt my-notes\n  fleet skill adopt my-notes --into ~/Developer/customs/skills",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			target, err := resolveAdoptTarget(cmd, p, into)
			if err != nil {
				return err
			}
			rep, err := customs.AdoptTo(p, args[0], target)
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
			// Report the home that was actually wired: the resolved
			// target for a move, the skill's actual home for a re-ensure.
			wired := filepath.Dir(rep.To)
			for _, w := range rep.Wired {
				if _, err := fmt.Fprintln(out, pal.dim("adopt: ")+formatWired(string(w.Harness), wired, w.Where, pal)); err != nil {
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
	cmd.Flags().StringVar(&into, "into", "", "collection dir to adopt into for this run (created on demand, never saved)")
	return cmd
}

// stdinIsTTY reports whether fleet's stdin is a terminal. Ambiguous adopts
// prompt only on a terminal; piped runs fail instead of blocking.
// Indirect through stdinTTY so tests can stub it.
func stdinIsTTY() bool { return stdinTTY() }

var stdinTTY = func() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// resolveAdoptTarget implements the adopt destination order: the --into flag
// for the run (created on demand, never persisted), else the configured
// adopt target, else the fallback when zero collections are tracked, else a
// one-shot numbered prompt over the tracked collections plus the fallback
// with an opt-in save-back defaulting to No. A supplied flag suppresses
// both prompts and any config write. Piped/non-terminal ambiguous runs fail
// with the candidate list and the flag hint.
func resolveAdoptTarget(cmd *cobra.Command, p *paths.Paths, into string) (string, error) {
	if into != "" {
		abs, err := config.AbsolutePath(into)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return "", err
	}
	if f.AdoptTarget() != "" {
		return f.AdoptTarget(), nil
	}
	candidates, err := customs.AdoptCandidates(p)
	if err != nil {
		return "", err
	}
	if len(candidates) <= 1 {
		// Zero tracked collections: the fallback alone, no prompt.
		return candidates[0], nil
	}
	if !stdinIsTTY() {
		return "", adoptNonTerminalError(candidates)
	}
	out := cmd.OutOrStdout()
	in := bufio.NewScanner(cmd.InOrStdin())
	idx, err := promptAdoptChoice(out, in, candidates)
	if err != nil {
		return "", err
	}
	target := candidates[idx]
	if err := promptAdoptSaveback(out, in, p, target); err != nil {
		return "", err
	}
	return target, nil
}

// adoptNonTerminalError fails an ambiguous adopt when there is no terminal
// to prompt on: it lists the candidates and hints the run-scoped flag.
func adoptNonTerminalError(candidates []string) error {
	var b strings.Builder
	b.WriteString("adopt: multiple destinations available — re-run with --into <skills-dir> or from a terminal:\n")
	for i, c := range candidates {
		fmt.Fprintf(&b, "  %d: %s\n", i+1, c)
	}
	return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
}

// promptAdoptChoice prints the numbered candidates and reads one choice.
// The choice is one-shot: a missing or out-of-range answer is an error, not
// a re-prompt.
func promptAdoptChoice(out io.Writer, in *bufio.Scanner, candidates []string) (int, error) {
	if _, err := fmt.Fprintln(out, "adopt: multiple destinations — choose one:"); err != nil {
		return 0, err
	}
	for i, c := range candidates {
		if _, err := fmt.Fprintf(out, "  [%d] %s\n", i+1, c); err != nil {
			return 0, err
		}
	}
	if _, err := fmt.Fprintf(out, "choose adopt destination [1-%d]: ", len(candidates)); err != nil {
		return 0, err
	}
	if !in.Scan() {
		return 0, fmt.Errorf("adopt: no input — re-run with --into <skills-dir>")
	}
	text := strings.TrimSpace(in.Text())
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 || n > len(candidates) {
		return 0, fmt.Errorf("adopt: invalid choice %q — want 1-%d", text, len(candidates))
	}
	return n - 1, nil
}

// promptAdoptSaveback offers to persist the prompt choice as the adopt
// target. The default is No: empty input, EOF, or anything but an explicit
// yes leaves config untouched.
func promptAdoptSaveback(out io.Writer, in *bufio.Scanner, p *paths.Paths, target string) error {
	if _, err := fmt.Fprint(out, "save as adopt target? [y/N]: "); err != nil {
		return err
	}
	if !in.Scan() {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(in.Text())) {
	case "y", "yes":
	default:
		return nil
	}
	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return err
	}
	f.SetAdoptTarget(target)
	return config.Save(p.FleetConfigFile(), f)
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
