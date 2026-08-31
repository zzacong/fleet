package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/zacong/fleet/internal/harness"
	"github.com/zacong/fleet/internal/paths"
	"github.com/zacong/fleet/internal/snapshot"
)

// descriptionCap caps the description column; --json keeps the full text.
const descriptionCap = 60

func newSkillLsCmd(p *paths.Paths) *cobra.Command {
	var asJSON, quiet bool

	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List every skill and where it is active",
		Long: "List every skill — the canonical store's, plus the fleet repo's custom skills when run inside the repo — marked custom or installed and grouped by source repo, with a per-harness on/off column for each installed harness.\n" +
			"\nSync runs first: the state file is projected into each harness config, and entries fleet doesn't recognize are reported on stderr, never touched.\n" +
			"\n" +
			"UPDATE marks skills whose source repo has moved on: ↑ update available, ✓ current, ? unknown. Fleet checks the skills CLI lockfile's recorded hash against GitHub's current tree hash for the skill folder — one API call per source repo, cached for an hour so repeated runs don't hammer the API. Custom skills and non-GitHub sources are always ?, never guessed; a failed check degrades to ? without failing the command.\n" +
			"\n" +
			"--json carries the same badge as the tri-state \"outdated\" field: true = update available, false = current, null = unknown.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListing(cmd, p, asJSON, quiet)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON instead of a table")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "omit the summary line")
	return cmd
}

// runListing is the ls body, shared with bare `fleet`'s piped fallback:
// sync, then the snapshot, then the table (or JSON). Findings go to stderr
// so stdout stays machine-readable.
func runListing(cmd *cobra.Command, p *paths.Paths, asJSON, quiet bool) error {
	if err := runSyncTo(cmd.ErrOrStderr(), p); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	report, warnings, err := buildReport(cmd.Context(), p)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		// Best-effort visibility: a stderr write failure must not fail a
		// command whose real output already succeeded.
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w)
	}
	if asJSON {
		return printJSON(out, report)
	}
	header := shouldPrintHeader(asJSON, quiet, stdoutIsTTY())
	return printTable(out, report, header)
}

// newTreeClient builds the update-check client: the real GitHub API behind
// the persistent TTL cache in fleet's config dir. Tests swap it for a
// scripted stub.
var newTreeClient = snapshot.DefaultTreeClient

// buildReport snapshots the world the ls table renders. The scan, read,
// grouping, and badge logic live in internal/snapshot — the TUI renders
// the same picture through the same code.
func buildReport(ctx context.Context, p *paths.Paths) (*snapshot.Report, []string, error) {
	return snapshot.Build(ctx, p, newTreeClient(p))
}

func printJSON(out io.Writer, report *snapshot.Report) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func printTable(out io.Writer, report *snapshot.Report, header bool) error {
	if len(report.Skills) == 0 {
		_, err := fmt.Fprintf(out, "no skills found\n")
		return err
	}

	if header {
		custom := 0
		stale := 0
		for _, row := range report.Skills {
			if row.Custom {
				custom++
			}
			if row.Outdated != nil && *row.Outdated {
				stale++
			}
		}
		summary := fmt.Sprintf("fleet · %d skills · %d installed · %d custom",
			len(report.Skills), len(report.Skills)-custom, custom)
		if stale > 0 {
			summary += fmt.Sprintf(" · %d update%s", stale, plural(stale))
		}
		if _, err := fmt.Fprintf(out, "%s\n\n", summary); err != nil {
			return err
		}
	}

	tw := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)

	cells := []string{"NAME", "ORIGIN", "SOURCE", "UPDATE", "DESCRIPTION"}
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
		cells := []string{row.Name, origin, source, tableOutdated(row.Outdated), truncate(row.Description, descriptionCap)}
		for _, h := range report.Harnesses {
			cells = append(cells, tableState(row.States[h]))
		}
		if _, err := fmt.Fprintln(tw, strings.Join(cells, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// tableOutdated renders the tri-state badge for the table: ↑ update
// available, ✓ current, ? unknown (custom skills, non-GitHub sources, and
// failed checks — fleet never guesses).
func tableOutdated(outdated *bool) string {
	switch {
	case outdated == nil:
		return "?"
	case *outdated:
		return "↑"
	default:
		return "✓"
	}
}

// plural suffixes a count in the summary line.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
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
