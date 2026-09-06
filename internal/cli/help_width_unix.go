//go:build unix

package cli

import (
	"os"

	"golang.org/x/sys/unix"
)

// terminalWidth reports the column count of stdout via TIOCGWINSZ. The bool
// is false when stdout is not a terminal or the size is unknown.
func terminalWidth() (int, bool) {
	winsize, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || winsize.Col == 0 {
		return 0, false
	}
	return int(winsize.Col), true
}
