// Package cli wires fleet's Cobra command tree. Every command receives the
// injected home root; nothing here resolves paths on its own.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/zacong/fleet/internal/buildinfo"
	"github.com/zacong/fleet/internal/paths"
	"github.com/zacong/fleet/internal/tui"
)

// NewRoot builds the `fleet` command tree under an injected home root.
func NewRoot(p *paths.Paths) *cobra.Command {
	var quiet bool

	root := &cobra.Command{
		Use:   "fleet",
		Short: "Manage agent skills across harnesses",
		Long: "Fleet manages agent skills across AI coding agents: enable or disable a skill per harness without uninstalling it. The skills CLI stays the install and update backend.\n" +
			"\n" +
			"Run bare `fleet` to open the interactive skill × harness matrix. It needs a terminal: with piped output fleet prints the same listing as `fleet skill ls` instead, and the hero banner never appears. `--quiet` keeps the TUI but drops the banner.",
		// Bare `fleet` opens the TUI (RunE below); a pipe gets the plain
		// listing — a matrix no one can steer is not a face, it is garbage
		// on stdout.
		// Piped, `--version`, or `completion` never reach RunE: Cobra
		// handles those before dispatch.
		SilenceUsage: true,
		Version:      buildinfo.Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !stdoutIsTTY() {
				return runListing(cmd, p, false, quiet)
			}
			return tui.Run(p, quiet)
		},
	}
	root.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress the hero banner")
	root.AddCommand(newSkillCmd(p))
	return root
}

func newSkillCmd(p *paths.Paths) *cobra.Command {
	skill := &cobra.Command{
		Use:   "skill",
		Short: "Manage skills in the canonical store",
	}
	skill.AddCommand(newSkillLsCmd(p))
	skill.AddCommand(newSkillOnCmd(p))
	skill.AddCommand(newSkillOffCmd(p))
	skill.AddCommand(newSkillAdoptCmd(p))
	skill.AddCommand(newSkillDoctorCmd(p))
	skill.AddCommand(newSkillUpdateCmd(p))
	return skill
}
