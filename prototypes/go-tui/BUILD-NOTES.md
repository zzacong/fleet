# Build notes — Prototype C (Go + Bubble Tea v2 + Cobra)

Written while building, per PLAN.md. Supplement to prototypes/NOTES.md (feel-test
impressions go there; this file is build friction and API detail).

## Versions pinned

- Go 1.27.0 darwin/arm64
- charm.land/bubbletea/v2 v2.0.9
- charm.land/bubbles/v2 v2.2.1 (textinput, spinner)
- charm.land/lipgloss/v2 v2.0.6
- github.com/spf13/cobra v1.10.2

Total: 743 lines of Go (260 main.go CLI, 483 tui.go). The plan predicted
250–300 for the TUI; I came in higher mostly because of hand-rolled row
layout/truncation and the help overlay (~60 lines combined), not because of
message plumbing.

## API surprises (v2 vs the plan sketch)

- The plan sketch was accurate on the two headline changes: `View()` returns
  `tea.View` (build it with `tea.NewView(s)`, set `v.AltScreen = true` on it)
  and key input arrives as `tea.KeyPressMsg` with `msg.String()` matching.
  There is no `WithAltScreen` program option anymore; alt screen is a field on
  the returned View. That feels odd at first but is fine once you notice it.
- **The real gotcha: space is `"space"`, not `" "`.** Under the Kitty keyboard
  protocol (v2 enables it by default), `KeyPressMsg.String()` for the space bar
  returns `"space"`. I matched `case " "` and staging silently never fired. It
  only surfaced because I scripted keystrokes through a pty and checked the
  staged-count status line. If you eyeball-test in a real terminal you might
  notice it immediately; from code review you will not. Match both.
- `bubbles/v2/textinput`: `Update` has a value receiver but `Focus`, `Blur`,
  `SetValue`, `SetWidth`, `Reset` are pointer receivers — the usual Go
  awkwardness when your model methods are value-receiver Elm style. Works fine
  since `m` is an addressable local in `Update`, but it reads inconsistently.
- `spinner.Tick` is a method used directly as a `tea.Cmd` (`m.spinner.Tick`),
  same as v1. Unchanged.
- `tea.Tick(d, fn)` unchanged from v1 shape; used for the 300 ms apply and the
  refresh flash.

## Docs quality

Good. The UPGRADE_GUIDE_V2.md in each repo (bubbletea and bubbles) is genuinely
useful and current. The pkg.go.dev landing for charm.land modules resolves
correctly. I did not need to read source — except for the space key, where the
String() doc comment never mentions that printable-text keys short-circuit to
their text and space deliberately does not (`k.Text != " "` in Key.String()).
That one cost a debug cycle.

## Testing without a TTY (the part the plan doesn't warn you about)

`printf 'q' | script -q /dev/null go run . tui` does NOT work: `script` gives
the pty a 0x0 window size when stdin is a pipe, WindowSizeMsg reports 0x0, and
the renderer emits only clears — the frame is never visible and you learn
nothing. I wrote a ~50-line Python pty driver (`pty.fork` + `TIOCSWINSZ` +
timed key writes) to drive the TUI: filter, stage, apply, esc-cancel, help,
refresh, `--rows 500`, and `--bench` all verified that way. Second lesson from
that harness: v2's renderer does diff-based partial repaints, so grepping raw
output for a full status line is unreliable after the first frame — verify
against changed cells or structured frames instead.

## Cobra vs the plan

Cobra is the most "framework" of anything here. For the required command set
(7 commands + `root list`), the tree declared itself in ~40 lines and
everything the plan demanded came free: `-h/--help` everywhere, unknown-command
errors with exit 1 (`Error: unknown command "x" for "skillctl"`), `Args`
validation, help generated per command. Two small notes: the auto-added
`completion` command shows up in help output (noise for a prototype; can be
hidden with `CompletionOptions.DisableDefaultCmd` — didn't bother), and a root
command with a `Run` is what makes bare `skillctl` print help rather than
error. Cobra's error style (`Error: unknown skill: foo` on stderr, exit 1,
usage suppressed via `SilenceUsage`) needed no code. Commander comparison is
for the other prototypes' notes, but Cobra's opinionated structure meant zero
decision fatigue — everything has a place whether you want it or not.

## Ceremony vs safety, typed messages

- The compiler checking message types is real: `applyDoneMsg{seq, count}` and
  `refreshedMsg` can't be confused. But the phases themselves are still an
  int enum I switch by hand — the type system catches unhandled *messages*, not
  unhandled *phase × key* combinations.
- Cancellation was the one place ceremony earned its keep: esc during the
  300 ms apply would otherwise let the pending `tea.Tick` fire and apply
  anyway. A generation counter (`seq`) in the model + carried in the message
  solved it in ~6 lines. In an async/await world (the TS prototypes) that bug
  is an `AbortController` you will probably forget to wire.
- Value-receiver `Update(msg) (tea.Model, tea.Cmd)` means every mutation is
  "mutate the local copy and return it" — 743 lines in partly because of the
  return-value plumbing. Fine, just verbose.

## Things I was tempted by and did NOT build (per ground rules)

- Bubbles `table` or `viewport` for the list — the plan's known-risk note says
  it gives scrolling/virtualization free, but hand-rolling a scroll window was
  ~25 lines and kept full control of row rendering. At 500 rows it was fine.
- Mouse support, fuzzy matching, left/right cursor for the filter column
  layout, persisted staged set, colored status segments beyond the spec, a
  `--json --rows` combination (they compose; --json ignores --rows by design).
- Hiding Cobra's `completion` command.

## Bench (single run here; five-run pass belongs to NOTES.md)

`skillctl tui --bench`: startup→first frame ≈ 1 ms (probe cmd fired from
`Init`, i.e., after the first render). Binary startup dominates nothing; there
is effectively no cold-start cost.
