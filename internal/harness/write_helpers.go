// Write-side helpers shared by the adapter implementations: reading config
// text, parsing it tolerantly, writing back only when it changed, and
// small JSON/TOML utilities.

package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zzacong/fleet/internal/jsonc"
)

// readConfig returns the file's text; empty when the file is missing.
func readConfig(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(body), nil
}

// parseJSONConfig parses JSONC config text, treating a missing or
// whitespace-only file as an empty object document.
func parseJSONConfig(src string) (*jsonc.Document, error) {
	if strings.TrimSpace(src) == "" {
		src = "{}"
	}
	return jsonc.Parse(src)
}

// parseStrictJSONConfig is parseJSONConfig for files whose harness
// requires strict JSON: comments and trailing commas are errors, so a
// strict writer can never emit them.
func parseStrictJSONConfig(src string) (*jsonc.Document, error) {
	if strings.TrimSpace(src) == "" {
		src = "{}"
	}
	return jsonc.ParseStrict(src)
}

// renderConfig renders an edited document; a fresh file (no prior source)
// ends with a newline like any hand-maintained config.
func renderConfig(src string, doc *jsonc.Document) string {
	out := doc.Render()
	if strings.TrimSpace(src) == "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

// writeIfConfigChanged writes the rendered config only when it differs
// from the source in a meaningful way: an existing file is rewritten on
// any change; a missing (or whitespace-only) file is only created when the
// document gained content, so a no-op sync never creates files.
func writeIfConfigChanged(path, src, out string) error {
	if strings.TrimSpace(src) == "" {
		switch strings.TrimSpace(out) {
		case "", "{}", "[]":
			return nil
		}
	}
	return writeIfChanged(path, src, out)
}

// writeIfChanged rewrites path only when the text differs, preserving an
// existing file's mode. Skipping unchanged writes keeps sync from churning
// mtimes on every command.
func writeIfChanged(path, before, after string) error {
	if before == after {
		return nil
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(after), mode)
}

// writeNames extracts just the skill names from a write batch.
func writeNames(writes []SkillWrite) []string {
	names := make([]string, len(writes))
	for i, w := range writes {
		names[i] = w.Name
	}
	return names
}

// writeNameSet is writeNames as a set, covering every written skill
// regardless of direction.
func writeNameSet(writes []SkillWrite) map[string]bool {
	set := make(map[string]bool, len(writes))
	for _, w := range writes {
		set[w.Name] = true
	}
	return set
}

// isGlobPattern reports whether s uses glob metacharacters, distinguishing
// pattern rules from exact-name entries.
func isGlobPattern(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// flagNotTracked is the standard flag for an exact-name disable entry that
// fleet's state file doesn't track.
func flagNotTracked(rep *WriteReport, skill string) {
	rep.Flags = append(rep.Flags, Flag{Skill: skill, Message: "disabled in config but not tracked by fleet's state — left alone"})
}

// flagForeign is the standard flag for a disable entry fleet can't own.
func flagForeign(rep *WriteReport, message string) {
	rep.Flags = append(rep.Flags, Flag{Message: message + " — left alone"})
}

// patternCoversWrite reports whether a pattern rule matches any skill a
// write manages, whose report already accounts for the rule.
func patternCoversWrite(pattern string, writes []SkillWrite) bool {
	for _, w := range writes {
		if matchPattern(pattern, w.Name) {
			return true
		}
	}
	return false
}

// jsonString decodes a JSON string literal; ok=false for anything else.
func jsonString(raw string) (string, bool) {
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return "", false
	}
	return s, true
}

// validateNames rejects names that cannot be targeted by a harness rule:
// skill names become path segments and pattern-free exact entries.
func validateNames(writes []SkillWrite) error {
	for _, w := range writes {
		if w.Name == "" {
			return fmt.Errorf("skill name must not be empty")
		}
		if w.State != StateOn && w.State != StateOff {
			return fmt.Errorf("unsupported desired state %q for skill %q", w.State, w.Name)
		}
	}
	return nil
}
