// The sync command: the ambient sync every fleet command runs, exposed
// as an explicit verb. Same machinery (internal/sync.Run), same report —
// the state file is projected into every installed writable harness and
// redundant links are removed, so scripts can converge a home after
// running the skills CLI by hand without invoking any other command.

package cli

import (
	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/paths"
)

func newSkillSyncCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Repair drift now: converge harness configs with the state file",
		Long: "Run the same sync every fleet command runs: load the state file, remove redundant links, project the recorded disables into each installed harness with a write side, and flag anything fleet doesn't recognize without touching it.\n\n" +
			"The explicit, scriptable way to converge harness configs with the state after running the skills CLI by hand. `fleet skill doctor` previews all of it read-only; sync does it. Nothing to repair is not an error: sync is idempotent, and a converged home prints nothing.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSyncTo(cmd.OutOrStdout(), p)
		},
	}
}
