package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/snapshot"
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
	// Sync is buffered so a non-empty report can be followed by one blank
	// line, separating it from the listing on a terminal.
	var syncOut bytes.Buffer
	if err := runSyncTo(&syncOut, p); err != nil {
		return err
	}
	tty := stdoutIsTTY()
	if syncOut.Len() > 0 {
		errOut := cmd.ErrOrStderr()
		if _, err := errOut.Write(syncOut.Bytes()); err != nil {
			return err
		}
		if tty {
			if _, err := fmt.Fprintln(errOut); err != nil {
				return err
			}
		}
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
	header := shouldPrintHeader(asJSON, quiet, tty)
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

	pal := newPalette(stdoutIsTTY())

	if header {
		custom := 0
		outdated := 0
		for _, row := range report.Skills {
			if row.Custom {
				custom++
			}
			if row.Outdated != nil && *row.Outdated {
				outdated++
			}
		}
		summary := fmt.Sprintf("fleet · %d skills · %d installed · %d custom",
			len(report.Skills), len(report.Skills)-custom, custom)
		if outdated > 0 {
			summary += " · " + pal.warn(fmt.Sprintf("%d update%s", outdated, plural(outdated)))
		}
		if _, err := fmt.Fprintf(out, "%s\n\n", summary); err != nil {
			return err
		}
	}

	// The enablement columns come right after the name — they are the
	// table's point — and the description goes last, truncated to
	// whatever room the terminal has left, so no column ever wraps.
	states := make([][]string, len(report.Skills))
	for i, row := range report.Skills {
		states[i] = make([]string, len(report.Harnesses))
		for j, h := range report.Harnesses {
			states[i][j] = row.States[h]
		}
	}
	sources := make([]string, len(report.Skills))
	for i, row := range report.Skills {
		if row.Custom {
			sources[i] = "custom"
		} else {
			sources[i] = row.Source
		}
	}
	badges := make([]string, len(report.Skills))
	for i, row := range report.Skills {
		if row.Custom {
			badges[i] = "—" // never checked: custom by definition, not unknown
		} else {
			badges[i] = tableOutdated(row.Outdated)
		}
	}

	nameW := len("NAME")
	for _, row := range report.Skills {
		if n := len([]rune(row.Name)); n > nameW {
			nameW = n
		}
	}
	harnessHeaders := make([]string, len(report.Harnesses))
	harnessW := make([]int, len(report.Harnesses))
	for j, h := range report.Harnesses {
		harnessHeaders[j] = strings.ToUpper(h)
		harnessW[j] = len(harnessHeaders[j])
	}
	for j := range report.Harnesses {
		for i := range report.Skills {
			if n := len(tableState(states[i][j])); n > harnessW[j] {
				harnessW[j] = n
			}
		}
	}
	updateW := len("UPDATE")
	fullSourceW := len("SOURCE")
	for i := range report.Skills {
		if n := len([]rune(sources[i])); n > fullSourceW {
			fullSourceW = n
		}
	}
	fixed := nameW + 2 + colWidths(harnessW) + 2 + updateW
	flex := flexWidth(fixed, fullSourceW)
	sourceW, descW := flex.source, flex.desc
	if sourceW > fullSourceW || sourceW < 0 {
		sourceW = fullSourceW
	}
	if sourceW < fullSourceW {
		// Source is the last column and must fit exactly, or the terminal
		// wraps the row onto two lines.
		for i := range sources {
			sources[i] = truncate(sources[i], sourceW)
		}
	}

	headerCells := []string{padRight("NAME", nameW)}
	headerCells = append(headerCells, paddedCells(harnessHeaders, harnessW)...)
	headerCells = append(headerCells, padRight("UPDATE", updateW), padRight("SOURCE", sourceW))
	if descW > 0 {
		headerCells = append(headerCells, "DESCRIPTION")
	}
	styled := make([]string, len(headerCells))
	for i, cell := range headerCells {
		styled[i] = pal.dim(cell)
	}
	if _, err := fmt.Fprintln(out, strings.Join(styled, "  ")); err != nil {
		return err
	}

	for i, row := range report.Skills {
		line := padRight(row.Name, nameW)
		for j, state := range states[i] {
			line += "  " + colorState(padRight(tableState(state), harnessW[j]), pal)
		}
		line += "  " + colorBadge(padRight(badges[i], updateW), pal)
		line += "  " + colorSource(padRight(sources[i], sourceW), pal)
		if descW > 0 {
			line += "  " + truncate(row.Description, descW)
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

// colWidths sums a column-width list plus the two-space gutter between
// columns.
func colWidths(widths []int) int {
	total := 0
	for _, w := range widths {
		total += 2 + w
	}
	return total
}

// flexWidths carries the two flexible columns' widths. sourceFull means
// the source column takes its natural width.
type flexWidths struct {
	source int // -1 = natural width
	desc   int // 0 = no description column
}

// flexWidth splits the room left after the fixed columns between the
// source and description columns. Description first — it benefits most
// from room — and when it doesn't fit, source becomes the last column and
// is truncated to exactly what remains, so no row ever exceeds the
// terminal and wraps. Without a terminal: natural source, plain
// 60-column description cap.
func flexWidth(fixed, sourceW int) flexWidths {
	w := stdoutWidth()
	if w <= 0 {
		return flexWidths{source: -1, desc: descriptionCap}
	}
	room := w - fixed
	if desc := min(80, room-2-sourceW-2-len("DESCRIPTION")); desc >= 15 {
		return flexWidths{source: -1, desc: desc}
	}
	if s := room - 2; s >= 8 {
		if s > sourceW {
			s = sourceW // slack stays unused; no reason to pad the last column
		}
		return flexWidths{source: s, desc: 0}
	}
	// Even the fixed columns overflow; nothing sensible to truncate.
	return flexWidths{source: -1, desc: 0}
}

// colorState styles an enablement cell: on green, off and absent dim —
// the TUI's same reading: off is the quiet state, not an error.
func colorState(state string, pal palette) string {
	if state == "on" {
		return pal.good(state)
	}
	return pal.dim(state)
}

// colorBadge styles the update badge: ↑ available in yellow, ✓ current in
// green, ? unknown and — never-checked dim.
func colorBadge(badge string, pal palette) string {
	switch badge {
	case "↑":
		return pal.warn(badge)
	case "✓":
		return pal.good(badge)
	default:
		return pal.dim(badge)
	}
}

// colorSource styles the source column: custom in cyan, an installed
// skill's source repo dim.
func colorSource(source string, pal palette) string {
	if source == "custom" {
		return pal.info(source)
	}
	return pal.dim(source)
}

// stdoutWidth returns the terminal width of stdout, or 0 when stdout
// isn't a terminal (or the size can't be had). Indirect so tests can stub.
var stdoutWidth = func() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		return 0
	}
	return w
}

// paddedCells pads each cell to its column's width.
func paddedCells(cells []string, widths []int) []string {
	out := make([]string, len(cells))
	for i, cell := range cells {
		out[i] = padRight(cell, widths[i])
	}
	return out
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
