// The update command: the wrapped `skills update -g -y` run, then sync —
// disabled markers re-applied and redundant links re-removed no matter
// what the wrapped run re-created — and a report built from fleet's own
// post-run state: the canonical store scan, the skills CLI lockfile (read
// only, like always), the state file, and the harness configs. The
// skills CLI's prose is shown raw on failure and never parsed.

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/customs"
	"github.com/zzacong/fleet/internal/harness"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/scan"
	"github.com/zzacong/fleet/internal/skillscli"
	"github.com/zzacong/fleet/internal/state"
)

// newSkillsRunner builds the skills CLI runner: the real exec runner.
// Tests swap it for a scripted stub — the skills binary is absent in
// sandboxes.
var newSkillsRunner = func() skillscli.Runner { return skillscli.Exec{} }

func newSkillUpdateCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "update [skill]",
		Short: "Update installed skills with the skills CLI, then sync",
		Long: "Run `skills update -g -y` — the skills CLI stays the update backend — then sync, so disabled skills stay disabled and cleaned links stay clean no matter what the wrapped run re-created.\n\n" +
			"With a skill name, only that skill is updated (`skills update -g -y <skill>`); otherwise every installed skill is updated. The wrapped call is fully explicit and non-interactive: stdin is piped closed, so an unexpected prompt fails fast instead of hanging. Its output is shown raw on failure and never parsed; fleet reports from its own post-run state (state file, lockfile, harness configs). The skills CLI lockfile is read-only, and running the skills CLI by hand keeps working.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Snapshot before the wrapped run so the report can state
			// what — if anything — actually changed. The lockfile is the
			// source of truth for installed skills; the store census is
			// the fallback when the lock is empty.
			beforeLock, _ := scan.ReadLockfile(p.SkillLock())
			beforeSkills, _ := scan.ScanStore(p.SkillsStore())

			if len(args) == 1 {
				name := args[0]
				if err := validateUpdateTarget(p, name, beforeSkills); err != nil {
					return err
				}
				if _, err := skillscli.UpdateOne(newSkillsRunner(), name); err != nil {
					return showSkillsFailure(cmd, err)
				}
			} else {
				// The wrapped run. A failure here is the headline: show the
				// CLI's raw output and stop — a half-finished update is the
				// user's to resolve before anything else runs.
				if _, err := skillscli.Update(newSkillsRunner()); err != nil {
					return showSkillsFailure(cmd, err)
				}
			}

			out := cmd.OutOrStdout()

			// Sync re-projects the state file and re-removes redundant
			// links, then reports what the wrapped run disturbed. This
			// is intentional: `fleet skill update` is `skills update -g -y`
			// followed by sync, so disables stay disabled even when the
			// wrapped run re-creates links or resurrects config.
			if err := runSyncTo(out, p); err != nil {
				return err
			}
			return printUpdateReport(out, p, beforeLock, beforeSkills)
		},
	}
}

func validateUpdateTarget(p *paths.Paths, name string, storeSkills []scan.Skill) error {
	for _, s := range storeSkills {
		if s.Name == name {
			return nil
		}
	}
	// Not in the canonical store — check if it's a custom skill in the
	// tracked set or the fleet-home fallback.
	if ok, _ := isCustomSkill(p, name); ok {
		return fmt.Errorf("skill %q is a custom skill — nothing to update", name)
	}
	return fmt.Errorf("skill %q not found in %s", name, p.SkillsStore())
}

func isCustomSkill(p *paths.Paths, name string) (bool, error) {
	// Every custom home: each tracked collection plus the fleet-home
	// fallback, in adopt-candidate order.
	homes, err := customs.AdoptCandidates(p)
	if err != nil {
		return false, err
	}
	for _, home := range homes {
		// Direct check for <home>/<name>/SKILL.md — the common case
		// where Dir == Name.
		if _, err := os.Stat(filepath.Join(home, name, "SKILL.md")); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, err
		}
		// Frontmatter name may differ from directory name; scan the home
		// and match by Skill.Name.
		skills, err := scan.ScanStore(home)
		if err != nil {
			return false, err
		}
		for _, s := range skills {
			if s.Name == name {
				return true, nil
			}
		}
	}
	return false, nil
}

// showSkillsFailure prints the skills CLI's captured output — verbatim,
// fleet doesn't interpret it — and returns the error so cobra reports the
// failure.
func showSkillsFailure(cmd *cobra.Command, err error) error {
	var cliErr *skillscli.Error
	if errors.As(err, &cliErr) && cliErr.Output != "" {
		if _, werr := fmt.Fprintln(cmd.ErrOrStderr(), cliErr.Output); werr != nil {
			return werr
		}
	}
	return err
}

// printUpdateReport summarizes the post-run state from fleet's own
// records: the store scan and lockfile for what is installed, and the
// state file re-read against the harness configs for what stayed
// disabled. It also reports what — if anything — changed compared to
// the pre-update snapshot, so "zero updated" is an explicit outcome
// rather than an ambiguous census. The outcome line carries the same
// on/off weight: the leading verb is green on a terminal, details dim.
func printUpdateReport(out io.Writer, p *paths.Paths, beforeLock map[string]scan.Provenance, beforeSkills []scan.Skill) error {
	skills, err := scan.ScanStore(p.SkillsStore())
	if err != nil {
		return fmt.Errorf("scan canonical store: %w", err)
	}
	lock, err := scan.ReadLockfile(p.SkillLock())
	if err != nil {
		return fmt.Errorf("read skills lockfile: %w", err)
	}
	installed := scan.CountInstalled(skills, lock)
	pal := newPalette(stdoutIsTTY())
	// Census is context, not the outcome — keep it dim so the eye lands
	// on the outcome line (like on/off's "enabled … for …" headline).
	census := fmt.Sprintf("%d skill%s in %s (%d installed, %d custom)",
		len(skills), plural(len(skills)), p.SkillsStore(), installed, len(skills)-installed)
	if _, err := fmt.Fprintln(out, pal.dim(census)); err != nil {
		return err
	}
	if err := printUpdateDiff(out, beforeLock, beforeSkills, lock, skills); err != nil {
		return err
	}
	return verifyDisables(out, p, skills)
}

// printUpdateDiff reports what the wrapped run actually changed, derived
// from fleet's own state rather than the skills CLI's prose. A zero
// result is explicit ("no skills updated") so the update is not
// mistaken for a silent failure. The outcome line carries the same
// weight as on/off's headline: verb in green on a terminal.
func printUpdateDiff(out io.Writer, beforeLock map[string]scan.Provenance, beforeSkills []scan.Skill, afterLock map[string]scan.Provenance, afterSkills []scan.Skill) error {
	if beforeLock == nil {
		beforeLock = map[string]scan.Provenance{}
	}
	if afterLock == nil {
		afterLock = map[string]scan.Provenance{}
	}
	updated := updatedSkillNames(beforeLock, afterLock)
	// Also consider store adds/removes when lock is empty (all custom) or
	// a skill was installed/removed outside the lock's hash (new dir).
	if len(updated) == 0 && len(beforeSkills) != len(afterSkills) {
		// Store count changed but lock didn't — treat as an update (custom
		// skill added/removed or lock missing). Report the count delta.
		if len(afterSkills) > len(beforeSkills) {
			updated = []string{fmt.Sprintf("%d added", len(afterSkills)-len(beforeSkills))}
		} else {
			updated = []string{fmt.Sprintf("%d removed", len(beforeSkills)-len(afterSkills))}
		}
	}
	pal := newPalette(stdoutIsTTY())
	if len(updated) == 0 {
		// Outcome: matches on/off's "already enabled" weight — verb in green
		// so a no-op update is not mistaken for silent noise after sync lines.
		_, err := fmt.Fprintln(out, pal.dim("update: ")+pal.good("no skills updated"))
		return err
	}
	// Name the updated skills when the set is small; otherwise just count.
	// Verb green like "enabled"/"disabled" on/off headline.
	if len(updated) <= 8 {
		_, err := fmt.Fprintf(out, "%s%s %d skill%s: %s\n", pal.dim("update: "), pal.good("updated"), len(updated), plural(len(updated)), strings.Join(updated, ", "))
		return err
	}
	_, err := fmt.Fprintf(out, "%s%s %d skills\n", pal.dim("update: "), pal.good("updated"), len(updated))
	return err
}

func updatedSkillNames(before, after map[string]scan.Provenance) []string {
	var out []string
	for dir, provAfter := range after {
		provBefore, ok := before[dir]
		if !ok {
			out = append(out, dir)
			continue
		}
		if provBefore.Hash != provAfter.Hash || provBefore.Source != provAfter.Source {
			out = append(out, dir)
		}
	}
	for dir := range before {
		if _, ok := after[dir]; !ok {
			out = append(out, dir+" (removed)")
		}
	}
	sort.Strings(out)
	return out
}

// verifyDisables re-reads each writable harness's config after sync and
// reports the state file's disables that hold: the skill is not loading,
// which is what "disabled" promised. A disable that still doesn't hold
// means interference sync already flagged — say so plainly instead of
// counting it as held.
func verifyDisables(out io.Writer, p *paths.Paths, skills []scan.Skill) error {
	st, err := state.Load(p.FleetStateFile())
	if err != nil {
		return err
	}

	// The universe of names the configs can talk about: everything in the
	// store plus everything the state knows (a disable may outlive the
	// skill it was recorded for).
	storeNames := make([]string, 0, len(skills))
	for _, s := range skills {
		storeNames = append(storeNames, s.Name)
	}
	names := st.Universe(storeNames)

	holds := map[string][]string{} // skill -> harnesses where the disable holds
	pal := newPalette(stdoutIsTTY())
	for _, a := range harness.Installed(p) {
		if !a.CanProject() {
			continue
		}
		h := string(a.Harness())
		disabled := st.Disabled(h)
		if len(disabled) == 0 {
			continue
		}
		read, err := a.Read(names)
		if err != nil {
			return fmt.Errorf("read %s config: %w", h, err)
		}
		for _, name := range disabled {
			switch read.States[name] {
			case harness.StateOff, harness.StateAbsent:
				holds[name] = append(holds[name], h)
			default:
				if _, err := fmt.Fprintf(out, "%s%s %q for %s did not stay disabled\n", pal.dim("update: "), pal.broken("verified:"), name, pal.info(h)); err != nil {
					return err
				}
			}
		}
	}

	for _, name := range sortedStrings(holds) {
		if _, err := fmt.Fprintf(out, "%s%s %q for %s\n", pal.dim("update: "), pal.good("verified disabled:"), name, pal.info(strings.Join(holds[name], ", "))); err != nil {
			return err
		}
	}
	return nil
}

// sortedStrings returns the map's keys in sorted order, for a stable
// report.
func sortedStrings[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
