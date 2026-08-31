# Build notes: Prototype A (Ink + TypeScript)

Built per `../PLAN.md` against the shared fixture. 542 lines of src/ total
(store 169, TUI 245, CLI 128) — within the plan's 200-260 TUI + 60 CLI
estimate, once you add the shared hook and the rows-expansion/table helpers.

## Friction encountered

1. **pnpm 11 blocked esbuild's postinstall** (`ERR_PNPM_IGNORED_BUILDS`), and
   pnpm 11 renamed the approval setting: `pnpm.onlyBuiltDependencies` in
   package.json is silently ignored ("no longer read"), `onlyBuiltDependencies`
   in pnpm-workspace.yaml also didn't work — the new home is an `allowBuilds:`
   map in pnpm-workspace.yaml (`esbuild: true`). Cost ~15 min of archaeology.
   Also note `pnpm exec`/`pnpm run` re-run an install check that fails hard
   until the build is approved, so even `tsc` wouldn't run.
2. **Ink version pinning nit:** `ink-spinner@^5.0.1` does not exist (5.0.0 is
   latest). Docs don't advertise that the spinner package versions 1:1 with
   Ink majors, so I guessed wrong.
3. **pnpm forwards the literal `--` separator into the script's argv**
   (`pnpm cli -- list --json` → `tsx src/cli.ts -- list --json`), and Commander
   then treats `--json` as belonging to nothing (it parsed `list` but dropped
   the flag). Fixed in cli.ts by stripping a leading `--` before
   `parseAsync(argv, {from: "user"})`. Cobra would have the same problem; it's
   a runner quirk, not a framework one, but the PLAN's `cli -- list` shape
   forces you to handle it.
4. **tsx's CLI spawns a child node process with piped stdio**, so under a pty
   the Ink app saw `stdin.isTTY === false` and disabled `useInput` entirely —
   keys were silently swallowed. Running `node --import tsx src/main.tsx`
   directly keeps the real TTY. Relevant for anyone scripting the TUI.
5. **Ink treats one stdin chunk as one keypress.** A burst of 60 `j` bytes in a
   single write arrives as one `input: "jjjj…"` string; `input === "j"` fails
   and nothing scrolls. Real terminals send key repeats as separate reads, so
   held-`j` is fine, but programmatic bursts are not. Worth remembering when
   comparing scroll feel across stacks.
6. **`--bench` needs care to exit cleanly.** `performance.now()` conveniently
   measures from process start (including tsx transpile), which is exactly the
   "startup→first frame" number. Unmounting from the first-frame `useEffect`
   exits cleanly — no `process.exit` needed.

## API notes

- ink-text-input and ink-spinner both ship TypeScript types; no @types needed.
- Composition of `useInput` + a focused text input works well: all useInput
  handlers see every key, so the App-level handler just guards on
  `filterFocused` and lets TextInput own the keystrokes. Esc reaches the
  App handler even while the input is focused.
- `useStdout().stdout.columns/rows` update on resize; derived the row window
  from those, so --rows 500 renders a scroll-into-view slice, not 500 lines.

## Deliberately not built (per no-creep rule)

- Mouse support (Ink 5 has none built in anyway — would need ink-mouse or
  hand-rolled escape parsing; not tempted).
- Virtualized rendering beyond the simple scroll-into-view window.
- Any animation beyond the mandated 300ms apply spinner and refresh flash.
- Fuzzy filter matching (spec says case-insensitive substring; stopped there).

## Semantics choices worth noting for the comparison

- `q` physically cannot quit while the filter is focused (keystrokes belong to
  the input), which satisfies "ignored while focused and non-empty" trivially;
  esc blurs and clears, then q works.
- Refresh reloads the fixture from disk, which also resets in-memory apply
  flips — that's what "refresh" means here; staged set is cleared with it.
- Bench (5 warm runs, `pnpm bench`): 291/287/288/287/286 ms — dominated by
  tsx/Node startup, not React render. Haven't tried the optional Bun run yet.
