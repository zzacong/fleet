package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/config"
	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
)

func newHarnessCmd(p *paths.Paths) *cobra.Command {
	harnessCmd := &cobra.Command{
		Use:   "harness",
		Short: "Inspect and hide the harnesses fleet manages",
	}
	harnessCmd.AddCommand(newHarnessLsCmd(p), newHarnessHideCmd(p), newHarnessUnhideCmd(p))
	return harnessCmd
}

func newHarnessLsCmd(p *paths.Paths) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List the supported harnesses and which are installed",
		Long: "List the six harnesses fleet supports, marking each installed (its config directory exists), whether it is hidden, and whether fleet can write a per-skill off switch into its config.\n" +
			"\nRead-only: unlike most fleet commands, no sync runs — listing what is installed must not converge anything. Cursor and Bob have no config off switch; a custom skill is disabled there by removing its managed link, which the toggle verbs do instead.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			rows, err := buildHarnessRows(p)
			if err != nil {
				return err
			}
			if asJSON {
				return printHarnessJSON(out, rows)
			}
			header := shouldPrintHeader(asJSON, false, stdoutIsTTY())
			return printHarnessTable(out, rows, header)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON instead of a table")
	return cmd
}

// newHarnessHideCmd hides an installed harness from fleet.
func newHarnessHideCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "hide <harness>",
		Short: "Hide an installed harness from fleet",
		Long:  "Hide an installed harness on this machine. Fleet stops showing it and writes nothing into it: no links, toggles, doctor findings, or prune. Existing links and state entries stay on disk; unhide brings them back. Only a supported, installed harness can be hidden.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setHarnessHidden(cmd.OutOrStdout(), p, args[0], true)
		},
	}
}

// newHarnessUnhideCmd brings a hidden harness back under fleet's management.
func newHarnessUnhideCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "unhide <harness>",
		Short: "Bring a hidden harness back under fleet's management",
		Long:  "Clear a harness's hidden mark and sync, which restores its links and state projections on this machine.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setHarnessHidden(cmd.OutOrStdout(), p, args[0], false)
		},
	}
}

// setHarnessHidden records or clears a hidden harness in the fleet config,
// then syncs when something changed. Hiding refuses a name that is not a
// supported harness, or one that is not installed, so a typo never leaves a
// dead key behind. Both directions are idempotent and say so.
func setHarnessHidden(out io.Writer, p *paths.Paths, name string, hide bool) error {
	adapters := harness.All(p)
	if hide {
		target, ok := adapterNamed(adapters, name)
		if !ok {
			return fmt.Errorf("unknown harness %q (want one of: %s)", name, harnessList(adapters))
		}
		if !target.Installed() {
			return fmt.Errorf("%s is not installed on this machine", name)
		}
	}

	f, err := config.Load(p.FleetConfigFile())
	if err != nil {
		return err
	}
	var changed bool
	var noop, done string
	if hide {
		changed = f.Hide(name)
		noop = name + " is already hidden"
		done = name + " is now hidden; fleet ignores it until `fleet harness unhide " + name + "`"
	} else {
		changed = f.Unhide(name)
		noop = name + " is not hidden"
		done = name + " is managed again"
	}
	if !changed {
		_, err := fmt.Fprintln(out, noop)
		return err
	}
	if err := config.Save(p.FleetConfigFile(), f); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, done); err != nil {
		return err
	}
	return runSyncTo(out, p)
}

// adapterNamed returns the adapter for a supported harness name.
func adapterNamed(adapters []harness.Adapter, name string) (harness.Adapter, bool) {
	for _, a := range adapters {
		if string(a.Harness()) == name {
			return a, true
		}
	}
	return nil, false
}

// harnessRow is one supported harness's status on this machine. The
// adapter supplies detection and capability; the hidden mark comes from the
// fleet config.
type harnessRow struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Hidden    bool   `json:"hidden"`
	ConfigDir string `json:"configDir"`
	CanOff    bool   `json:"canDisable"`
}

func buildHarnessRows(p *paths.Paths) ([]harnessRow, error) {
	hidden, err := harness.HiddenNames(p)
	if err != nil {
		return nil, err
	}
	var rows []harnessRow
	for _, a := range harness.All(p) {
		rows = append(rows, harnessRow{
			Name:      string(a.Harness()),
			Installed: a.Installed(),
			Hidden:    slices.Contains(hidden, string(a.Harness())),
			ConfigDir: a.Dir(),
			CanOff:    a.CanProject(),
		})
	}
	return rows, nil
}

func printHarnessJSON(out io.Writer, rows []harnessRow) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(struct {
		Harnesses []harnessRow `json:"harnesses"`
	}{rows})
}

func printHarnessTable(out io.Writer, rows []harnessRow, header bool) error {
	if header {
		installed := 0
		for _, r := range rows {
			if r.Installed {
				installed++
			}
		}
		summary := fmt.Sprintf("fleet · %d of %d harnesses installed", installed, len(rows))
		if _, err := fmt.Fprintf(out, "%s\n\n", summary); err != nil {
			return err
		}
	}

	tw := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tINSTALLED\tHIDDEN\tCONFIG DIR\tOFF SWITCH"); err != nil {
		return err
	}
	for _, r := range rows {
		cells := []string{r.Name, harnessCheck(r.Installed), yesNo(r.Hidden), r.ConfigDir, yesNo(r.CanOff)}
		if _, err := fmt.Fprintln(tw, strings.Join(cells, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// harnessCheck renders the installed column: ✓ present, – not found.
func harnessCheck(installed bool) string {
	if installed {
		return "✓"
	}
	return "-"
}

// yesNo renders a capability flag for the table: fleet says so instead of
// pretending (Cursor and Bob have no config off switch).
func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
