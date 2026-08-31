import { pathToFileURL } from "node:url";
import { render, Box, Text, useApp, useInput, useStdout } from "ink";
import TextInput from "ink-text-input";
import Spinner from "ink-spinner";
import { useEffect, useMemo, useState } from "react";
import { loadFixture, expandRows, useSkillctl, type SkillRow } from "./store.ts";

const PRIMARY = 1 + 1 + 1 + 2 + 2; // glyph + star + space + two 2-space gaps

function trunc(s: string, width: number): string {
  if (s.length <= width) return s;
  return width <= 1 ? s.slice(0, width) : s.slice(0, width - 1) + "…";
}

function padEnd(s: string, width: number): string {
  return s.length >= width ? s : s + " ".repeat(width - s.length);
}

function Row({
  row,
  selected,
  staged,
  nameW,
  descW,
  rootsW,
}: {
  row: SkillRow;
  selected: boolean;
  staged: boolean;
  nameW: number;
  descW: number;
  rootsW: number;
}) {
  const glyph = row.enabled ? (
    <Text color="green">●</Text>
  ) : (
    <Text dimColor>○</Text>
  );
  const star = staged ? <Text color="yellow">*</Text> : <Text> </Text>;
  const name = padEnd(trunc(row.name, nameW), nameW);
  const desc = padEnd(trunc(row.description, descW), descW);
  const roots = trunc(row.roots.join(", "), rootsW);
  return (
    <Text inverse={selected}>
      {glyph}
      {star} {name}  {desc}  {roots}
    </Text>
  );
}

function HelpOverlay() {
  const keys: Array<[string, string]> = [
    ["j / k, arrows", "move selection"],
    ["/", "focus filter input"],
    ["esc", "clear filter / cancel apply"],
    ["space", "stage or unstage selected skill"],
    ["enter", "apply staged changes (300ms)"],
    ["r", "refresh fixture from disk"],
    ["?", "toggle this help overlay"],
    ["q", "quit (not while filter is focused)"],
  ];
  return (
    <Box flexDirection="column" borderStyle="round" borderColor="cyan" paddingX={1}>
      <Text bold>keybindings</Text>
      {keys.map(([key, action]) => (
        <Text key={key}>
          <Text color="cyan">{padEnd(key, 16)}</Text>
          {action}
        </Text>
      ))}
      <Text dimColor>press ? or esc to close</Text>
    </Box>
  );
}

function App({
  rows,
  bench,
  onReady,
}: {
  rows: SkillRow[];
  bench: boolean;
  onReady?: () => void;
}) {
  const ctl = useSkillctl(rows, rows.length);
  const { exit } = useApp();
  const { stdout } = useStdout();
  const columns = stdout.columns ?? 80;
  const bodyHeight = Math.max(3, (stdout.rows ?? 24) - 5);
  const [helpOpen, setHelpOpen] = useState(false);

  useEffect(() => {
    if (!bench) return;
    const ms = Math.round(performance.now());
    process.stderr.write(`startup→first frame: ${ms} ms\n`);
    onReady?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useInput((input, key) => {
    // While the filter input is focused, keystrokes belong to the input.
    // (q therefore cannot quit until the filter is blurred via esc.)
    if (ctl.filterFocused) {
      if (key.escape) {
        ctl.setFilter("");
        ctl.setFilterFocused(false);
      }
      return;
    }
    if (key.escape) {
      if (helpOpen) setHelpOpen(false);
      else if (ctl.phase !== "idle") ctl.cancelApply();
      return;
    }
    if (helpOpen) {
      setHelpOpen(false);
      return;
    }
    if (input === "j" || key.downArrow) ctl.move(1);
    else if (input === "k" || key.upArrow) ctl.move(-1);
    else if (input === "/") ctl.setFilterFocused(true);
    else if (input === " ") ctl.stage(ctl.filtered[ctl.cursor]?.name);
    else if (key.return) ctl.apply();
    else if (input === "r") ctl.refresh();
    else if (input === "?") setHelpOpen(true);
    else if (input === "q") exit();
  });

  const { filtered, cursor } = ctl;

  // Scroll-into-view window over the filtered rows.
  const view = useMemo(() => {
    const start =
      cursor >= 0 && cursor < filtered.length
        ? Math.min(
            Math.max(cursor - Math.floor(bodyHeight / 2), 0),
            Math.max(filtered.length - bodyHeight, 0),
          )
        : 0;
    return { start, slice: filtered.slice(start, start + bodyHeight) };
  }, [filtered, cursor, bodyHeight]);

  const nameW = Math.min(
    34,
    Math.max(6, ...filtered.map((r) => r.name.length)),
  );
  const rootsW = Math.min(
    24,
    Math.max(9, ...filtered.map((r) => r.roots.join(", ").length)),
  );
  const descW = Math.max(12, columns - PRIMARY - nameW - rootsW);

  const enabled = ctl.rows.filter((r) => r.enabled).length;
  const disabled = ctl.rows.length - enabled;
  const stagedCount = ctl.staged.size;

  const statusLine = (
    <Text>
      skillctl · {ctl.rows.length} skills · <Text color="green">{enabled} enabled</Text> ·{" "}
      <Text dimColor>{disabled} disabled</Text>
      {stagedCount > 0 && <Text color="yellow"> · {stagedCount} staged</Text>}
    </Text>
  );

  const filterLine = ctl.filterFocused ? (
    <Text>
      / <TextInput value={ctl.filter} onChange={ctl.setFilter} placeholder="type to filter" showCursor />
    </Text>
  ) : (
    <Text dimColor>
      / {ctl.filter || "press / to filter"}
    </Text>
  );

  const busyLine =
    ctl.phase === "applying" ? (
      <Text color="cyan">
        <Spinner type="dots" /> {ctl.preview}
      </Text>
    ) : ctl.refreshing ? (
      <Text color="cyan">
        <Spinner type="dots" /> refreshing…
      </Text>
    ) : null;

  const messageLine =
    busyLine ?? (ctl.message ? <Text color="cyan">{ctl.message}</Text> : null);

  return (
    <Box flexDirection="column">
      {statusLine}
      {filterLine}
      {helpOpen ? (
        <HelpOverlay />
      ) : (
        <Box flexDirection="column">
          {view.slice.map((row, i) => (
            <Row
              key={row.name}
              row={row}
              selected={view.start + i === cursor}
              staged={ctl.staged.has(row.name)}
              nameW={nameW}
              descW={descW}
              rootsW={rootsW}
            />
          ))}
        </Box>
      )}
      {messageLine}
      <Text dimColor>
        {" "}
        space toggle   enter apply staged   / filter   r refresh   ? help   q quit
      </Text>
    </Box>
  );
}

export function runTui(opts: { rows?: number; bench?: boolean } = {}) {
  const fixture = loadFixture();
  const expanded = expandRows(fixture.skills, opts.rows ?? fixture.skills.length);
  let app!: ReturnType<typeof render>;
  app = render(
    <App
      rows={expanded}
      bench={opts.bench ?? false}
      onReady={opts.bench ? () => app.unmount() : undefined}
    />,
  );
  return app;
}

function isMain(): boolean {
  const entry = process.argv[1];
  return entry ? import.meta.url === pathToFileURL(entry).href : false;
}

if (isMain()) {
  const args = process.argv.slice(2);
  const bench = args.includes("--bench");
  const rowsFlag = args.indexOf("--rows");
  const rows =
    rowsFlag >= 0 && args[rowsFlag + 1] ? Number(args[rowsFlag + 1]) : undefined;
  runTui({ rows, bench });
}
