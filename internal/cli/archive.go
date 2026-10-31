// The archive command: move a custom skill out of its collection dir into
// the sibling archived/ folder and unlink it from every harness. The rules
// live in the customs layer; this layer resolves the name, prints, and
// syncs.
package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zzacong/fleet/internal/customs"
	"github.com/zzacong/fleet/internal/paths"
	"github.com/zzacong/fleet/internal/skillindex"
)

func newSkillArchiveCmd(p *paths.Paths) *cobra.Command {
	return &cobra.Command{
		Use:   "archive <name>",
		Short: "Archive a custom skill and unlink it from every harness",
		Long: "Move a custom skill's folder into an `archived/` folder beside its collection dir, unlink its managed links from every installed harness, and drop its enablement entries, Claude's included.\n\n" +
			"`<name>` is the skill's directory name. A skill in the fleet-home fallback archives into the fleet home's `archived/`. Archive is one-way: restoring a skill is a manual move back.\n\n" +
			"After the move, sync runs as on every command. Fleet consults no git binary, so committing the move is yours.",
		Example: "  fleet skill archive my-notes",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			pal := newPalette(stdoutIsTTY())
			hit, err := resolveCustomCopy(p, args[0])
			if err != nil {
				return err
			}
			arc, err := customs.Archive(p, hit)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "%s%s %q\n", pal.dim("archive: "), pal.good("archived"), arc.To); err != nil {
				return err
			}
			for _, l := range arc.Unlinked {
				if _, err := fmt.Fprintln(out, pal.dim("archive: ")+formatUnlinked(string(l.Harness), l, pal)); err != nil {
					return err
				}
			}
			return runSyncTo(out, p)
		},
	}
}

// resolveCustomCopy returns the one custom copy named name. A name with no
// custom copy is an error, and so is one with several: archive must never
// pick a copy the user didn't name, so the error lists every path.
func resolveCustomCopy(p *paths.Paths, name string) (skillindex.Hit, error) {
	copies, err := customs.CustomCopies(p, name)
	if err != nil {
		return skillindex.Hit{}, err
	}
	switch len(copies) {
	case 0:
		return skillindex.Hit{}, fmt.Errorf("archive: %q is not a custom skill in a tracked dir", name)
	case 1:
		return copies[0], nil
	}
	lines := make([]string, 0, len(copies))
	for _, c := range copies {
		lines = append(lines, "- "+filepath.Join(c.Home, c.Skill.Dir))
	}
	return skillindex.Hit{}, fmt.Errorf("archive: %q matches %d custom copies; archive one by hand:\n%s", name, len(copies), strings.Join(lines, "\n"))
}
