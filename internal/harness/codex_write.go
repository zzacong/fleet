// The codex write side: [[skills.config]] entries in ~/.codex/config.toml
// with name/enabled. TOML has no comment-preserving encoder, so fleet edits
// at the line level: blocks fleet recognizes (a header plus simple
// name/enabled lines) are flipped or removed; every other line passes
// through byte for byte. Entries fleet doesn't recognize — path selectors,
// extra keys, multi-line values, the single-table form — are flagged, not
// touched.

package harness

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// CanProject implements Adapter: [[skills.config]] is a config lever.
func (a *CodexAdapter) CanProject() bool { return true }

// Project implements Adapter.
func (a *CodexAdapter) Project(writes []SkillWrite) (WriteReport, error) {
	rep := WriteReport{}
	if err := validateNames(writes); err != nil {
		return rep, err
	}

	read, err := a.Read(writeNames(writes))
	if err != nil {
		return WriteReport{}, err
	}

	src, err := readConfig(a.home.CodexConfig())
	if err != nil {
		return WriteReport{}, err
	}
	lines := strings.Split(src, "\n")
	blocks := codexParseBlocks(lines)

	for _, w := range writes {
		before := read.States[w.Name]
		switch w.State {
		case StateOff:
			if before == StateOff {
				codexFlagSatisfied(blocks, w.Name, &rep)
				continue
			}
			if !codexFlipBlocks(lines, blocks, w.Name) {
				lines = codexAppendBlock(lines, w.Name)
			}
			if err := codexVerifyOff(lines, w.Name, &rep); err != nil {
				return WriteReport{}, err
			}
		case StateOn:
			if before != StateOff {
				continue
			}
			var removed bool
			lines, removed = codexRemoveBlocks(lines, blocks, w.Name)
			if !removed {
				// The disable comes from an entry fleet doesn't manage.
				rep.Flags = append(rep.Flags, Flag{Skill: w.Name, Message: "still disabled by a skills.config entry fleet doesn't manage — left alone"})
				continue
			}
			// Something fleet doesn't manage may still disable the skill.
			disabled, err := codexDisabled([]byte(strings.Join(lines, "\n")), w.Name)
			if err != nil {
				return WriteReport{}, err
			}
			if disabled {
				rep.Flags = append(rep.Flags, Flag{Skill: w.Name, Message: "still disabled by a skills.config entry fleet doesn't manage — left alone"})
				continue
			}
			rep.Changed = append(rep.Changed, Change{Skill: w.Name, From: StateOff, To: StateOn})
		}
	}

	flagUnmanagedCodex(blocks, writes, &rep)

	if err := writeIfChanged(a.home.CodexConfig(), src, strings.Join(lines, "\n")); err != nil {
		return WriteReport{}, err
	}
	return rep, nil
}

// flagUnmanagedCodex reports enabled = false entries no write covers.
// Blocks fleet can't model (path selectors, extra keys) are attributed to
// the skill they select when one can be recovered.
func flagUnmanagedCodex(blocks []codexBlock, writes []SkillWrite, rep *WriteReport) {
	written := writeNameSet(writes)
	for _, b := range blocks {
		if b.enabled != "false" {
			continue
		}
		skill := b.name
		if skill == "" {
			skill = b.pathSkill()
		}
		if skill == "" {
			flagForeign(rep, "a skills.config entry disables a skill fleet can't identify")
			continue
		}
		if written[skill] {
			continue // a write manages this skill
		}
		flagNotTracked(rep, skill)
	}
}

// codexFlagSatisfied reports a disable that already holds but isn't
// fleet's own simple name entry.
func codexFlagSatisfied(blocks []codexBlock, name string, rep *WriteReport) {
	winner, found := codexWinner(blocks, name)
	if found && winner.enabled == "false" && winner.simple && winner.name == name {
		return // fleet's own entry holds; state and config agree
	}
	rep.Flags = append(rep.Flags, Flag{Skill: name, Message: "still disabled by a skills.config entry fleet doesn't manage — left alone"})
}

// codexVerifyOff re-parses the rendered config with the read side's
// semantics and records the change, or flags when something else still
// keeps the skill enabled over fleet's disable.
func codexVerifyOff(lines []string, name string, rep *WriteReport) error {
	disabled, err := codexDisabled([]byte(strings.Join(lines, "\n")), name)
	if err != nil {
		return err
	}
	if disabled {
		rep.Changed = append(rep.Changed, Change{Skill: name, From: StateOn, To: StateOff})
		return nil
	}
	rep.Flags = append(rep.Flags, Flag{Skill: name, Message: "a skills.config entry fleet doesn't manage overrides fleet's disable — left alone"})
	return nil
}

// codexDisabled reports whether the config disables name, using the same
// parse the read side uses.
func codexDisabled(body []byte, name string) (bool, error) {
	disabled, err := codexDisabledEntries(body)
	if err != nil {
		return false, err
	}
	return disabled[name], nil
}

var (
	codexHeaderRe  = regexp.MustCompile(`^\s*\[\[\s*skills\.config\s*\]\]\s*(?:#.*)?$`)
	codexNameRe    = regexp.MustCompile(`^\s*name\s*=\s*"((?:[^"\\]|\\.)*)"\s*(?:#.*)?$`)
	codexNameLitRe = regexp.MustCompile(`^\s*name\s*=\s*'([^']*)'\s*(?:#.*)?$`)
	codexPathRe    = regexp.MustCompile(`^\s*path\s*=\s*"((?:[^"\\]|\\.)*)"\s*(?:#.*)?$`)
	codexEnabledRe = regexp.MustCompile(`^(\s*enabled\s*=\s*)(true|false)(.*)$`)
)

// codexBlock is one [[skills.config]] table as fleet sees it: the line
// span it owns, the selectors it sets, and whether fleet may edit it.
type codexBlock struct {
	start, end  int // line indexes, end exclusive; includes trailing blanks
	name        string
	path        string
	enabled     string // "true", "false", or "" when absent
	enabledLine int    // index of the enabled = line, -1 when absent
	simple      bool   // body holds only comments, blanks, and single-line name/path/enabled entries
}

// pathSkill recovers the skill name from a path selector pointing at
// .../skills/<name>/SKILL.md, mirroring the read side.
func (b codexBlock) pathSkill() string {
	if b.path == "" {
		return ""
	}
	return filepath.Base(filepath.Dir(b.path))
}

// codexParseBlocks scans config lines for [[skills.config]] tables. Any
// other header ends the current block; lines outside such tables are not
// tracked at all.
func codexParseBlocks(lines []string) []codexBlock {
	var blocks []codexBlock
	var cur *codexBlock
	finish := func(i int) {
		if cur != nil {
			cur.end = i
			blocks = append(blocks, *cur)
			cur = nil
		}
	}
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			finish(i)
			if codexHeaderRe.MatchString(line) {
				cur = &codexBlock{start: i, end: i, enabledLine: -1, simple: true}
			}
			continue
		}
		if cur != nil {
			cur.absorb(line, i)
		}
	}
	finish(len(lines))
	return blocks
}

// absorb folds a body line into the block, extracting selectors and
// downgrading simplicity on anything fleet wouldn't have written.
func (b *codexBlock) absorb(line string, i int) {
	b.end = i + 1

	if m := codexNameRe.FindStringSubmatch(line); m != nil {
		b.name = codexUnquote(`"` + m[1] + `"`)
		return
	}
	if m := codexNameLitRe.FindStringSubmatch(line); m != nil {
		b.name = m[1]
		return
	}
	if m := codexPathRe.FindStringSubmatch(line); m != nil {
		b.path = codexUnquote(`"` + m[1] + `"`)
		return
	}
	if m := codexEnabledRe.FindStringSubmatch(line); m != nil {
		b.enabled, b.enabledLine = m[2], i
		return
	}
	// A key fleet doesn't model, or a multi-line value: codex still reads
	// it as part of this table, so the block stays in place — fleet just
	// no longer edits it.
	if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "#") {
		b.simple = false
	}
}

// codexUnquote decodes a quoted TOML basic string.
func codexUnquote(quoted string) string {
	s, ok := jsonString(quoted)
	if !ok {
		return ""
	}
	return s
}

// codexWinner returns the last block selecting name (by name or path);
// codex lets later entries override earlier ones.
func codexWinner(blocks []codexBlock, name string) (codexBlock, bool) {
	var winner codexBlock
	found := false
	for _, b := range blocks {
		if b.name == name || (b.path != "" && b.pathSkill() == name) {
			winner, found = b, true
		}
	}
	return winner, found
}

// codexFlipBlocks turns enabled = true into false on every simple
// fleet-shape block for name. It reports whether anything was flipped.
// Line contents are replaced in place; block boundaries don't move.
func codexFlipBlocks(lines []string, blocks []codexBlock, name string) bool {
	flipped := false
	for _, b := range blocks {
		if !b.simple || b.name != name || b.enabledLine < 0 || b.enabled == "false" {
			continue
		}
		lines[b.enabledLine] = codexEnabledRe.ReplaceAllString(lines[b.enabledLine], "${1}false${3}")
		flipped = true
	}
	return flipped
}

// codexRemoveBlocks deletes every simple fleet-shape block for name and
// reports whether any were removed. Blocks are visited last-first so the
// line indexes of the remaining ones stay valid.
func codexRemoveBlocks(lines []string, blocks []codexBlock, name string) ([]string, bool) {
	removed := false
	for i := len(blocks) - 1; i >= 0; i-- {
		b := blocks[i]
		if !b.simple || b.name != name {
			continue
		}
		lines = append(lines[:b.start], lines[b.end:]...)
		lines = codexTidySeam(lines, b.start)
		removed = true
	}
	return lines, removed
}

// codexTidySeam collapses the doubled blank lines a block removal can
// leave at the seam and trims trailing blanks to one newline.
func codexTidySeam(lines []string, at int) []string {
	if at > 0 && at < len(lines) &&
		strings.TrimSpace(lines[at-1]) == "" && strings.TrimSpace(lines[at]) == "" {
		lines = append(lines[:at-1], lines[at:]...)
	}
	for len(lines) > 1 &&
		strings.TrimSpace(lines[len(lines)-1]) == "" &&
		strings.TrimSpace(lines[len(lines)-2]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// codexAppendBlock adds a fleet block at the end of the file, separated by
// one blank line. An empty file gets just the block.
func codexAppendBlock(lines []string, name string) []string {
	block := []string{
		"[[skills.config]]",
		fmt.Sprintf("name = %s", codexQuote(name)),
		"enabled = false",
	}
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			if strings.TrimSpace(lines[len(lines)-1]) != "" {
				lines = append(lines, "") // separator after trailing content
			}
			lines = append(lines, block...)
			return append(lines, "")
		}
	}
	return append(block, "")
}

// codexQuote encodes s as a TOML basic string (JSON escaping is a valid
// subset for the values fleet writes).
func codexQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return string(b)
}
