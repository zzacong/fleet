// The A key: archive the selected custom skill. A asks for confirmation,
// then runs the same core verb the CLI runs, then sync, then refreshes the
// matrix. A refusal from the core is about one row, so it shows under that
// row rather than on the notice line.

package tui

import (
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/zzacong/fleet/internal/customs"
	"github.com/zzacong/fleet/internal/skillindex"
	fleetsync "github.com/zzacong/fleet/internal/sync"
)

// archiveAsk is an archive waiting on y: the custom copy the row showed when
// A was pressed, so the confirmed move is the copy on screen.
type archiveAsk struct {
	name string
	hit  skillindex.Hit
}

// inlineError is a refusal drawn under the row it names. The next key clears
// it, like a notice.
type inlineError struct {
	skill string
	text  string
}

// askArchive answers A. An installed row gets a notice and nothing else,
// since the skills CLI owns installed skills. A custom row opens the prompt
// on the copy it displays: the highest-precedence custom copy of the name.
func (m model) askArchive() (tea.Model, tea.Cmd) {
	name := m.selectedName()
	if name == "" {
		m.notice = notice{text: "no skill selected", kind: noticeInfo}
		return m, nil
	}
	if !m.isCustom(name) {
		m.notice = notice{text: fmt.Sprintf("%q is installed — archive moves only custom skills; remove it with the skills CLI", name), kind: noticeInfo}
		return m, nil
	}
	copies, err := customs.CustomCopies(m.p, name)
	if err != nil {
		m.notice = notice{text: "archive failed: " + err.Error(), kind: noticeErr}
		return m, nil
	}
	if len(copies) == 0 {
		m.notice = notice{text: fmt.Sprintf("archive: %q not found among custom skills", name), kind: noticeErr}
		return m, nil
	}
	hit := copies[0]
	m.archiveAsk = &archiveAsk{name: name, hit: hit}
	m.notice = notice{text: fmt.Sprintf("archive %s? y confirms, any other key cancels", filepath.Join(hit.Home, name)), kind: noticeInfo}
	return m, nil
}

// archive runs the confirmed move: the core verb, then sync, the two steps
// the CLI verb takes. A refusal leaves the matrix as it was and shows under
// the row. A move that landed reports its unlinks and sync, then refreshes.
func (m model) archive(ask *archiveAsk) (tea.Model, tea.Cmd) {
	arc, err := customs.Archive(m.p, ask.hit)
	if err != nil {
		m.inlineErr = inlineError{skill: ask.name, text: err.Error()}
		m.scrollToCursor()
		return m, nil
	}
	text := fmt.Sprintf("archived %q to %s · %d link%s removed", ask.name, arc.To, len(arc.Unlinked), plural(len(arc.Unlinked)))
	reports, err := fleetsync.Run(m.p)
	if err != nil {
		m.notice = notice{text: text + " · sync failed: " + err.Error(), kind: noticeErr}
	} else {
		if s := syncNotice(reports); s != "" {
			text += " · " + s
		}
		m.notice = notice{text: text, kind: noticeGood}
	}
	return m, startRefresh(&m, "", reloadCached)
}
