import { execFileSync } from "node:child_process";
import { mkdirSync, rmSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

// Demo art for README + homepage.
// Content mirrors real fleet output shapes:
// - ls table: same columns/order as `fleet skill ls` (see www/src/content/docs/cli.md)
// - matrix: same glyphs/cells as internal/tui/view.go (● on, ○ off, - absent, ↑/✓/— badges)
// Rendered as SVG terminal windows -> PNG via sharp; GIF frames -> ffmpeg.
import sharp from "sharp";

const root = path.dirname(fileURLToPath(import.meta.url));
// render.mjs lives at www/scripts/demo/render.mjs -> public is ../../public.
const pub = path.resolve(root, "../../public");

const FONT = "GeistMono Nerd Font, Menlo, SF Mono, monospace";
const BG = "#0C1116";
const BAR = "#161D26";
const FG = "#E6EDF3";
const DIM = "#8B949E";
const GREEN = "#3FB950";
const YELLOW = "#D29922";
const CYAN = "#39C5CF";
const FAINT = "#6E7681";
const SEL = "#1C2530";

function esc(s) {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

// A line = array of [text, fill] spans. Selected rows get a bg rect.
function renderLines(lines, x, y, fs, lh, innerW) {
  let out = "";
  let cy = y;
  for (const spans of lines) {
    if (spans.__sel) {
      out += `<rect x="${x - 12}" y="${cy - fs - 5}" width="${innerW}" height="${lh}" rx="6" fill="${SEL}"/>`;
    }
    out += `<text x="${x}" y="${cy}" font-family="${FONT}" font-size="${fs}" xml:space="preserve">`;
    for (const [t, fill] of spans) {
      out += `<tspan fill="${fill}">${esc(t)}</tspan>`;
    }
    out += `</text>`;
    cy += lh;
  }
  return { svg: out, height: cy - y };
}

function pad(s, n) {
  const len = [...s].length;
  return len >= n ? s : s + " ".repeat(n - len);
}

function windowSvg({ title, bodySvg, bodyH, W = 1200 }) {
  const barH = 52;
  const padV = 28;
  const H = barH + bodyH + padV * 2;
  const innerW = W - 72;
  return {
    svg: `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}">
  <defs><clipPath id="body"><rect x="36" y="${barH}" width="${innerW}" height="${bodyH + padV * 2}" rx="8"/></clipPath></defs>
  <rect width="${W}" height="${H}" rx="16" fill="${BG}" stroke="#232D38" stroke-width="1.5"/>
  <rect width="${W}" height="${barH}" rx="16" fill="${BAR}"/>
  <rect y="${barH - 16}" width="${W}" height="16" fill="${BAR}"/>
  <circle cx="34" cy="26" r="7" fill="#FF5F57"/>
  <circle cx="58" cy="26" r="7" fill="#FEBC2E"/>
  <circle cx="82" cy="26" r="7" fill="#28C840"/>
  <text x="${W / 2}" y="32" text-anchor="middle" font-family="${FONT}" font-size="15" fill="${DIM}">${esc(title)}</text>
  <g clip-path="url(#body)"><g transform="translate(36, ${barH + padV})">${bodySvg}</g></g>
</svg>`,
    H,
    innerW,
  };
}

// ---- ls screenshot content (matches cli.md example shape) ----
function lsLines() {
  const D = DIM,
    G = GREEN,
    Y = YELLOW,
    C = CYAN,
    F = FG;
  return [
    [
      ["$ ", D],
      ["fleet skill ls", F],
    ],
    [["", F]],
    [
      ["fleet · ", F],
      ["2 skills · 1 installed · 1 custom · ", D],
      ["1 update", Y],
    ],
    [["", F]],
    [
      [
        pad("NAME", 10) +
          "  " +
          pad("OPENCODE", 8) +
          "  " +
          pad("PI", 3) +
          "  " +
          pad("CODEX", 5) +
          "  " +
          pad("CLAUDE", 6) +
          "  " +
          pad("CURSOR", 6) +
          "  " +
          pad("BOB", 3) +
          "  " +
          pad("UPDATE", 6) +
          "  " +
          pad("SOURCE", 17) +
          "  DESCRIPTION",
        D,
      ],
    ],
    [
      [pad("git-helper", 10) + "  ", F],
      [pad("on", 8) + "  ", G],
      [pad("on", 3) + "  ", G],
      [pad("on", 5) + "  ", G],
      [pad("-", 6) + "  ", D],
      [pad("on", 6) + "  ", G],
      [pad("on", 3) + "  ", G],
      [pad("—", 6) + "  ", D],
      [pad("custom", 17) + "  ", C],
      ["Commit-message helper.", FAINT],
    ],
    [
      [pad("tdd", 10) + "  ", F],
      [pad("on", 8) + "  ", G],
      [pad("off", 3) + "  ", D],
      [pad("on", 5) + "  ", G],
      [pad("-", 6) + "  ", D],
      [pad("on", 6) + "  ", G],
      [pad("on", 3) + "  ", G],
      [pad("↑", 6) + "  ", Y],
      [pad("mattpocock/skills", 17) + "  ", D],
      ["Strict TDD workflow.", FAINT],
    ],
    [["", F]],
    [
      ["$ ", D],
      ["fleet skill off tdd --harness pi", F],
    ],
    [
      ["skill: ", D],
      ["pi", C],
      [": disabled ", D],
      [`"tdd"`, F],
    ],
  ];
}

// ---- TUI matrix screenshot (mirrors internal/tui/view.go glyphs) ----
// Header labels follow harnessAbbrev: oc, cx, cl, cu abbreviated;
// pi and bob render full. Cursor/bob carry ! (no per-skill off switch).
function tuiLines() {
  const D = DIM,
    G = GREEN,
    Y = YELLOW,
    F = FG;
  const sel = (spans) => Object.assign(spans, { __sel: true });
  return [
    [
      ["fleet", F],
      [" · ", D],
      ["3 skills · 2 installed · 1 custom · 1 outdated", D],
    ],
    [
      ["/ ", D],
      ["filter", D],
    ],
    [["", F]],
    [[pad("", 32) + "oc   pi   cx   cl   cu!  bob!", D]],
    [["· custom", D]],
    [
      ["●  ", G],
      [pad("my-notes", 10) + " ", F],
      [pad("Personal note-taking…", 22) + " ", FAINT],
      ["—   ", D],
      ["●    ", G],
      ["●    ", G],
      ["●    ", G],
      ["-    ", D],
      ["●     ", G],
      ["●", G],
    ],
    [["· mattpocock/skills", D]],
    [
      ["●  ", G],
      [pad("git-helper", 10) + " ", F],
      [pad("Wraps git flows…", 22) + " ", FAINT],
      ["—   ", D],
      ["●    ", G],
      ["●    ", G],
      ["●    ", G],
      ["-    ", D],
      ["●     ", G],
      ["●", G],
    ],
    sel([
      ["○* ", Y],
      [pad("tdd", 10) + " ", F],
      [pad("Red-green-refactor…", 22) + " ", FAINT],
      ["↑   ", Y],
      ["●    ", G],
      ["○    ", D],
      ["●    ", G],
      ["-    ", D],
      ["●     ", G],
      ["●", G],
    ]),
    [["", F]],
    [["1 staged · enter applies", Y]],
    [
      [
        "space toggle   ←/→ harness   enter apply   u update   / filter   q quit",
        D,
      ],
    ],
  ];
}

async function toPng(svg, out, width) {
  await sharp(Buffer.from(svg)).resize({ width }).png().toFile(out);
  console.log("wrote", out);
}

async function main() {
  // ls + tui share one canvas size: equal pixel dimensions means equal
  // display height in the README's side-by-side table.
  const stillFS = 16,
    stillLH = 26,
    stillW = 1440;
  const stillInnerW = stillW - 72;
  const lsBody = renderLines(
    lsLines(),
    0,
    stillFS + 4,
    stillFS,
    stillLH,
    stillInnerW,
  );
  const tuiBody = renderLines(
    tuiLines(),
    0,
    stillFS + 4,
    stillFS,
    stillLH,
    stillInnerW,
  );
  const stillBodyH = Math.max(lsBody.height, tuiBody.height) + 12;
  {
    const { svg } = windowSvg({
      title: "fleet — fleet skill ls",
      bodySvg: lsBody.svg,
      bodyH: stillBodyH,
      W: stillW,
    });
    await toPng(svg, path.join(pub, "demo-ls.png"), 1600);
  }
  {
    const { svg } = windowSvg({
      title: "fleet — skill × harness matrix",
      bodySvg: tuiBody.svg,
      bodyH: stillBodyH,
      W: stillW,
    });
    await toPng(svg, path.join(pub, "demo-tui.png"), 1600);
  }
  // gif frames
  const frames = path.join(root, ".frames");
  rmSync(frames, { recursive: true, force: true });
  mkdirSync(frames, { recursive: true });
  const fullLs = lsLines();
  const fullTui = tuiLines();
  const scenes = [];
  const cmd = "fleet skill ls";
  for (let i = 1; i <= cmd.length; i += 2) {
    scenes.push({
      title: "fleet — demo",
      lines: [
        [
          ["$ ", DIM],
          [cmd.slice(0, i), FG],
          ["▊", GREEN],
        ],
      ],
    });
  }
  scenes.push({ title: "fleet — fleet skill ls", lines: fullLs.slice(0, 7) });
  scenes.push({ title: "fleet — fleet skill ls", lines: fullLs });
  scenes.push({ title: "fleet — skill × harness matrix", lines: fullTui });
  scenes.push({ title: "fleet — skill × harness matrix", lines: fullTui });
  scenes.push({ title: "fleet — skill × harness matrix", lines: fullTui });

  const FS = 15,
    LH = 24,
    W = 960;
  const innerW = W - 72;
  let n = 0;
  for (const s of scenes) {
    const { svg: body } = renderLines(s.lines, 0, FS + 4, FS, LH, innerW);
    const { svg } = windowSvg({ title: s.title, bodySvg: body, bodyH: 430, W });
    const fp = path.join(frames, `f-${String(n++).padStart(3, "0")}.png`);
    await sharp(Buffer.from(svg)).resize({ width: W }).png().toFile(fp);
  }
  const out = path.join(pub, "demo.gif");
  execFileSync(
    "ffmpeg",
    [
      "-y",
      "-framerate",
      "2",
      "-i",
      path.join(frames, "f-%03d.png"),
      "-vf",
      "split[s0][s1];[s0]palettegen=max_colors=256[p];[s1][p]paletteuse=dither=bayer:bayer_scale=5",
      out,
    ],
    { stdio: "inherit" },
  );
  console.log("wrote", out);
  rmSync(frames, { recursive: true, force: true });
}

await main();
