//go:build unix

package cli

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// terminalWidth reports the column count of stdout via TIOCGWINSZ. The bool
// is false when stdout is not a terminal or the size is unknown.
func terminalWidth() (int, bool) {
	var winsize struct {
		rows, cols, xpixel, ypixel uint16
	}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, os.Stdout.Fd(), unix.TIOCGWINSZ,
		uintptr(unsafe.Pointer(&winsize)))
	if errno != 0 || winsize.cols == 0 {
		return 0, false
	}
	return int(winsize.cols), true
}
