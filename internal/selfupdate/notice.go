package selfupdate

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// display renders a tag with its leading "v" so "0.1.0" and "v0.1.0" show
// identically.
func display(tag string) string {
	return "v" + normalize(tag)
}

// lines is the notice body: one title line plus the generic re-install
// hint. Fleet cannot detect which door installed it, so all three doors
// are listed every time.
func lines(current, latest string) []string {
	return []string{
		fmt.Sprintf("fleet update available: %s → %s", display(current), display(latest)),
		"  rerun your install command:",
		"    curl -fsSL https://raw.githubusercontent.com/zzacong/fleet/main/install.sh | sh",
		"    npm i -g @zzacong/fleet",
		"    go install github.com/zzacong/fleet/cmd/fleet@latest",
	}
}

// Render formats the notice for stderr: a yellow rounded box on a terminal,
// the same words as plain text otherwise (pipes, captures, NO_COLOR).
func Render(current, latest string, tty bool) string {
	body := lines(current, latest)
	if !tty || os.Getenv("NO_COLOR") != "" {
		return strings.Join(body, "\n")
	}
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true).Render(body[0])
	rest := strings.Join(body[1:], "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("3")).
		Padding(0, 1)
	return box.Render(title + "\n" + rest)
}

// FooterLine is the one-line TUI variant: the matrix footer has no room
// for the three-door hint, so it points at the install command generically.
func FooterLine(current, latest string) string {
	return fmt.Sprintf("fleet %s → %s available — rerun your install command", display(current), display(latest))
}
