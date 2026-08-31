// The on/off commands: record the toggle in the state file, then let sync
// project it into each harness's native config. Sync runs on every
// command; here its output is the command's own report, so it goes to
// stdout. Cursor and Bob have no write side — toggling for them is a no-op
// with a clear message.

package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zacong/fleet/internal/harness"
	"github.com/zacong/fleet/internal/paths"
	"github.com/zacong/fleet/internal/scan"
	"github.com/zacong/fleet/internal/state"
	fleetsync "github.com/zacong/fleet/internal/sync"
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

			st, err := state.Load(p.FleetStateFile())
			if err != nil {
				return err
			}
			for _, a := range targets {
				if !a.CanProject() {
					continue // nothing to record for harnesses fleet can't write
				}
				if on {
					st.SetEnabled(name, string(a.Harness()))
				} else {
					st.SetDisabled(name, string(a.Harness()))
				}
			}
			if err := state.Save(p.FleetStateFile(), st); err != nil {
				return fmt.Errorf("save state: %w", err)
			}

			out := cmd.OutOrStdout()

			// Enabling must strip fleet's own markers before ambient sync
			// runs: the state entry is already gone, so sync would
			// otherwise flag the still-present markers as untracked.
			// Disabling needs no explicit step — sync below projects it.
			if on {
				for _, a := range targets {
					if !a.CanProject() {
						continue
					}
					rep, err := a.Project([]harness.SkillWrite{{Name: name, State: harness.StateOn}})
					if err != nil {
						return fmt.Errorf("enable %s in %s: %w", name, a.Harness(), err)
					}
					if err := printReport(out, string(a.Harness()), rep.Changed, rep.Flags); err != nil {
						return err
					}
				}
			}

			// Sync: repair any other drift while we are here.
			if err := runSyncTo(out, p); err != nil {
				return err
			}

			for _, a := range targets {
				if !a.CanProject() {
					verb := "disable"
					if on {
						verb = "enable"
					}
					if _, err := fmt.Fprintf(out, "%s: no per-skill disable mechanism — %s %q is a no-op\n", a.Harness(), verb, name); err != nil {
						return err
					}
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
		var installed []harness.Adapter
		for _, a := range all {
			if a.Installed() {
				installed = append(installed, a)
			}
		}
		return installed, nil
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

// runSyncTo runs sync and writes the reports in human form.
func runSyncTo(out io.Writer, p *paths.Paths) error {
	reports, err := fleetsync.Run(p)
	if err != nil {
		return err
	}
	for _, r := range reports {
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
