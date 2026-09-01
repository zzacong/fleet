// The doctor command: the read-only report of what's wrong. It does not
// run ambient sync — the point is to surface everything sync would change
// before sync changes it. By default it reports everything, conflicts
// included, and touches nothing. With --interactive it walks each
// manual-edit conflict as a prompt: keep adopts the edit into the state
// file, restore syncs the state back into the config. Unknown entries and
// unmanageable rules are reported and never touched either way.

package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/doctor"
	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
)

func newSkillDoctorCmd(p *paths.Paths) *cobra.Command {
	var interactive bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report what's wrong: drift, redundant links, broken links, unknown entries, manual edits",
		Long: "Inspect every installed harness, the canonical store, and the fleet repo's skills, and report what is wrong: " +
			"redundant per-agent links, broken symlinks, unknown entries in skills dirs, " +
			"a skill name present in both the store and the repo, stale lockfile entries for adopted skills, " +
			"missing directories, and manual config edits that disagree with the state file.\n\n" +
			"Doctor is read-only: it reports without changing anything, so you see what sync would " +
			"do before sync does it (sync runs on every other command).\n\n" +
			"By default everything is reported at once and nothing is asked — manual-edit conflicts " +
			"are listed with their options and left as is. Pass --interactive to walk each conflict " +
			"as a prompt: \"keep my change\" records the edit in the state file, \"restore\" syncs " +
			"the state back into the config, and skipping changes nothing.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			pal := newDoctorPalette(stdoutIsTTY())

			rep, err := doctor.Analyze(p)
			if err != nil {
				return err
			}

			if err := printFindings(out, rep.Findings, pal); err != nil {
				return err
			}
			resolved := 0
			if interactive {
				resolved, err = resolveConflicts(cmd, p, rep.Conflicts, pal)
			} else {
				err = reportConflicts(out, rep.Conflicts, pal)
			}
			if err != nil {
				return err
			}
			return printSummary(out, rep, resolved, interactive, pal)
		},
	}
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false,
		"resolve each manual-edit conflict with a keep/restore prompt instead of reporting it")
	return cmd
}

// findingSections fixes the report's section order and presentation:
// title for the header, note for the dim annotation after the count, and
// severity for the icon and color.
var findingSections = []struct {
	kind  doctor.Kind
	title string
	note  string
	sev   string // "broken", "warn", or "info"
}{
	{doctor.KindRedundantLink, "redundant links", "sync removes them", "warn"},
	{doctor.KindBrokenLink, "broken symlinks", "", "broken"},
	{doctor.KindUnknownEntry, "unknown entries", "reported, never touched", "info"},
	{doctor.KindManualEdit, "manual edits fleet can't manage", "", "warn"},
	{doctor.KindDrift, "state drift", "", "warn"},
	{doctor.KindDoublePresence, "double presence (store and repo)", "", "warn"},
	{doctor.KindStaleLock, "stale lockfile entries", "fleet never writes the lockfile", "warn"},
	{doctor.KindMissingDir, "missing directories", "", "warn"},
	{doctor.KindBrokenConfig, "unreadable configs", "", "broken"},
}

// doctorPalette holds doctor's text styles. When stdout isn't a terminal
// every style is the identity, so pipes and captures get clean plain
// text — the same report, no escapes.
type doctorPalette struct {
	broken func(string) string // red — needs fixing before anything works
	warn   func(string) string // yellow — sync or the user should act
	info   func(string) string // cyan — informational, nothing to do
	good   func(string) string // green
	dim    func(string) string // annotations, legends
	bold   func(string) string
}

func newDoctorPalette(tty bool) doctorPalette {
	identity := func(s string) string { return s }
	if !tty {
		return doctorPalette{broken: identity, warn: identity, info: identity, good: identity, dim: identity, bold: identity}
	}
	color := func(c string) func(string) string {
		st := lipgloss.NewStyle().Foreground(lipgloss.Color(c))
		return func(s string) string { return st.Render(s) }
	}
	return doctorPalette{
		broken: color("1"),
		warn:   color("3"),
		info:   color("6"),
		good:   color("2"),
		dim:    func(s string) string { return lipgloss.NewStyle().Faint(true).Render(s) },
		bold:   func(s string) string { return lipgloss.NewStyle().Bold(true).Render(s) },
	}
}

// sevIcon renders a section's severity marker.
func (pal doctorPalette) sevIcon(sev string) string {
	switch sev {
	case "broken":
		return pal.broken("✖")
	case "warn":
		return pal.warn("⚠")
	default:
		return pal.info("◦")
	}
}

func printFindings(out io.Writer, findings []doctor.Finding, pal doctorPalette) error {
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
		if err := printSectionHeader(out, section.title, section.note, section.sev, len(group), pal); err != nil {
			return err
		}
		// Align the harness column by hand: ANSI styles would throw off
		// tabwriter's width math, and the names are plain anyway.
		width := 0
		for _, f := range group {
			if n := len([]rune(f.Harness)); n > width {
				width = n
			}
		}
		for _, f := range group {
			line := "  " + f.Message
			if f.Harness != "" {
				line = "  " + pal.info(padRight(f.Harness, width)) + "  " + f.Message
			}
			if _, err := fmt.Fprintln(out, line); err != nil {
				return err
			}
		}
	}
	return nil
}

// printSectionHeader renders one section header: severity icon, title,
// count, and the dim note.
func printSectionHeader(out io.Writer, title, note, sev string, n int, pal doctorPalette) error {
	header := fmt.Sprintf("%s %s %s", pal.sevIcon(sev), pal.bold(title), pal.warn(fmt.Sprintf("(%d)", n)))
	if note != "" {
		header += " " + pal.dim("· "+note)
	}
	_, err := fmt.Fprintln(out, header)
	return err
}

// reportConflicts prints the manual-edit conflicts as a table: one row per
// disagreement, the options once in a legend below. Nothing is asked and
// nothing changes.
func reportConflicts(out io.Writer, conflicts []doctor.Conflict, pal doctorPalette) error {
	if len(conflicts) == 0 {
		return nil
	}
	if err := printSectionHeader(out, "manual edit conflicts", "left as is", "warn", len(conflicts), pal); err != nil {
		return err
	}

	hw, sw := len("HARNESS"), len("SKILL")
	for _, c := range conflicts {
		if n := len([]rune(c.Harness)); n > hw {
			hw = n
		}
		if n := len([]rune(c.Skill)); n > sw {
			sw = n
		}
	}
	rows := make([]string, len(conflicts))
	for i, c := range conflicts {
		disagreement := "config off · state on"
		if !c.ConfigDisables {
			disagreement = "config on · state off"
		}
		rows[i] = fmt.Sprintf("  %s  %s  %s",
			pal.info(padRight(c.Harness, hw)),
			padRight(c.Skill, sw),
			pal.warn(disagreement))
	}
	header := fmt.Sprintf("  %s  %s  %s",
		pal.dim(padRight("HARNESS", hw)),
		pal.dim(padRight("SKILL", sw)),
		pal.dim("DISAGREEMENT"))

	if _, err := fmt.Fprintln(out, header); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(out, row); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out, "  "+pal.dim("k keep my change · r restore — run `fleet skill doctor -i` to pick per skill"))
	return err
}

// resolveConflicts walks the manual-edit conflicts and applies whatever the
// user picks at each prompt. With no input (piped stdin, EOF) everything is
// reported and left as is. It returns how many conflicts were resolved.
//
// Conflicts arrive grouped per harness, so "all" means the current
// harness's remaining batch: [a] keeps every one of them (adopting that
// config's side into the state) and [x] skips them. The batch options only
// appear when there is more than one, and the next harness's conflicts are
// still prompted individually — one keypress never adopts edits from a
// config the user hasn't been shown.
func resolveConflicts(cmd *cobra.Command, p *paths.Paths, conflicts []doctor.Conflict, pal doctorPalette) (int, error) {
	if len(conflicts) == 0 {
		return 0, nil
	}
	out := cmd.OutOrStdout()
	in := bufio.NewScanner(cmd.InOrStdin())
	inputEnded := false
	noted := false
	resolved := 0

	for i := 0; i < len(conflicts); {
		c := conflicts[i]
		// The harness batch: this conflict plus the ones that follow for
		// the same harness.
		batchEnd := i
		for batchEnd+1 < len(conflicts) && conflicts[batchEnd+1].Harness == c.Harness {
			batchEnd++
		}
		batch := batchEnd - i + 1

		if _, err := fmt.Fprintf(out, "\n%s %s: %s\n", pal.warn("⚠"), pal.info(c.Harness), c.Message); err != nil {
			return resolved, err
		}
		if _, err := fmt.Fprintf(out, "  %s keep my change — %s\n", pal.good("[k]"), keepLabel(c)); err != nil {
			return resolved, err
		}
		if _, err := fmt.Fprintf(out, "  %s restore — %s\n", pal.info("[r]"), restoreLabel(c)); err != nil {
			return resolved, err
		}
		if _, err := fmt.Fprintf(out, "  %s skip — leave it as is\n", pal.dim("[s]")); err != nil {
			return resolved, err
		}
		if batch > 1 {
			if _, err := fmt.Fprintf(out, "  %s keep all %d %s conflicts\n", pal.warn("[a]"), batch, c.Harness); err != nil {
				return resolved, err
			}
			if _, err := fmt.Fprintf(out, "  %s skip all %d %s conflicts\n", pal.dim("[x]"), batch, c.Harness); err != nil {
				return resolved, err
			}
		}

		if inputEnded || !in.Scan() {
			inputEnded = true
			note := "  left as is (no input)"
			if !noted {
				noted = true
				note += "; run `fleet skill doctor -i` from a terminal to resolve"
			}
			if _, err := fmt.Fprintln(out, note); err != nil {
				return resolved, err
			}
			i++
			continue
		}

		next := i + 1
		switch strings.ToLower(strings.TrimSpace(in.Text())) {
		case "k":
			if err := keepConflict(out, p, c, pal); err != nil {
				return resolved, err
			}
			resolved++
		case "r":
			if err := restoreConflictReceipt(out, p, c, pal); err != nil {
				return resolved, err
			}
			resolved++
		case "a":
			for _, cj := range conflicts[i : batchEnd+1] {
				if err := keepConflict(out, p, cj, pal); err != nil {
					return resolved, err
				}
				resolved++
			}
			next = batchEnd + 1
		case "x":
			if _, err := fmt.Fprintf(out, "  skipped %d %s conflict%s\n", batch, c.Harness, plural(batch)); err != nil {
				return resolved, err
			}
			next = batchEnd + 1
		default:
			if _, err := fmt.Fprintln(out, "  skipped"); err != nil {
				return resolved, err
			}
		}
		i = next
	}
	return resolved, nil
}

// keepConflict applies one keep and prints its receipt.
func keepConflict(out io.Writer, p *paths.Paths, c doctor.Conflict, pal doctorPalette) error {
	if _, err := doctor.Resolve(p, c, true); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "  kept: %q recorded as %s for %s in the state file\n",
		c.Skill, keepState(c), c.Harness)
	return err
}

// restoreConflictReceipt applies one restore and prints what changed.
func restoreConflictReceipt(out io.Writer, p *paths.Paths, c doctor.Conflict, pal doctorPalette) error {
	rep, err := doctor.Resolve(p, c, false)
	if err != nil {
		return err
	}
	for _, ch := range rep.Changed {
		if _, err := fmt.Fprintf(out, "  restored: %s\n", restoreChange(c.Harness, ch)); err != nil {
			return err
		}
	}
	for _, f := range rep.Flags {
		if _, err := fmt.Fprintf(out, "  restored: %s: %s\n", c.Harness, f.Message); err != nil {
			return err
		}
	}
	if len(rep.Changed) == 0 && len(rep.Flags) == 0 {
		if _, err := fmt.Fprintf(out, "  restored: %s already matches the state\n", c.Harness); err != nil {
			return err
		}
	}
	return nil
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
// resolved counts only apply to interactive runs; a non-interactive run
// points at --interactive instead.
func printSummary(out io.Writer, rep doctor.Report, resolved int, interactive bool, pal doctorPalette) error {
	if rep.Empty() {
		_, err := fmt.Fprintln(out, "\n"+pal.good("no problems found"))
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
	} else if !interactive && len(rep.Conflicts) > 0 {
		parts = append(parts, "run `fleet skill doctor -i` to resolve")
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
	case doctor.KindDoublePresence:
		return pluralized("double-presence finding", n)
	case doctor.KindStaleLock:
		return pluralized("stale lockfile entry", n)
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

// padRight pads s with spaces to width w (runes, not bytes — the strings
// are plain, the styling happens after padding).
func padRight(s string, w int) string {
	if n := len([]rune(s)); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}
