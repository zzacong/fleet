// Package scan reads the canonical store (~/.agents/skills) and the skills
// CLI lockfile (~/.agents/.skill-lock.json). It lists what is installed and
// classifies each skill as custom (no lock entry) or installed (lock entry
// with provenance). It never writes.
package scan

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill is one directory in the canonical store.
type Skill struct {
	// Name is the frontmatter name, falling back to the directory name when
	// the frontmatter omits one. Harness rules target this name.
	Name string
	// Dir is the directory name in the store. The skills CLI keys its
	// lockfile by it.
	Dir string
	// Description is the frontmatter description, empty when absent.
	Description string
}

// ScanStore lists every skill directory (immediate child of store containing
// a SKILL.md) with its name and description. Dot directories, plain files,
// and directories without a SKILL.md are skipped. A missing store is not an
// error; it just yields no skills.
func ScanStore(store string) ([]Skill, error) {
	entries, err := os.ReadDir(store)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []Skill
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		mdPath := filepath.Join(store, entry.Name(), "SKILL.md")
		body, err := os.ReadFile(mdPath)
		if err != nil {
			continue // no SKILL.md: not a skill directory
		}
		fm := parseFrontmatter(string(body))
		name := fm.name
		if name == "" {
			name = entry.Name()
		}
		skills = append(skills, Skill{
			Name:        name,
			Dir:         entry.Name(),
			Description: fm.description,
		})
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Dir < skills[j].Dir })
	return skills, nil
}

type frontmatter struct {
	name        string
	description string
}

// parseFrontmatter extracts name and description from a SKILL.md's YAML
// frontmatter. It handles the subset the Agent Skills spec uses: plain or
// quoted single-line values, folded/literal block scalars, CRLF line ends,
// and unknown keys (ignored). No frontmatter yields zero values.
func parseFrontmatter(body string) frontmatter {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")

	// The frontmatter block must open on the first line.
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return frontmatter{}
	}

	var fm frontmatter
	var blockKey string
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		if trimmed == "" {
			continue
		}

		// Continuation of a block scalar: more deeply indented lines.
		if blockKey != "" && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			value := joinBlockScalar(fm.value(blockKey), trimmed)
			fm.setValue(blockKey, value)
			continue
		}
		blockKey = ""

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		switch value {
		case ">", ">-", "|", "|-", "|+":
			// Block scalar opener; continuation lines follow.
			blockKey = key
			fm.setValue(key, "")
			continue
		}

		if quoted, ok := unquote(value); ok {
			value = quoted
		} else if idx := strings.Index(value, " #"); idx >= 0 {
			value = value[:idx] // trailing comment on an unquoted value
		}

		fm.setValue(key, strings.TrimSpace(value))
	}
	return fm
}

func (f *frontmatter) value(key string) string {
	switch key {
	case "name":
		return f.name
	case "description":
		return f.description
	}
	return ""
}

func (f *frontmatter) setValue(key, value string) {
	switch key {
	case "name":
		f.name = value
	case "description":
		f.description = value
	}
}

// joinBlockScalar appends a continuation line to a block scalar, folding
// lines with single spaces (close enough for one-line display).
func joinBlockScalar(current, line string) string {
	if current == "" {
		return line
	}
	return current + " " + line
}

// unquote strips surrounding double or single quotes and resolves the double
// quote escapes YAML allows (\" and \\). It reports whether the value was
// quoted at all.
func unquote(value string) (string, bool) {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		inner := value[1 : len(value)-1]
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		return inner, true
	}
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'"), true
	}
	return value, false
}
