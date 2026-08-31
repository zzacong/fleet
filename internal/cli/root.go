// Package cli wires fleet's Cobra command tree. Every command receives the
// injected home root; nothing here resolves paths on its own.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/zacong/fleet/internal/paths"
)

// NewRoot builds the `fleet` command tree under an injected home root.
func NewRoot(p *paths.Paths) *cobra.Command {
	root := &cobra.Command{
		Use:   "fleet",
		Short: "Manage agent skills across harnesses",
		Long:  "Fleet manages agent skills across AI coding agents: enable or disable a skill per harness without uninstalling it. The skills CLI stays the install and update backend.",
		// Bare `fleet` opens the TUI in a later ticket; until then show help.
		SilenceUsage: true,
	}
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
	return skill
}
