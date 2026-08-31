package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/zacong/fleet/internal/harness"
	"github.com/zacong/fleet/internal/paths"
	"github.com/zacong/fleet/internal/scan"
)

// descriptionCap caps the description column; --json keeps the full text.
const descriptionCap = 60

// skillRow is one skill as `fleet skill ls` reports it, in both the table
// and the --json output.
type skillRow struct {
	Name        string            `json:"name"`
	Custom      bool              `json:"custom"`
	Source      string            `json:"source,omitempty"`
	SourceType  string            `json:"sourceType,omitempty"`
	Hash        string            `json:"hash,omitempty"`
	InstalledAt string            `json:"installedAt,omitempty"`
	UpdatedAt   string            `json:"updatedAt,omitempty"`
	Description string            `json:"description,omitempty"`
	States      map[string]string `json:"states"`
}

// lsReport is the full picture: the installed harnesses found on this
// machine, and every skill in the canonical store.
type lsReport struct {
	Harnesses []string   `json:"harnesses"`
	Skills    []skillRow `json:"skills"`
}

func newSkillLsCmd(p *paths.Paths) *cobra.Command {
	var asJSON, quiet bool

	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List every skill and where it is active",
		Long: "List every skill in the canonical store, marked custom or installed and grouped by source repo, with a per-harness on/off column for each installed harness.\n" +
			"\nSync runs first: the state file is projected into each harness config, and entries fleet doesn't recognize are reported on stderr, never touched.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Sync runs on every command: converge the harness configs
			// with the state file before reporting what they say. Findings
			// go to stderr so stdout stays machine-readable.
			if err := runSyncTo(cmd.ErrOrStderr(), p); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			report, err := buildReport(p)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(out, report)
			}
			header := shouldPrintHeader(asJSON, quiet, stdoutIsTTY())
			return printTable(out, report, header)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON instead of a table")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "omit the summary line")
	return cmd
}

// buildReport scans the canonical store and every installed harness's own
// config. It reads only; nothing is written anywhere.
func buildReport(p *paths.Paths) (*lsReport, error) {
	skills, err := scan.ScanStore(p.SkillsStore())
	if err != nil {
		return nil, fmt.Errorf("scan canonical store: %w", err)
	}
	lock, err := scan.ReadLockfile(p.SkillLock())
	if err != nil {
		return nil, fmt.Errorf("read skills lockfile: %w", err)
	}

	var installed []harness.Adapter
	for _, a := range harness.All(p) {
		if a.Installed() {
			installed = append(installed, a)
		}
	}

	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.Name
	}
	reads := make([]harness.ReadResult, len(installed))
	for i, a := range installed {
		read, err := a.Read(names)
		if err != nil {
			return nil, fmt.Errorf("read %s config: %w", a.Harness(), err)
		}
		reads[i] = read
	}

	harnessNames := make([]string, len(installed))
	for i, a := range installed {
		harnessNames[i] = string(a.Harness())
	}

	rows := make([]skillRow, len(skills))
	for i, s := range skills {
		states := make(map[string]string, len(installed))
		for j := range installed {
			state := reads[j].States[s.Name]
			if state == "" {
				state = harness.StateOn // adapters report every name; be defensive anyway
			}
			states[harnessNames[j]] = string(state)
		}

		row := skillRow{
			Name:        s.Name,
			Description: s.Description,
			States:      states,
		}
		if prov, ok := lock[s.Dir]; ok {
			row.Source = prov.Source
			row.SourceType = prov.SourceType
			row.Hash = prov.Hash
			row.InstalledAt = prov.InstalledAt
			row.UpdatedAt = prov.UpdatedAt
		} else {
			row.Custom = true
		}
		rows[i] = row
	}

	// Custom first, then installed grouped by source repo, alphabetical
	// inside each group.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Custom != rows[j].Custom {
			return rows[i].Custom
		}
		if rows[i].Source != rows[j].Source {
			return rows[i].Source < rows[j].Source
		}
		return rows[i].Name < rows[j].Name
	})

	return &lsReport{Harnesses: harnessNames, Skills: rows}, nil
}

func printJSON(out io.Writer, report *lsReport) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func printTable(out io.Writer, report *lsReport, header bool) error {
	if len(report.Skills) == 0 {
		_, err := fmt.Fprintf(out, "no skills found\n")
		return err
	}

	if header {
		custom := 0
		for _, row := range report.Skills {
			if row.Custom {
				custom++
			}
		}
		if _, err := fmt.Fprintf(out, "fleet · %d skills · %d installed · %d custom\n\n",
			len(report.Skills), len(report.Skills)-custom, custom); err != nil {
			return err
		}
	}

	tw := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)

	cells := []string{"NAME", "ORIGIN", "SOURCE", "DESCRIPTION"}
	for _, h := range report.Harnesses {
		cells = append(cells, strings.ToUpper(h))
	}
	if _, err := fmt.Fprintln(tw, strings.Join(cells, "\t")); err != nil {
		return err
	}

	for _, row := range report.Skills {
		origin := "installed"
		source := row.Source
		if row.Custom {
			origin = "custom"
			source = "-"
		}
		cells := []string{row.Name, origin, source, truncate(row.Description, descriptionCap)}
		for _, h := range report.Harnesses {
			cells = append(cells, tableState(row.States[h]))
		}
		if _, err := fmt.Fprintln(tw, strings.Join(cells, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// tableState renders a state for the table; absent skills get a bare dash.
func tableState(state string) string {
	if state == string(harness.StateAbsent) || state == "" {
		return "-"
	}
	return state
}

// truncate shortens s to at most max runes, marking the cut with an
// ellipsis.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// shouldPrintHeader decides whether the one-line summary prints: not for
// --json (scripts) and not when piped or --quiet.
func shouldPrintHeader(asJSON, quiet, tty bool) bool {
	return !asJSON && !quiet && tty
}

// stdoutIsTTY reports whether fleet's stdout is a terminal. Tests and pipes
// suppress the summary line automatically. Indirect through stdoutTTY so
// tests can stub it.
func stdoutIsTTY() bool { return stdoutTTY() }

var stdoutTTY = func() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
