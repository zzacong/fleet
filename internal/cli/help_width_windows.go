//go:build windows

package cli

// terminalWidth has no direct Windows implementation: help wrapping falls
// back to the 80-column default.
func terminalWidth() (int, bool) {
	return 0, false
}
