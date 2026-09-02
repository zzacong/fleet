// Cobra's default usage template prints each subcommand's Short on one line
// and lets the terminal wrap the overflow back to column 0, so long
// descriptions start flush under the command names. This file swaps in a
// usage template that re-wraps descriptions at the description column, so
// continuation lines stay aligned under the text that started them.
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// fallbackHelpWidth stands in when stdout is not a terminal (pipes, tests):
// an 80-column assumption keeps the wrapping deterministic there.
const fallbackHelpWidth = 80

// helpWidth is a variable so tests can pin the terminal width.
var helpWidth = func() int {
	if w, ok := terminalWidth(); ok && w > 0 {
		return w
	}
	return fallbackHelpWidth
}

func init() {
	// Package-level registry: templates inherit down the tree from root, so
	// one registration covers every command's help screen.
	cobra.AddTemplateFunc("wrapCommand", wrapCommand)
}

// usageTemplate mirrors Cobra's default usage template with one change: the
// Available Commands rows go through wrapCommand instead of rpad + Short, so
// descriptions wrap at the description column rather than the screen edge.
const usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{wrapCommand .Name .NamePadding .Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{wrapCommand .Name .NamePadding .Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{wrapCommand .Name .NamePadding .Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

// wrapCommand renders one Available Commands row: the name padded to its
// column, then the Short wrapped to the terminal width with continuation
// lines indented to the description column.
func wrapCommand(name string, padding int, short string) string {
	// The template supplies the two-space command indent; wrapCommand pads
	// the name and indents continuation lines to the description column —
	// the two-space indent plus the padded name, not the padded name alone.
	prefix := fmt.Sprintf("%-*s", padding, name) + " "
	indent := strings.Repeat(" ", 2+len(prefix))

	avail := helpWidth() - 2 - len(prefix)
	var lines []string
	line := ""
	for _, word := range strings.Fields(short) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) <= avail:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	lines = append(lines, line)

	return prefix + strings.Join(lines, "\n"+indent)
}
