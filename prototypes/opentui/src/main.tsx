// main.tsx — skillctl TUI on OpenTUI + React (Bun only).
// Exported runTui() is called by `skillctl tui` in cli.ts; the module also
// runs standalone via `bun run src/main.tsx [--rows N] [--bench]`.
import { createElement, useEffect, useRef, useState } from "react";
import { createCliRenderer, type CliRenderer } from "@opentui/core";
import { createRoot, useKeyboard, useRenderer, useTerminalDimensions } from "@opentui/react";
import {
  expandRows,
  formatRowParts,
  loadFixture,
  spinnerFrame,
  useSkillctl,
  type Skill,
} from "./store";

// Refresh reloads from disk; expandTo keeps the --rows expansion across refreshes.

export interface TuiOptions {
  rows: number;
  bench: boolean;
}

const startupStart = performance.now();

const COLORS = {
  enabled: "#4ade80",
  disabled: "#666666",
  staged: "#eab308",
  accent: "#8b8bff",
  dim: "#888888",
  selectionBg: "#3a3a4a",
  selectionFg: "#ffffff",
  overlayBg: "#20202a",
};

function StatusBar({ counts }: { counts: SkillctlCounts }) {
  const staged = ` · ${counts.stagedCount} staged`;
  return (
    <text fg={COLORS.accent}>
      skillctl · {counts.total} skills · {counts.enabled} enabled · {counts.disabled} disabled
      {counts.stagedCount > 0 ? <span fg={COLORS.staged}>{staged}</span> : null}
    </text>
  );
}

interface SkillctlCounts {
  total: number;
  enabled: number;
  disabled: number;
  stagedCount: number;
}

function Spinner() {
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 80);
    return () => clearInterval(id);
  }, []);
  // Must be a text node: OpenTUI throws "span must be created inside of a text
  // node" when a span is mounted directly under a box.
  return <text fg={COLORS.staged}>{spinnerFrame(tick)} </text>;
}

export function App({ initialRows, bench }: { initialRows: Skill[]; bench: boolean }) {
  const renderer = useRenderer();
  const { width, height } = useTerminalDimensions();
  const ctl = useSkillctl(initialRows);
  const [filterFocused, setFilterFocused] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  const [scrollTop, setScrollTop] = useState(0);
  const quitRef = useRef(false);

  // --bench: measure startup → first frame, print to stderr, exit cleanly.
  useEffect(() => {
    if (!bench) return;
    const cb = () => {
      const ms = (performance.now() - startupStart).toFixed(1);
      process.stderr.write(`startup→first frame: ${ms}ms\n`);
      renderer.destroy();
      process.exit(0);
    };
    renderer.addPostProcessFn(cb);
    return () => renderer.removePostProcessFn(cb);
  }, [bench, renderer]);

  const chrome = 4; // status bar + filter row + footer + one spare line
  const viewHeight = Math.max(1, height - chrome);

  // Keep the windowed slice around the cursor.
  useEffect(() => {
    if (ctl.cursor < scrollTop) setScrollTop(ctl.cursor);
    else if (ctl.cursor >= scrollTop + viewHeight) setScrollTop(ctl.cursor - viewHeight + 1);
  }, [ctl.cursor, viewHeight, scrollTop]);

  useKeyboard((key) => {
    if (bench || quitRef.current) return;
    if (filterFocused) {
      if (key.name === "escape") {
        ctl.setFilter("");
        setFilterFocused(false);
      } else if (key.name === "backspace" || key.sequence === "\b" || key.sequence === "\x7f") {
        ctl.setFilter((f) => f.slice(0, -1));
      } else if (key.name === "return" || key.sequence === "\r") {
        setFilterFocused(false);
      } else if (key.sequence.length === 1 && key.sequence >= " " && !key.ctrl && !key.meta) {
        ctl.setFilter((f) => f + key.sequence);
      }
      return; // all other bindings suspended while the filter is focused
    }
    if (helpOpen) {
      if (key.name === "escape" || key.sequence === "?" || key.name === "q") setHelpOpen(false);
      return;
    }
    switch (key.sequence) {
      case "j":
      case "down":
        ctl.move(1);
        break;
      case "k":
      case "up":
        ctl.move(-1);
        break;
      case " ":
      case "space":
        if (ctl.filtered[ctl.cursor]) ctl.stage(ctl.filtered[ctl.cursor].name);
        break;
      case "\r":
      case "return":
      case "enter":
        ctl.apply();
        break;
      case "r":
        ctl.refresh();
        break;
      case "?":
        setHelpOpen(true);
        break;
      case "/":
        setFilterFocused(true);
        break;
      case "q":
        quitRef.current = true;
        renderer.destroy();
        process.exit(0);
        break;
      case "\x1b":
      case "escape":
        if (ctl.phase !== "idle") ctl.cancelApply();
        else ctl.clearTransient();
        break;
    }
  });

  const nw = Math.min(30, Math.max(10, ...ctl.filtered.map((r) => r.name.length)));
  const descWidth = Math.max(20, width - nw - 30);

  return (
    <box flexDirection="column" width="100%" height="100%" backgroundColor="#101014">
      <StatusBar counts={ctl.counts} />
      <box flexDirection="row" width="100%" height={1}>
        <text fg={filterFocused ? COLORS.staged : COLORS.dim}>/ </text>
        {ctl.filter ? (
          <text fg="#ffffff">{ctl.filter}</text>
        ) : (
          <text fg={COLORS.disabled}>filter skills</text>
        )}
      </box>
      <box flexDirection="column" flexGrow={1} overflow="hidden">
        {ctl.filtered.slice(scrollTop, scrollTop + viewHeight).map((row) => {
          const selected = row.name === ctl.filtered[ctl.cursor]?.name;
          const staged = ctl.staged.has(row.name);
          const p = formatRowParts(row, staged, nw, descWidth);
          return (
            <box
              key={row.name}
              flexDirection="row"
              width="100%"
              height={1}
              backgroundColor={selected ? COLORS.selectionBg : undefined}
            >
              <text fg={selected ? COLORS.selectionFg : row.enabled ? COLORS.enabled : COLORS.disabled}>
                {p.glyph}
                {staged ? <span fg={COLORS.staged}>*</span> : " "}
                {" "}
              </text>
              <text fg={selected ? COLORS.selectionFg : "#d0d0d0"}>{p.name} </text>
              <text fg={selected ? COLORS.selectionFg : COLORS.dim} flexGrow={1}>
                {p.description}
              </text>
              <text fg={selected ? COLORS.selectionFg : COLORS.disabled}>{p.roots}</text>
            </box>
          );
        })}
        {ctl.filtered.length === 0 ? <text fg={COLORS.disabled}>no skills match</text> : null}
      </box>
      <box flexDirection="row" width="100%" height={1}>
        {ctl.phase === "applying" ? (
          <>
            <Spinner />
            <text fg={COLORS.staged}>{ctl.preview ?? "applying…"}</text>
          </>
        ) : ctl.phase === "refreshing" ? (
          <>
            <Spinner />
            <text fg={COLORS.staged}>refreshing…</text>
          </>
        ) : ctl.message ? (
          <text fg={COLORS.enabled}>{ctl.message}</text>
        ) : null}
      </box>
      <text fg={COLORS.disabled}>
        space toggle   enter apply staged   / filter   r refresh   ? help   q quit
      </text>
      {helpOpen ? (
        <box
          position="absolute"
          top="15%"
          left="15%"
          width="70%"
          height="60%"
          border
          borderStyle="single"
          borderColor={COLORS.accent}
          backgroundColor={COLORS.overlayBg}
          title=" keybindings "
          titleColor={COLORS.accent}
          zIndex={10}
        >
          <box flexDirection="column" paddingLeft={2} paddingTop={1}>
            <text fg="#d0d0d0">j / k, arrows   move selection</text>
            <text fg="#d0d0d0">space           stage / unstage toggle for selected row</text>
            <text fg="#d0d0d0">enter           apply staged changes (preview, spinner, flip)</text>
            <text fg="#d0d0d0">/               focus filter (esc clears and unfocuses)</text>
            <text fg="#d0d0d0">r               refresh (reload fixture)</text>
            <text fg="#d0d0d0">?               toggle this help overlay</text>
            <text fg="#d0d0d0">esc             clear filter / cancel apply</text>
            <text fg={COLORS.disabled}>q               quit (not while the filter is focused)</text>
          </box>
        </box>
      ) : null}
    </box>
  );
}

export async function runTui(options: TuiOptions): Promise<void> {
  const initialRows = expandRows(loadFixture().skills, options.rows);
  const renderer = await createCliRenderer({ exitOnCtrlC: true });
  createRoot(renderer).render(createElement(App, { initialRows, bench: options.bench }));
}

// Standalone entry: bun run src/main.tsx [--rows N] [--bench]
if (import.meta.main) {
  const argv = process.argv.slice(2);
  let rows = 45;
  let bench = false;
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--rows") rows = Number.parseInt(argv[++i] ?? "", 10) || 45;
    else if (argv[i]?.startsWith("--rows=")) rows = Number.parseInt(argv[i].slice(7), 10) || 45;
    else if (argv[i] === "--bench") bench = true;
  }
  await runTui({ rows, bench });
}
