package harness

import "os"

// isDir probes for a harness's config directory (detection).
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// linkPresent reports whether a path exists, following the name not the
// target: a broken symlink still counts as present (that's drift for
// doctor, not absence).
func linkPresent(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// onForAll seeds every requested skill with StateOn, the default for
// harnesses that read the canonical store natively.
func onForAll(names []string) map[string]State {
	states := make(map[string]State, len(names))
	for _, name := range names {
		states[name] = StateOn
	}
	return states
}
