// The on/off commands: record the toggle in the state file, then let sync
// project it into each harness's native config. The write sequence itself
// lives in internal/toggle — the TUI's staged apply runs the same code.
// The command reports its own outcome: one headline line naming the
// harnesses the recorded state now holds, then sync's leftover findings
// (real repairs, and flags that kept the toggle from landing). Ambient
// findings about other skills are sync's and doctor's business; the
// toggle stays quiet about them. Cursor and Bob have no write side —
// toggling for them is a no-op with a clear message.

package cli

import (
	"fmt"
	"io"
	"slices"
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

			// The outcome leads; sync's leftover findings follow quietly.
			writeTargets := make([]string, 0, len(toggles))
			for _, t := range toggles {
				writeTargets = append(writeTargets, t.Harness)
			}
			if err := printToggleOutcome(out, name, on, writeTargets, projected, reports); err != nil {
				return err
			}
			if err := printToggleDetail(out, name, writeTargets, projected, reports); err != nil {
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

// printToggleOutcome writes the command's headline: one line naming every
// targeted harness whose config now holds the recorded state —
// `disabled "tdd" for opencode, pi`. A harness flagged in the reports
// stays out of the list; its flag line below explains what fleet couldn't
// change. When nothing moved the line says so: `"tdd" is already enabled
// for opencode, pi`.
func printToggleOutcome(out io.Writer, name string, on bool, targets []string, projected []toggle.Projected, reports []fleetsync.Report) error {
	if len(targets) == 0 {
		return nil
	}
	blocked := toggledFlagHarnesses(name, targets, projected, reports)
	reached := make([]string, 0, len(targets))
	for _, h := range targets {
		if !blocked[h] {
			reached = append(reached, h)
		}
	}
	if len(reached) == 0 {
		return nil // every target is flagged; the flags are the report
	}
	verb := "enabled"
	if !on {
		verb = "disabled"
	}
	pal := newPalette(stdoutIsTTY())
	if toggledFlips(name, targets, projected, reports) {
		_, err := fmt.Fprintf(out, "%s %q for %s\n", pal.good(verb), name, strings.Join(reached, ", "))
		return err
	}
	_, err := fmt.Fprintf(out, "%q is %s for %s\n", name, pal.good("already "+verb), strings.Join(reached, ", "))
	return err
}

// toggledFlagHarnesses collects the targeted harnesses whose reports flag
// the toggled skill: there the write did not fully hold.
func toggledFlagHarnesses(name string, targets []string, projected []toggle.Projected, reports []fleetsync.Report) map[string]bool {
	blocked := map[string]bool{}
	for _, pr := range projected {
		for _, f := range pr.Report.Flags {
			if f.Skill == name {
				blocked[string(pr.Harness)] = true
			}
		}
	}
	for _, r := range reports {
		for _, f := range r.Flags {
			if f.Skill == name && slices.Contains(targets, r.Harness) {
				blocked[r.Harness] = true
			}
		}
	}
	return blocked
}

// toggledFlips reports whether any targeted harness's config actually
// moved for the skill: the direct "on" writes report their own flips,
// sync's reports carry the "off" projections.
func toggledFlips(name string, targets []string, projected []toggle.Projected, reports []fleetsync.Report) bool {
	for _, pr := range projected {
		for _, c := range pr.Report.Changed {
			if c.Skill == name {
				return true
			}
		}
	}
	for _, r := range reports {
		for _, c := range r.Changed {
			if c.Skill == name && slices.Contains(targets, r.Harness) {
				return true
			}
		}
	}
	return false
}

// printToggleDetail writes what the run did beyond the toggle itself, in
// sync's own order: redundant-link removals, then drift repairs, then
// flags. Flags about the toggled skill always print — they mean the
// recorded state did not fully land — deduplicated, because the direct
// "on" writes and the sync pass that follows can flag the same entry.
// Flags about other skills are ambient config findings: `fleet skill
// sync` and `fleet skill doctor` report them, the toggle stays quiet.
func printToggleDetail(out io.Writer, name string, targets []string, projected []toggle.Projected, reports []fleetsync.Report) error {
	pal := newPalette(stdoutIsTTY())
	targeted := map[string]bool{}
	for _, h := range targets {
		targeted[h] = true
	}

	for _, r := range reports {
		for _, e := range r.Removed {
			if _, err := fmt.Fprintln(out, styleSyncLine(formatRemoved(r.Harness, e), pal)); err != nil {
				return err
			}
		}
	}
	for _, r := range reports {
		for _, c := range r.Changed {
			if c.Skill == name && targeted[r.Harness] {
				continue // the outcome line owns this flip
			}
			if _, err := fmt.Fprintln(out, styleChange(formatChange(r.Harness, c), pal)); err != nil {
				return err
			}
		}
	}

	seen := map[string]bool{}
	printToggledFlag := func(harnessName string, f harness.Flag) error {
		key := harnessName + "\x00" + f.Message
		if seen[key] {
			return nil
		}
		seen[key] = true
		_, err := fmt.Fprintln(out, styleSyncLine(formatFlag(harnessName, f), pal))
		return err
	}
	for _, pr := range projected {
		for _, f := range pr.Report.Flags {
			if f.Skill != name {
				continue
			}
			if err := printToggledFlag(string(pr.Harness), f); err != nil {
				return err
			}
		}
	}
	for _, r := range reports {
		for _, f := range r.Flags {
			if f.Skill != name {
				continue
			}
			if err := printToggledFlag(r.Harness, f); err != nil {
				return err
			}
		}
	}
	return nil
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
	pal := newPalette(stdoutIsTTY())
	for _, r := range reports {
		for _, e := range r.Removed {
			if _, err := fmt.Fprintln(out, styleSyncLine(formatRemoved(r.Harness, e), pal)); err != nil {
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
	pal := newPalette(stdoutIsTTY())
	for _, c := range changed {
		if _, err := fmt.Fprintln(out, styleChange(formatChange(harnessName, c), pal)); err != nil {
			return err
		}
	}
	for _, g := range groupFlags(flags) {
		line := ""
		if len(g.skills) == 1 {
			line = formatFlag(harnessName, harness.Flag{Skill: g.skills[0], Message: g.message})
		} else {
			line = formatFlagGroup(harnessName, g)
		}
		if _, err := fmt.Fprintln(out, styleSyncLine(line, pal)); err != nil {
			return err
		}
	}
	return nil
}

// styleSyncLine dims the "sync:" prefix and the harness scope so the eye
// lands on what happened, not on who said it.
func styleSyncLine(line string, pal palette) string {
	rest, ok := strings.CutPrefix(line, "sync: ")
	if !ok {
		return line
	}
	scope := rest
	tail := ""
	if i := strings.Index(rest, ": "); i >= 0 {
		scope, tail = rest[:i], rest[i:]
	}
	return pal.dim("sync: ") + pal.info(scope) + tail
}

// styleChange renders one applied flip with the weight the on/off outcome
// line gives its verb: dim "sync:" prefix, cyan harness scope, the verb in
// green, and the "(was on)" provenance demoted to a faint annotation.
// Change lines only — flags never pass through here, because their
// messages can open with the same words ("disabled in config but not
// tracked…") and must not borrow the verb's color.
func styleChange(line string, pal palette) string {
	rest, ok := strings.CutPrefix(line, "sync: ")
	if !ok {
		return line
	}
	scope, tail, ok := strings.Cut(rest, ": ")
	if !ok {
		return line
	}
	verb, detail, ok := strings.Cut(tail, " ")
	if !ok {
		return line
	}
	name, note, hasNote := strings.Cut(detail, " (was ")
	styled := pal.dim("sync: ") + pal.info(scope) + ": " + pal.good(verb) + " " + name
	if hasNote {
		styled += " " + pal.dim("(was "+note)
	}
	return styled
}

// flagGroup collapses flags that share a message within one harness: twelve
// near-identical "not tracked" lines become one line with the skill names.
type flagGroup struct {
	message string
	skills  []string
}

// groupFlags merges flags by message, keeping first-appearance order.
func groupFlags(flags []harness.Flag) []flagGroup {
	var groups []flagGroup
	index := map[string]int{}
	for _, f := range flags {
		if i, ok := index[f.Message]; ok {
			groups[i].skills = append(groups[i].skills, f.Skill)
			continue
		}
		index[f.Message] = len(groups)
		groups = append(groups, flagGroup{message: f.Message, skills: []string{f.Skill}})
	}
	return groups
}

// formatChange renders one applied flip as plain text: "sync: opencode:
// disabled \"tdd\" (was on)". styleChange adds the terminal styling.
func formatChange(harnessName string, c harness.Change) string {
	verb := "disabled"
	if c.To == harness.StateOn {
		verb = "enabled"
	}
	return fmt.Sprintf("sync: %s: %s %q (was %s)", harnessName, verb, c.Skill, c.From)
}

// formatRemoved renders one redundant-link removal as plain text: "sync:
// opencode: removed redundant link \"tdd\" — opencode scans the canonical
// store natively".
func formatRemoved(harnessName string, e harness.Entry) string {
	return fmt.Sprintf("sync: %s: removed redundant link %q — %s", harnessName, e.Name, e.Reason)
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

// formatFlagGroup renders several flags sharing one message as a single
// line: "sync: pi: disabled in config but not tracked by fleet's state —
// left alone (12 skills): a, b, c". The names carry the detail; the
// message is not repeated per skill.
func formatFlagGroup(harnessName string, g flagGroup) string {
	return fmt.Sprintf("sync: %s: %s (%d skills): %s",
		harnessName, g.message, len(g.skills), strings.Join(g.skills, ", "))
}
