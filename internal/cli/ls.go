package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/zacong/fleet/internal/harness"
	"github.com/zacong/fleet/internal/outdated"
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
	// Outdated is the tri-state update badge: true = update available,
	// false = current, null = unknown (custom skills, non-GitHub sources,
	// and failed checks are unknown — fleet never guesses). Always present
	// in the JSON so scripts can rely on the key.
	Outdated *bool `json:"outdated"`
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
		Long: "List every skill — the canonical store's, plus the fleet repo's custom skills when run inside the repo — marked custom or installed and grouped by source repo, with a per-harness on/off column for each installed harness.\n" +
			"\nSync runs first: the state file is projected into each harness config, and entries fleet doesn't recognize are reported on stderr, never touched.\n" +
			"\n" +
			"UPDATE marks skills whose source repo has moved on: ↑ update available, ✓ current, ? unknown. Fleet checks the skills CLI lockfile's recorded hash against GitHub's current tree hash for the skill folder — one API call per source repo, cached for an hour so repeated runs don't hammer the API. Custom skills and non-GitHub sources are always ?, never guessed; a failed check degrades to ? without failing the command.\n" +
			"\n" +
			"--json carries the same badge as the tri-state \"outdated\" field: true = update available, false = current, null = unknown.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Sync runs on every command: converge the harness configs
			// with the state file before reporting what they say. Findings
			// go to stderr so stdout stays machine-readable.
			if err := runSyncTo(cmd.ErrOrStderr(), p); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			report, warnings, err := buildReport(cmd.Context(), p)
			if err != nil {
				return err
			}
			for _, w := range warnings {
				// Best-effort visibility: a stderr write failure must
				// not fail a command whose real output already succeeded.
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w)
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

// newTreeClient builds the update-check client: the real GitHub API behind
// the persistent TTL cache in fleet's config dir. Tests swap it for a
// scripted stub.
var newTreeClient = func(p *paths.Paths) outdated.TreeClient {
	return outdated.NewCachingClient(outdated.NewHTTPClient(""), filepath.Join(p.FleetConfigDir(), "tree-cache.json"))
}

// buildReport scans the canonical store and the fleet repo's custom
// skills, then every installed harness's own config, and classifies each
// installed skill's update state. It writes only fleet's own cache;
// nothing else is touched. Check failures come back as warnings, not
// errors.
func buildReport(ctx context.Context, p *paths.Paths) (*lsReport, []string, error) {
	storeSkills, err := scan.ScanStore(p.SkillsStore())
	if err != nil {
		return nil, nil, fmt.Errorf("scan canonical store: %w", err)
	}
	// Repo customs join the list; custom means lives in the repo. A name
	// present in both places reports once, from the canonical store — the
	// double-visibility is drift for doctor to flag, not ls's job.
	repoNames := map[string]bool{}
	skills := storeSkills
	if p.Repo != "" {
		repoSkills, err := scan.ScanStore(p.RepoSkills())
		if err != nil {
			return nil, nil, fmt.Errorf("scan repo skills: %w", err)
		}
		seen := make(map[string]bool, len(storeSkills))
		for _, s := range storeSkills {
			seen[s.Name] = true
		}
		for _, s := range repoSkills {
			if seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			repoNames[s.Name] = true
			skills = append(skills, s)
		}
	}
	lock, err := scan.ReadLockfile(p.SkillLock())
	if err != nil {
		return nil, nil, fmt.Errorf("read skills lockfile: %w", err)
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
			return nil, nil, fmt.Errorf("read %s config: %w", a.Harness(), err)
		}
		reads[i] = read
	}

	harnessNames := make([]string, len(installed))
	for i, a := range installed {
		harnessNames[i] = string(a.Harness())
	}

	// One update check for every skill in the store, grouped inside the
	// checker: installed skills carry their lockfile provenance, customs
	// stay the zero entry (unknown by definition, no API call). Repo
	// customs are skipped entirely — a lingering lock entry from a
	// pre-adoption install must not make fleet check (or misreport) a
	// skill that now lives in the repo.
	entries := make(map[string]outdated.Entry, len(skills))
	for _, s := range skills {
		if repoNames[s.Name] {
			continue
		}
		if prov, ok := lock[s.Dir]; ok {
			entries[s.Dir] = outdated.Entry{
				Source:     prov.Source,
				SourceType: prov.SourceType,
				SkillPath:  prov.SkillPath,
				Ref:        prov.Ref,
				Hash:       prov.Hash,
			}
		} else {
			entries[s.Dir] = outdated.Entry{}
		}
	}
	check := outdated.Check(ctx, newTreeClient(p), entries)

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
			Outdated:    outdatedPtr(check.Statuses[s.Dir]),
		}
		if repoNames[s.Name] {
			// Custom by definition: it lives in the repo, unversioned by
			// the skills CLI. A lingering lock entry under the same
			// directory name is stale provenance, not provenance.
			row.Custom = true
		} else if prov, ok := lock[s.Dir]; ok {
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

	return &lsReport{Harnesses: harnessNames, Skills: rows}, check.Warnings, nil
}

// outdatedPtr renders the tri-state for JSON: outdated true, current
// false, unknown null.
func outdatedPtr(s outdated.Status) *bool {
	var v bool
	switch s {
	case outdated.StatusOutdated:
		v = true
	case outdated.StatusCurrent:
		v = false
	default:
		return nil
	}
	return &v
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
