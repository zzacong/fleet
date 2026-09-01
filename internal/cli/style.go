// Shared text styling for the CLI's human output: one palette so every
// command speaks the same colors, and the helpers to lay out aligned
// columns without tabwriter (ANSI styles inside cells would throw off its
// width math). Styles are the identity when stdout isn't a terminal, so
// pipes and captures get clean plain text — the same words, no escapes.

package cli

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// palette holds the CLI's text styles.
type palette struct {
	broken func(string) string // red — needs fixing before anything works
	warn   func(string) string // yellow — sync or the user should act
	info   func(string) string // cyan — metadata, informational
	good   func(string) string // green — healthy
	dim    func(string) string // faint — annotations, legends, noise
	bold   func(string) string
}

func newPalette(tty bool) palette {
	identity := func(s string) string { return s }
	if !tty {
		return palette{broken: identity, warn: identity, info: identity, good: identity, dim: identity, bold: identity}
	}
	color := func(c string) func(string) string {
		st := lipgloss.NewStyle().Foreground(lipgloss.Color(c))
		return func(s string) string { return st.Render(s) }
	}
	return palette{
		broken: color("1"),
		warn:   color("3"),
		info:   color("6"),
		good:   color("2"),
		dim:    func(s string) string { return lipgloss.NewStyle().Faint(true).Render(s) },
		bold:   func(s string) string { return lipgloss.NewStyle().Bold(true).Render(s) },
	}
}

// padRight pads s with spaces to width w (runes, not bytes — the strings
// are plain, the styling happens after padding).
func padRight(s string, w int) string {
	if n := len([]rune(s)); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}
