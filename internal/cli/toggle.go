// The on/off commands: record the toggle in the state file, then let sync
// project it into each harness's native config. The write sequence itself
// lives in internal/toggle — the TUI's staged apply runs the same code.
// Sync's output is the command's own report, so it goes to stdout. Cursor
// and Bob have no write side — toggling for them is a no-op with a clear
// message.

package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/scan"
	fleetsync "github.com/zzacong/fleet/internal/sync"
	"github.com/zzacong/fleet/internal/toggle"
)

func newSkillOnCmd(p *paths.Paths) *cobra.Command  { return newSkillToggleCmd(p, true) }
func newSkillOffCmd(p *paths.Paths) *cobra.Command { return newSkillToggleCmd(p, false) }

func newSkillToggleCmd(p *paths.Paths, on bool) *cobra.Command {
	var harnessFlags []string

	verb := "off"
	short := "Disable a skill for one harness or all installed ones"
	long := "Disable a skill for the given harnesses (--harness, repeatable) or for every installed harness, writing each harness's own native off setting.\n\n" +
		"The state file records the toggle; sync projects it. The skill's files stay in the canonical store."
	if on {
		verb = "on"
		short = "Enable a skill for one harness or all installed ones"
		long = "Enable a skill for the given harnesses (--harness, repeatable) or for every installed harness, removing fleet's disable entries.\n\n" +
			"The state file records the toggle; sync projects it."
	}

	cmd := &cobra.Command{
		Use:   verb + " <name>",
		Short: short,
		Long:  long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			targets, err := resolveTargets(p, harnessFlags)
			if err != nil {
				return err
			}
			if !on {
				// Disabling a typo'd name would silently record state; the
				// skill must exist in the canonical store. Enabling is
				// lenient: it also cleans up entries for skills that were
				// uninstalled while disabled.
				if err := requireStoredSkill(p, name); err != nil {
					return err
				}
			}

			// Harnesses without a write side get their no-op message; they
			// contribute no toggle (and no state entry).
			var toggles []toggle.Toggle
			var nowrite []harness.Adapter
			for _, a := range targets {
				if !a.CanProject() {
					nowrite = append(nowrite, a)
					continue
				}
				toggles = append(toggles, toggle.Toggle{Name: name, Harness: string(a.Harness()), On: on})
			}

			_, projected, reports, err := toggle.Apply(p, toggles)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			for _, pr := range projected {
				if err := printReport(out, string(pr.Harness), pr.Report.Changed, pr.Report.Flags); err != nil {
					return err
				}
			}
			if err := printSyncReports(out, reports); err != nil {
				return err
			}

			for _, a := range nowrite {
				verb := "disable"
				if on {
					verb = "enable"
				}
				if _, err := fmt.Fprintf(out, "%s: no per-skill disable mechanism — %s %q is a no-op\n", a.Harness(), verb, name); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&harnessFlags, "harness", nil, "target only this harness (repeatable); default: all installed harnesses")
	return cmd
}

// resolveTargets validates --harness values and returns the adapters they
// name; without the flag, every installed harness is targeted.
func resolveTargets(p *paths.Paths, harnessFlags []string) ([]harness.Adapter, error) {
	all := harness.All(p)

	if len(harnessFlags) == 0 {
		return harness.Installed(p), nil
	}

	seen := map[string]bool{}
	var targets []harness.Adapter
	for _, flag := range harnessFlags {
		if seen[flag] {
			continue
		}
		seen[flag] = true
		var match harness.Adapter
		for _, a := range all {
			if string(a.Harness()) == flag {
				match = a
				break
			}
		}
		if match == nil {
			return nil, fmt.Errorf("unknown harness %q (want one of: %s)", flag, harnessList(all))
		}
		if !match.Installed() {
			return nil, fmt.Errorf("%s is not installed on this machine", flag)
		}
		targets = append(targets, match)
	}
	return targets, nil
}

func harnessList(adapters []harness.Adapter) string {
	names := make([]string, len(adapters))
	for i, a := range adapters {
		names[i] = string(a.Harness())
	}
	return strings.Join(names, ", ")
}

// requireStoredSkill fails when the named skill is not in the canonical
// store.
func requireStoredSkill(p *paths.Paths, name string) error {
	skills, err := scan.ScanStore(p.SkillsStore())
	if err != nil {
		return fmt.Errorf("scan canonical store: %w", err)
	}
	for _, s := range skills {
		if s.Name == name {
			return nil
		}
	}
	return fmt.Errorf("skill %q not found in %s", name, p.SkillsStore())
}

// runSyncTo runs sync and writes the reports in human form: link removals
// first, then enablement changes and flags.
func runSyncTo(out io.Writer, p *paths.Paths) error {
	reports, err := fleetsync.Run(p)
	if err != nil {
		return err
	}
	return printSyncReports(out, reports)
}

// printSyncReports writes sync's reports in human form: link removals
// first, then enablement changes and flags.
func printSyncReports(out io.Writer, reports []fleetsync.Report) error {
	for _, r := range reports {
		for _, e := range r.Removed {
			if _, err := fmt.Fprintf(out, "sync: %s: removed redundant link %q — %s\n", r.Harness, e.Name, e.Reason); err != nil {
				return err
			}
		}
		if err := printReport(out, r.Harness, r.Changed, r.Flags); err != nil {
			return err
		}
	}
	return nil
}

func printReport(out io.Writer, harnessName string, changed []harness.Change, flags []harness.Flag) error {
	for _, c := range changed {
		if _, err := fmt.Fprintln(out, formatChange(harnessName, c)); err != nil {
			return err
		}
	}
	for _, f := range flags {
		if _, err := fmt.Fprintln(out, formatFlag(harnessName, f)); err != nil {
			return err
		}
	}
	return nil
}

// formatChange renders one applied flip: "sync: opencode: disabled \"tdd\"
// (was on)".
func formatChange(harnessName string, c harness.Change) string {
	verb := "disabled"
	if c.To == harness.StateOn {
		verb = "enabled"
	}
	return fmt.Sprintf("sync: %s: %s %q (was %s)", harnessName, verb, c.Skill, c.From)
}

// formatFlag renders one untouched-but-flagged entry, attributed to the
// skill when there is one: "sync: pi/tdd: still excluded by a !glob entry
// — left alone".
func formatFlag(harnessName string, f harness.Flag) string {
	scope := harnessName
	if f.Skill != "" {
		scope = harnessName + "/" + f.Skill
	}
	return fmt.Sprintf("sync: %s: %s", scope, f.Message)
}
