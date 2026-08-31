# Prototype B build notes — OpenTUI + TypeScript (Bun)

## What I took over and fixed

The previous agent finished the CLI but stalled before TUI smoke testing. Review
against PLAN.md found five issues, all fixed:

1. **`<span>` inside a `<box>` crashed the app.** The apply/refresh Spinner
   mounted a `span` directly under the footer box. OpenTUI throws `Component of
   type "span" must be created inside of a text node`; the ErrorBoundary then
   blanked the screen and the app looked frozen — this is exactly why the
   previous agent's smoke test stalled. Spinner now renders a `text`.
2. **esc could not cancel an apply.** The key switch matched `case "escape"`
   against `key.sequence`, but OpenTUI delivers ESC with the raw sequence
   `"\x1b"` (it normalizes arrows to `"down"` but not ESC to `"escape"`), so the
   branch was dead code. Worse, the store's apply chain used `await delay()`
   with no cancellation, so even a correct esc would only have hidden the
   preview while the flip still fired. Apply is now a cancellable `setTimeout`
   (`cancelApply()`), matching the Ink and Go prototypes ("apply cancelled").
3. **Filter input display broke on every keystroke.** OpenTUI's `<input>` shows
   only the last typed character once the parent re-renders per keystroke
   (uncontrolled internal buffer vs reconciler). Filter was hand-rolled as a
   plain `text` + `useKeyboard` append/backspace — the PLAN expected the list to
   be hand-rolled anyway; the input component isn't trustworthy at 0.5.9.
4. **Bursted key repeats read stale state.** `move()` derived the cursor from a
   committed closure, so a held `j` (spec scenario 5) moved one row per render,
   not per key. Cursor now mirrors into a synchronous ref; filter appends use
   functional `setState`. A 40-key unflushed burst now scrolls 40 rows.
5. **Small spec/CLI gaps.** `g`/`G` jump keys were feature creep (removed — not
   in the shared spec, absent from the other two prototypes); `skillctl tui`
   lacked `--bench`; `--rows=500` (equals form) didn't parse in the standalone
   entry; the CLI table's roots column was ragged because `formatRowParts`
   computed the roots width per row (now takes a fixed width).

## API surprises from 0.5.9

- `span` must live inside `text`; there is no inline-styled text outside one.
- The key event duality is inconsistent: `key.sequence` is the raw byte(s) for
  ESC (`"\x1b"`) but a normalized name for arrows (`"down"`, `"up"`). Match on
  both, or use `key.name` for named keys.
- `createTestRenderer` + `@opentui/react/test-utils` (`testRender`, `mockInput`,
  `captureCharFrame`) are excellent — I verified all 30+ behaviors
  deterministically against captured character frames instead of parsing ANSI.
  Quirks: the first committed buffer decodes as garbage (U+0A00 per cell) until
  after the first keypress, a lone ESC byte lingers in the parser's
  sequence-disambiguation window (~20ms timeout) before emitting, and rapid
  `mockInput` writes get dropped without flushes in between (single-write bursts
  of printable keys work fine; bursts of multi-byte arrow sequences don't —
  harness artifact, not app behavior).
- `captureCharFrame` was fixed by rendering + keypress warm-up; partial-diff
  redraws make raw pty transcripts nearly unparsable for assertions.

## Smoke testing what fought me

`script -q /dev/null` gives a 0x0 pty (the classic failure mode); the working
recipe was Python `pty.fork` + `TIOCSWINSZ` 120x40 + timed keystroke writes.
The pty runs "froze" until the Spinner crash was fixed — the app wasn't slow,
it was dead. After the fixes all pty scenarios exit 0: render+q, stage-3+apply
with visible spinner/preview/"applied 3 change(s)", filter, help, refresh.

## Commander under Bun

A non-event: identical code shape to the Ink prototype, `parseAsync` just works,
`process.exit(1)` for unknown skills behaves. The only Bun-ism is `import.meta.dir`
for fixture resolution (nice — no `fileURLToPath` dance).

## Verification summary

- `bunx tsc --noEmit`: clean. All CLI commands re-verified after edits (exit
  codes, `--rows 60`, `--json`, unknown command → 1).
- `bun run bench`: 38.2–41.6ms warm (5 runs); `--rows 500 --bench`: 44.4ms.
  45 and 500 rows render and scroll; held-`j` burst scrolls the window.

## Deliberately not built

- **Mouse support** — OpenTUI enables mouse tracking by default and a hit-test
  API exists; tempting, skipped per the no-creep rule.
- A virtualized list component — the hand-rolled window (36 visible rows) is
  fine at 500; real virtualization would be step one of the real tool.
- Search highlighting of filter matches in rows.

## Stats

- Lines: `src/store.ts` 249, `src/main.tsx` 262, `src/cli.ts` 146 — 657 total.
- Versions: @opentui/core 0.5.9, @opentui/react 0.5.9, commander 15.0.0,
  typescript 7.0.2, @types/bun 1.4.0, Bun 1.4.0.
