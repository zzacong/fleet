// The `u` key: one-key update-all. It runs the exact machinery the
// `fleet skill update` verb runs — the wrapped `skills update -g -y`
// (internal/skillscli), then the post-run sync (internal/sync) — and
// refreshes the matrix through the ordinary snapshot reload. The outcome
// is reported from fleet's own post-run state, never from the skills
// CLI's prose.

package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/zacong/fleet/internal/paths"
	"github.com/zacong/fleet/internal/scan"
	"github.com/zacong/fleet/internal/skillscli"
	fleetsync "github.com/zacong/fleet/internal/sync"
)

// newSkillsRunner builds the skills CLI runner: the real exec runner. A
// var so tests can script it — the skills binary is absent in sandboxes.
// internal/cli carries its own copy of this seam for its verbs; the tui
// package cannot import it back (cli wires the TUI into the command
// tree), so the seam var is duplicated, the runner it builds is not.
var newSkillsRunner = func() skillscli.Runner { return skillscli.Exec{} }

// updateMsg is a finished update-all run. It is never superseded: `u`
// starts an update only when no core operation is in flight, so at most
// one runs at a time and the message is applied whenever it lands.
type updateMsg struct {
	summary string // fleet's own post-run report, on success
	err     error  // the wrapped run's or the post-run sync's failure
}

// startUpdate runs the whole update-all: the wrapped skills CLI call, the
// post-run sync — the exact calls `fleet skill update` makes — and the
// own-state summary. The matrix refresh rides separately, through the
// same startRefresh path every other reload takes, so the notice set
// here survives it.
func startUpdate(m *model) tea.Cmd {
	m.phase = phaseUpdating
	p := m.p
	return func() tea.Msg {
		if _, err := skillscli.Update(newSkillsRunner()); err != nil {
			return updateMsg{err: err}
		}
		reports, err := fleetsync.Run(p)
		if err != nil {
			return updateMsg{err: fmt.Errorf("post-run sync: %w", err)}
		}
		summary, err := updateSummary(p, reports)
		if err != nil {
			return updateMsg{err: err}
		}
		return updateMsg{summary: summary}
	}
}

// applyUpdate lands the finished run. A failure leaves the matrix as it
// was — the verb's stop-on-failure rule, a half-finished update is the
// user's to resolve — with the raw output surfaced in the notice.
// Success sets the summary notice, then reloads the matrix so the new
// badges and states show while the notice survives the refresh.
func (m model) applyUpdate(msg updateMsg) (tea.Model, tea.Cmd) {
	m.phase = phaseIdle
	if msg.err != nil {
		m.notice = notice{text: updateFailure(msg.err), kind: noticeErr}
		return m, nil
	}
	m.notice = notice{text: msg.summary, kind: noticeGood}
	return m, startRefresh(&m, "")
}

// updateFailure renders a failed run the way the CLI surfaces one: the
// skills CLI's captured output verbatim — fleet never interprets it —
// with the failing invocation named. The notice line is one line, so the
// output's newlines flatten to spaces, and the invocation leads so the
// reason survives the notice line's truncation.
func updateFailure(err error) string {
	var cliErr *skillscli.Error
	if errors.As(err, &cliErr) && cliErr.Output != "" {
		out := strings.ReplaceAll(cliErr.Output, "\n", " ")
		return fmt.Sprintf("update failed: %v — %s", err, out)
	}
	return "update failed: " + err.Error()
}

// updateSummary builds the success notice from fleet's own post-run
// state — the canonical store scan and the skills CLI lockfile, the same
// summary `fleet skill update` prints — plus what the post-run sync had
// to repair, if anything. The skills CLI's prose is never parsed.
func updateSummary(p *paths.Paths, reports []fleetsync.Report) (string, error) {
	skills, err := scan.ScanStore(p.SkillsStore())
	if err != nil {
		return "", fmt.Errorf("scan canonical store: %w", err)
	}
	lock, err := scan.ReadLockfile(p.SkillLock())
	if err != nil {
		return "", fmt.Errorf("read skills lockfile: %w", err)
	}
	installed := scan.CountInstalled(skills, lock)
	line := fmt.Sprintf("updated all — %d skill%s in %s (%d installed, %d custom)",
		len(skills), plural(len(skills)), p.SkillsStore(), installed, len(skills)-installed)
	if s := syncNotice(reports); s != "" {
		line += " · " + s
	}
	return line, nil
}
