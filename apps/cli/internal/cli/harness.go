package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
)

func newHarnessCmd(p *paths.Paths) *cobra.Command {
	harnessCmd := &cobra.Command{
		Use:   "harness",
		Short: "Inspect the harnesses fleet manages",
	}
	harnessCmd.AddCommand(newHarnessLsCmd(p))
	return harnessCmd
}

func newHarnessLsCmd(p *paths.Paths) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List the supported harnesses and which are installed",
		Long: "List the six harnesses fleet supports, marking each installed (its config directory exists) or not, with the directory itself and whether fleet can write a per-skill off switch into its config.\n" +
			"\nRead-only: unlike most fleet commands, no sync runs — listing what is installed must not converge anything. Cursor and Bob have no per-skill off switch; fleet says so instead of pretending.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			rows := buildHarnessRows(p)
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

// harnessRow is one supported harness's status on this machine. The
// adapter supplies everything; nothing is guessed from paths.
type harnessRow struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	ConfigDir string `json:"configDir"`
	CanOff    bool   `json:"canDisable"`
}

func buildHarnessRows(p *paths.Paths) []harnessRow {
	var rows []harnessRow
	for _, a := range harness.All(p) {
		rows = append(rows, harnessRow{
			Name:      string(a.Harness()),
			Installed: a.Installed(),
			ConfigDir: a.Dir(),
			CanOff:    a.CanProject(),
		})
	}
	return rows
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
	if _, err := fmt.Fprintln(tw, "NAME\tINSTALLED\tCONFIG DIR\tOFF SWITCH"); err != nil {
		return err
	}
	for _, r := range rows {
		cells := []string{r.Name, harnessCheck(r.Installed), r.ConfigDir, yesNo(r.CanOff)}
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
// pretending (Cursor and Bob have no off switch).
func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
