package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// The hero ASCII banner. It belongs to the TUI launch: plain verbs carry a
// one-line header instead, and --quiet (or a pipe, which never reaches the
// TUI at all) suppresses it here.

var bannerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))

const bannerArt = `
 ███████╗██╗     ███████╗███████╗████████╗
 ██╔════╝██║     ██╔════╝██╔════╝╚══██╔══╝
 █████╗  ██║     █████╗  █████╗     ██║
 ██╔══╝  ██║     ██╔══╝  ██╔══╝     ██║
 ██║     ███████╗██║     ███████╗   ██║
 ╚═╝     ╚══════╝╚═╝     ╚══════╝   ╚═╝
`

// bannerLines is the banner as one string per line, with the art indented
// under a tagline. The view budget counts these lines, so the body shrinks
// by exactly this much when the banner shows.
func bannerLines() []string {
	art := strings.Split(strings.TrimRight(strings.TrimPrefix(bannerArt, "\n"), "\n"), "\n")
	lines := make([]string, 0, len(art)+1)
	for _, line := range art {
		lines = append(lines, bannerStyle.Render(strings.TrimRight(line, " ")))
	}
	lines = append(lines, styDim.Render("fleet · per-harness agent skill manager"))
	return lines
}
