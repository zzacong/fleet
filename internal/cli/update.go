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
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zacong/fleet/internal/harness"
	"github.com/zacong/fleet/internal/paths"
	"github.com/zacong/fleet/internal/scan"
	"github.com/zacong/fleet/internal/skillscli"
	"github.com/zacong/fleet/internal/state"
)

// newSkillsRunner builds the skills CLI runner: the real exec runner.
// Tests swap it for a scripted stub — the skills binary is absent in
// sandboxes.
var newSkillsRunner = func() skillscli.Runner { return skillscli.Exec{} }

func newSkillUpdateCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update installed skills with the skills CLI, then sync",
		Long: "Run `skills update -g -y` — the skills CLI stays the update backend — then sync, so disabled skills stay disabled and cleaned links stay clean no matter what the wrapped run re-created.\n\n" +
			"The wrapped call is fully explicit and non-interactive: stdin is piped closed, so an unexpected prompt fails fast instead of hanging. Its output is shown raw on failure and never parsed; fleet reports from its own post-run state (state file, lockfile, harness configs). The skills CLI lockfile is read-only, and running the skills CLI by hand keeps working.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// The wrapped run. A failure here is the headline: show the
			// CLI's raw output and stop — a half-finished update is the
			// user's to resolve before anything else runs.
			if _, err := skillscli.Update(newSkillsRunner()); err != nil {
				return showSkillsFailure(cmd, err)
			}

			out := cmd.OutOrStdout()

			// Sync re-projects the state file and re-removes redundant
			// links, then reports what the wrapped run disturbed.
			if err := runSyncTo(out, p); err != nil {
				return err
			}
			return printUpdateReport(out, p)
		},
	}
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
// disabled.
func printUpdateReport(out io.Writer, p *paths.Paths) error {
	skills, err := scan.ScanStore(p.SkillsStore())
	if err != nil {
		return fmt.Errorf("scan canonical store: %w", err)
	}
	lock, err := scan.ReadLockfile(p.SkillLock())
	if err != nil {
		return fmt.Errorf("read skills lockfile: %w", err)
	}
	installed := 0
	for _, s := range skills {
		if _, ok := lock[s.Dir]; ok {
			installed++
		}
	}
	line := fmt.Sprintf("%d skill%s in %s (%d installed, %d custom)",
		len(skills), plural(len(skills)), p.SkillsStore(), installed, len(skills)-installed)
	if _, err := fmt.Fprintln(out, line); err != nil {
		return err
	}
	return verifyDisables(out, p, skills)
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
				if _, err := fmt.Fprintf(out, "%s: %s did not stay disabled\n", name, h); err != nil {
					return err
				}
			}
		}
	}

	for _, name := range sortedStrings(holds) {
		if _, err := fmt.Fprintf(out, "disabled: %q for %s\n", name, strings.Join(holds[name], ", ")); err != nil {
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
