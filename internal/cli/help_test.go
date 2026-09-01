// Help screens re-wrap long command descriptions at the description column
// instead of letting the terminal wrap them back to column 0. The tests pin
// the terminal width so the wrapping is deterministic.

package cli

import (
	"strings"
	"testing"
)

func pinHelpWidth(t *testing.T, width int) {
	t.Helper()
	prev := helpWidth
	helpWidth = func() int { return width }
	t.Cleanup(func() { helpWidth = prev })
}

func TestWrapCommandIndentsContinuationAtDescriptionColumn(t *testing.T) {
	pinHelpWidth(t, 40)

	got := wrapCommand("doctor", 11, "Report what's wrong: drift, redundant links, broken links, unknown entries, manual edits")
	lines := strings.Split(got, "\n")

	// The description starts after the padded name column; every wrapped
	// line must resume at that same column, not column 0.
	const descColumn = 2 + 11 + 1
	for i, line := range lines[1:] {
		if got := len(line) - len(strings.TrimLeft(line, " ")); got != descColumn {
			t.Errorf("continuation line %d indented to column %d, want %d:\n%q", i+1, got, descColumn, line)
		}
	}
}

func TestWrapCommandKeepsRowsWithinTerminalWidth(t *testing.T) {
	pinHelpWidth(t, 40)

	got := wrapCommand("doctor", 11, "Report what's wrong: drift, redundant links, broken links, unknown entries, manual edits")
	for i, line := range strings.Split(got, "\n") {
		if len(line) > 40 {
			t.Errorf("line %d is %d columns, over the 40-column terminal:\n%q", i, len(line), line)
		}
	}
}

func TestWrapCommandShortDescriptionStaysOnOneLine(t *testing.T) {
	pinHelpWidth(t, 80)

	got := wrapCommand("ls", 11, "List every skill and where it is active")
	if strings.Contains(got, "\n") {
		t.Errorf("short description should not wrap:\n%q", got)
	}
	if want := "ls          List every skill and where it is active"; got != want {
		t.Errorf("wrapCommand = %q, want %q", got, want)
	}
}

func TestHelpScreenWrapsDescriptionsAtDescriptionColumn(t *testing.T) {
	out, err := runBare(t, toggleHome(t), "skill", "--help")
	if err != nil {
		t.Fatalf("skill --help: %v", err)
	}

	// Piped output falls back to 80 columns, so `doctor`'s long description
	// wraps. Inside the command list, every continuation line must line up
	// with the description column of the row above it.
	const sectionStart = "Available Commands:"
	const sectionEnd = "Flags:"
	section := false
	descColumn := 0
	for line := range strings.SplitSeq(out, "\n") {
		if line == sectionStart {
			section = true
			continue
		}
		if line == sectionEnd {
			break
		}
		if !section || strings.TrimSpace(line) == "" {
			continue
		}
		switch indent := len(line) - len(strings.TrimLeft(line, " ")); {
		case indent == 2:
			// A command row: its description column is where the first
			// character after the padded command name lands.
			name := strings.Fields(line)[0]
			descColumn = len(line) - len(strings.TrimLeft(line[2+len(name):], " "))
		default:
			if got := indent; got != descColumn {
				t.Errorf("continuation line indented to column %d, want %d:\n%q", got, descColumn, line)
			}
		}
	}
	if descColumn == 0 {
		t.Fatalf("no command rows found in help output:\n%s", out)
	}
}
