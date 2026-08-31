// The doctor command: the read-only report of what's wrong. It does not
// run ambient sync — the point is to surface everything sync would change
// before sync changes it. The one thing it may write is the manual-edit
// resolution the user picks at the prompt: keep adopts the edit into the
// state file, restore syncs the state back into the config. Unknown
// entries and unmanageable rules are reported and never touched.

package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zacong/fleet/internal/doctor"
	"github.com/zacong/fleet/internal/harness"
	"github.com/zacong/fleet/internal/paths"
)

func newSkillDoctorCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Report what's wrong: drift, redundant links, broken links, unknown entries, manual edits",
		Long: "Inspect every installed harness and the canonical store, and report what is wrong: " +
			"redundant per-agent links, broken symlinks, unknown entries in skills dirs, " +
			"missing directories, and manual config edits that disagree with the state file.\n\n" +
			"Doctor is read-only: it reports without changing anything, so you see what sync would " +
			"do before sync does it (sync runs on every other command). Manual-edit disagreements " +
			"are the exception: each one is a prompt — \"keep my change\" records the edit in the " +
			"state file, \"restore\" syncs the state back into the config, and skipping changes nothing.\n\n" +
			"Answer the prompts from a terminal. With piped input, conflicts are reported and left as is.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			rep, err := doctor.Analyze(p)
			if err != nil {
				return err
			}

			if err := printFindings(out, rep.Findings); err != nil {
				return err
			}
			resolved, err := resolveConflicts(cmd, p, rep.Conflicts)
			if err != nil {
				return err
			}
			return printSummary(out, rep, resolved)
		},
	}
}

// findingSections fixes the report's section order and headers.
var findingSections = []struct {
	kind   doctor.Kind
	header string
}{
	{doctor.KindRedundantLink, "redundant links (sync removes them)"},
	{doctor.KindBrokenLink, "broken symlinks"},
	{doctor.KindUnknownEntry, "unknown entries (reported, never touched)"},
	{doctor.KindManualEdit, "manual edits fleet can't manage"},
	{doctor.KindDrift, "state drift"},
	{doctor.KindMissingDir, "missing directories"},
	{doctor.KindBrokenConfig, "unreadable configs"},
}

func printFindings(out io.Writer, findings []doctor.Finding) error {
	for _, section := range findingSections {
		var group []doctor.Finding
		for _, f := range findings {
			if f.Kind == section.kind {
				group = append(group, f)
			}
		}
		if len(group) == 0 {
			continue
		}
		if _, err := fmt.Fprintln(out, section.header+":"); err != nil {
			return err
		}
		for _, f := range group {
			line := f.Message
			if f.Harness != "" {
				line = f.Harness + ": " + line
			}
			if _, err := fmt.Fprintln(out, "  "+line); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveConflicts walks the manual-edit conflicts and applies whatever the
// user picks at each prompt. With no input (piped stdin, EOF) everything is
// reported and left as is. It returns how many conflicts were resolved.
func resolveConflicts(cmd *cobra.Command, p *paths.Paths, conflicts []doctor.Conflict) (int, error) {
	if len(conflicts) == 0 {
		return 0, nil
	}
	out := cmd.OutOrStdout()
	in := bufio.NewScanner(cmd.InOrStdin())
	inputEnded := false
	noted := false
	resolved := 0

	for _, c := range conflicts {
		if _, err := fmt.Fprintf(out, "\n%s: %s\n", c.Harness, c.Message); err != nil {
			return resolved, err
		}
		if _, err := fmt.Fprintf(out, "  [k] keep my change — %s\n", keepLabel(c)); err != nil {
			return resolved, err
		}
		if _, err := fmt.Fprintf(out, "  [r] restore — %s\n", restoreLabel(c)); err != nil {
			return resolved, err
		}
		if _, err := fmt.Fprintln(out, "  [s] skip — leave it as is"); err != nil {
			return resolved, err
		}

		if inputEnded || !in.Scan() {
			inputEnded = true
			note := "  left as is (no input)"
			if !noted {
				noted = true
				note += "; run `fleet skill doctor` from a terminal to resolve"
			}
			if _, err := fmt.Fprintln(out, note); err != nil {
				return resolved, err
			}
			continue
		}

		switch strings.ToLower(strings.TrimSpace(in.Text())) {
		case "k":
			if _, err := doctor.Resolve(p, c, true); err != nil {
				return resolved, err
			}
			if _, err := fmt.Fprintf(out, "  kept: %q recorded as %s for %s in the state file\n",
				c.Skill, keepState(c), c.Harness); err != nil {
				return resolved, err
			}
			resolved++
		case "r":
			rep, err := doctor.Resolve(p, c, false)
			if err != nil {
				return resolved, err
			}
			for _, ch := range rep.Changed {
				if _, err := fmt.Fprintf(out, "  restored: %s\n", restoreChange(c.Harness, ch)); err != nil {
					return resolved, err
				}
			}
			for _, f := range rep.Flags {
				if _, err := fmt.Fprintf(out, "  restored: %s: %s\n", c.Harness, f.Message); err != nil {
					return resolved, err
				}
			}
			if len(rep.Changed) == 0 && len(rep.Flags) == 0 {
				if _, err := fmt.Fprintf(out, "  restored: %s already matches the state\n", c.Harness); err != nil {
					return resolved, err
				}
			}
			resolved++
		default:
			if _, err := fmt.Fprintln(out, "  skipped"); err != nil {
				return resolved, err
			}
		}
	}
	return resolved, nil
}

// keepLabel describes what "keep my change" does, per conflict direction.
func keepLabel(c doctor.Conflict) string {
	if c.ConfigDisables {
		return "record the disable in fleet's state"
	}
	return "record the skill as enabled in fleet's state"
}

// restoreLabel describes what "restore" does, per conflict direction.
func restoreLabel(c doctor.Conflict) string {
	if c.ConfigDisables {
		return "sync fleet's state back into the config"
	}
	return "write the disable back into the config"
}

func keepState(c doctor.Conflict) string {
	if c.ConfigDisables {
		return "disabled"
	}
	return "enabled"
}

// restoreChange renders one applied restore in the sync report's shape.
func restoreChange(h string, c harness.Change) string {
	verb := "disabled"
	if c.To == harness.StateOn {
		verb = "enabled"
	}
	return fmt.Sprintf("%s: %s %q (was %s)", h, verb, c.Skill, c.From)
}

// printSummary closes the report: counts per kind, and what was resolved.
func printSummary(out io.Writer, rep doctor.Report, resolved int) error {
	if rep.Empty() {
		_, err := fmt.Fprintln(out, "\nno problems found")
		return err
	}

	counts := map[doctor.Kind]int{}
	for _, f := range rep.Findings {
		counts[f.Kind]++
	}
	var parts []string
	for _, section := range findingSections {
		if n := counts[section.kind]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, findingLabel(section.kind, n)))
		}
	}
	if len(rep.Conflicts) > 0 {
		parts = append(parts, fmt.Sprintf("%d manual edit%s to resolve", len(rep.Conflicts), plural(len(rep.Conflicts))))
	}
	if resolved > 0 {
		parts = append(parts, fmt.Sprintf("%d resolved", resolved))
	}
	_, err := fmt.Fprintln(out, "\n"+strings.Join(parts, ", "))
	return err
}

// findingLabel names a kind, pluralized to fit the count.
func findingLabel(kind doctor.Kind, n int) string {
	switch kind {
	case doctor.KindRedundantLink:
		return pluralized("redundant link", n)
	case doctor.KindBrokenLink:
		return pluralized("broken symlink", n)
	case doctor.KindUnknownEntry:
		if n == 1 {
			return "unknown entry"
		}
		return "unknown entries"
	case doctor.KindManualEdit:
		return pluralized("manual edit", n)
	case doctor.KindDrift:
		return pluralized("drift finding", n)
	case doctor.KindMissingDir:
		if n == 1 {
			return "missing directory"
		}
		return "missing directories"
	case doctor.KindBrokenConfig:
		return pluralized("unreadable config", n)
	}
	return string(kind)
}

func pluralized(singular string, n int) string {
	if n == 1 {
		return singular
	}
	return singular + "s"
}
