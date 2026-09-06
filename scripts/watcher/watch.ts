#!/usr/bin/env node
/**
 * Fleet Watcher — snapshot & diff every file fleet touches.
 *
 * What it does
 * ------------
 * Takes a content-hashed snapshot of all fleet-managed locations (fleet state,
 * canonical skill store, and each harness's config + skills dir) and diffs it
 * against the previous snapshot. Modified files get a unified line diff via a
 * deduped content store; identical-content rewrites are reported as "touched"
 * rather than "modified"; empty-dir creation/removal is reported explicitly;
 * minified blobs are summarized by hash.
 *
 * Usage
 * -----
 *   node scripts/watcher/watch.ts --initial --label=baseline   # record baseline
 *   node scripts/watcher/watch.ts --label=go-1                 # snapshot + diff
 *   # chat shorthand "go" runs the second form
 *
 * State lives in scripts/watcher/.state/ (gitignored) — snapshots as
 * snap-*.json and file contents deduped by SHA-1 under contents/.
 *
 * Node stdlib only, no dependencies. Run directly — Node 22+ strips types
 * with no build step and no install.
 */

import { createHash } from "node:crypto";
import fs from "node:fs";
import { homedir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const HOME = homedir();
const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
// Fleet repo root is two levels above scripts/watcher/ (scripts/watcher -> repo
// root). Go code lives at the repo root (internal/paths) and the watcher stays
// at scripts/watcher/watch.ts, so depth is unchanged — two levels still
// resolves to the repo root's skills/ publishable collection.
const REPO = path.resolve(SCRIPT_DIR, "..", "..");
// State lives inside the repo (gitignored) so the watcher is self-contained.
const STATE_DIR = path.join(SCRIPT_DIR, ".state");
const CONTENT_DIR = path.join(STATE_DIR, "contents");

// (label, absolute path) — paths derived from fleet's internal/paths/paths.go
const WATCH_TARGETS: Array<[string, string]> = [
  ["fleet-config", path.join(HOME, ".config/fleet")],
  ["agents-store", path.join(HOME, ".agents")],
  ["opencode-config", path.join(HOME, ".config/opencode/opencode.jsonc")],
  ["opencode-skills", path.join(HOME, ".config/opencode/skills")],
  ["pi-settings", path.join(HOME, ".pi/agent/settings.json")],
  ["pi-skills", path.join(HOME, ".pi/agent/skills")],
  ["codex-config", path.join(HOME, ".codex/config.toml")],
  ["codex-skills", path.join(HOME, ".codex/skills")],
  ["claude-skills", path.join(HOME, ".claude/skills")],
  ["claude-config", path.join(HOME, ".claude/settings.json")],
  ["cursor-skills", path.join(HOME, ".cursor/skills")],
  ["bob-skills", path.join(HOME, ".bob/skills")],
  ["bob-settings", path.join(HOME, ".bob/settings.json")],
  ["repo-skills", path.join(REPO, "skills")],
];

const MAX_HASH = 4 * 1024 * 1024; // skip hashing files > 4MB (record size+mtime only)
const MAX_DIFF_LINES = 120;
const MINIFIED_LEN = 500;
// mtime_ns is stored as a JSON number (doubles go granular past 2^53, so the
// Python-written exact nanoseconds round-trip within ~256ns). Comparisons
// below tolerate sub-microsecond drift so a snapshot taken by watch.py and
// the next taken by watch.ts don't flag every file as touched.
const MTIME_TOLERANCE_NS = 1000;

type Entry =
  | { type: "symlink"; target: string }
  | { type: "file"; size: number; mtime_ns: number; sha1?: string }
  | { type: "unreadable" };

type TargetSnap = {
  root: string;
  exists: boolean;
  entries: Record<string, Entry>;
};
type Snapshot = {
  taken_at: string;
  label: string;
  targets: Record<string, TargetSnap>;
};

function fileEntry(full: string): Entry {
  const st = fs.lstatSync(full, { bigint: true });
  const e: Entry = {
    type: "file",
    size: Number(st.size),
    mtime_ns: Number(st.mtimeNs),
  };
  if (Number(st.size) <= MAX_HASH) {
    const h = createHash("sha1");
    h.update(fs.readFileSync(full));
    (e as { sha1?: string }).sha1 = h.digest("hex");
  }
  return e;
}

function scan(target: string): Record<string, Entry> {
  const entries: Record<string, Entry> = {};
  let st: fs.Stats | fs.BigIntStats;
  try {
    st = fs.lstatSync(target);
  } catch {
    return entries; // missing — recorded implicitly by absence
  }
  if (st.isSymbolicLink()) {
    entries[target] = { type: "symlink", target: fs.readlinkSync(target) };
    return entries;
  }
  if (st.isFile()) {
    try {
      entries[target] = fileEntry(target);
    } catch {
      entries[target] = { type: "unreadable" };
    }
    return entries;
  }
  if (!st.isDirectory()) {
    return entries;
  }
  const walk = (dir: string): void => {
    let dirents: fs.Dirent[];
    try {
      dirents = fs.readdirSync(dir, { withFileTypes: true });
    } catch {
      return; // unreadable dir — os.walk skips these silently too
    }
    // Parent-level files first (mirrors os.walk top-down yield), then recurse,
    // so snapshot key order stays comparable with watch.py snapshots.
    const subdirs: string[] = [];
    for (const d of dirents) {
      const full = path.join(dir, d.name);
      if (d.isSymbolicLink()) {
        // Record all symlinks, including links-to-dir: fleet links skills
        // as dir symlinks (e.g. ~/.bob/skills/<name> -> repo skills/<name>).
        // Never descend into dir links — their contents belong to the target.
        entries[full] = { type: "symlink", target: fs.readlinkSync(full) };
        continue;
      }
      if (d.isDirectory()) {
        subdirs.push(full);
        continue;
      }
      try {
        entries[full] = fileEntry(full);
      } catch {
        entries[full] = { type: "unreadable" };
      }
    }
    for (const sub of subdirs) walk(sub);
  };
  walk(target);
  return entries;
}

function takeSnapshot(label = ""): Snapshot {
  const snap: Snapshot = { taken_at: nowStamp(), label, targets: {} };
  for (const [name, root] of WATCH_TARGETS) {
    snap.targets[name] = {
      root,
      exists: fs.existsSync(root),
      entries: scan(root),
    };
  }
  saveContents(snap);
  return snap;
}

function fileStamp(): string {
  // snap-%Y%m%d-%H%M%S, byte-identical to watch.py snapshot filenames.
  const d = new Date();
  const p = (n: number, w = 2): string => String(n).padStart(w, "0");
  return (
    `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}` +
    `-${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}`
  );
}

function nowStamp(): string {
  const d = new Date();
  const p = (n: number, w = 2): string => String(n).padStart(w, "0");
  return (
    `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}` +
    `T${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
  );
}

function saveContents(snap: Snapshot): void {
  // Store file contents under CONTENT_DIR/<sha1> (deduped) so later
  // snapshots can show line-level diffs even after files change or vanish.
  fs.mkdirSync(CONTENT_DIR, { recursive: true });
  for (const t of Object.values(snap.targets)) {
    for (const [p, e] of Object.entries(t.entries)) {
      const h = e.type === "file" ? e.sha1 : undefined;
      if (!h) continue;
      const dest = path.join(CONTENT_DIR, h);
      if (fs.existsSync(dest)) continue;
      try {
        fs.writeFileSync(dest, fs.readFileSync(p));
      } catch {
        // vanished or unreadable between scan and store — skip
      }
    }
  }
}

function readStored(sha1: string): string[] | null {
  try {
    const text = new TextDecoder("utf-8").decode(
      fs.readFileSync(path.join(CONTENT_DIR, sha1)),
    );
    if (text === "") return [];
    const lines = text.split(/\r\n|[\r\n]/);
    // str.splitlines() yields no trailing "" for a final line break.
    if (/(\r\n|[\r\n])$/.test(text) && lines[lines.length - 1] === "") {
      lines.pop();
    }
    return lines;
  } catch {
    return null;
  }
}

type Op = {
  tag: "equal" | "replace" | "delete" | "insert";
  a0: number;
  a1: number;
  b0: number;
  b1: number;
};

// Myers O(ND) diff over lines. Config/skill files are small; bail out (hash
// line still prints) rather than burning memory on huge inputs.
function opcodes(a: string[], b: string[]): Op[] | null {
  const n = a.length;
  const m = b.length;
  if (n + m > 20000) return null;
  const max = n + m;
  const v = new Map<number, number>([[1, 0]]);
  const trace: Map<number, number>[] = [];
  let found = false;
  for (let d = 0; d <= max; d++) {
    for (let k = -d; k <= d; k += 2) {
      let x: number;
      if (k === -d || (k !== d && (v.get(k - 1) ?? 0) < (v.get(k + 1) ?? 0))) {
        x = v.get(k + 1) ?? 0; // down (insertion)
      } else {
        x = (v.get(k - 1) ?? 0) + 1; // right (deletion)
      }
      let y = x - k;
      while (x < n && y < m && a[x] === b[y]) {
        x++;
        y++;
      }
      v.set(k, x);
      if (x >= n && y >= m) {
        trace.push(new Map(v));
        found = true;
        break;
      }
    }
    if (found) break;
    trace.push(new Map(v));
  }
  // Backtrack to an edit script.
  const script: Array<{ op: "eq" | "del" | "ins"; a: number; b: number }> = [];
  let x = n;
  let y = m;
  for (let d = trace.length - 1; d > 0; d--) {
    const prev = trace[d - 1];
    const k = x - y;
    const down =
      k === -d || (k !== d && (prev.get(k - 1) ?? 0) < (prev.get(k + 1) ?? 0));
    const kPrev = down ? k + 1 : k - 1;
    const xPrev = prev.get(kPrev) ?? 0;
    const yPrev = xPrev - kPrev;
    while (x > xPrev && y > yPrev) {
      script.push({ op: "eq", a: x - 1, b: y - 1 });
      x--;
      y--;
    }
    if (down) {
      script.push({ op: "ins", a: x, b: y - 1 });
      y--;
    } else {
      script.push({ op: "del", a: x - 1, b: y });
      x--;
    }
  }
  while (x > 0 && y > 0) {
    script.push({ op: "eq", a: x - 1, b: y - 1 });
    x--;
    y--;
  }
  script.reverse();
  // Merge into difflib-style opcodes.
  const ops: Op[] = [];
  let i = 0;
  while (i < script.length) {
    const s = script[i];
    if (s.op === "eq") {
      let j = i;
      while (j < script.length && script[j].op === "eq") j++;
      ops.push({
        tag: "equal",
        a0: s.a,
        a1: script[j - 1].a + 1,
        b0: s.b,
        b1: script[j - 1].b + 1,
      });
      i = j;
    } else {
      let j = i;
      let del = 0;
      let ins = 0;
      while (j < script.length && script[j].op !== "eq") {
        if (script[j].op === "del") del++;
        else ins++;
        j++;
      }
      const tag =
        del > 0 && ins > 0 ? "replace" : del > 0 ? "delete" : "insert";
      ops.push({
        tag,
        a0: script[i].a,
        a1: script[i].a + del,
        b0: script[i].b,
        b1: script[i].b + ins,
      });
      i = j;
    }
  }
  return ops;
}

function formatRange(start: number, stop: number): string {
  const beginning = start + 1;
  const length = stop - start;
  if (length === 1) return `${beginning}`;
  if (length === 0) return `${beginning - 1},0`;
  return `${beginning},${length}`;
}

// difflib.get_grouped_opcodes(n=3), ported verbatim.
function groupedOpcodes(ops: Op[], n = 3): Op[][] {
  const codes = ops.map((o) => ({ ...o }));
  if (codes.length > 0 && codes[0].tag === "equal") {
    const c = codes[0];
    c.a0 = Math.max(c.a0, c.a1 - n);
    c.b0 = Math.max(c.b0, c.b1 - n);
  }
  if (codes.length > 0 && codes[codes.length - 1].tag === "equal") {
    const c = codes[codes.length - 1];
    c.a1 = Math.min(c.a1, c.a0 + n);
    c.b1 = Math.min(c.b1, c.b0 + n);
  }
  const nn = n + n;
  const groups: Op[][] = [];
  let group: Op[] = [];
  for (const c of codes) {
    if (c.tag === "equal" && c.a1 - c.a0 > nn) {
      group.push({
        tag: c.tag,
        a0: c.a0,
        a1: Math.min(c.a1, c.a0 + n),
        b0: c.b0,
        b1: Math.min(c.b1, c.b0 + n),
      });
      groups.push(group);
      group = [
        {
          tag: c.tag,
          a0: Math.max(c.a0, c.a1 - n),
          a1: c.a1,
          b0: Math.max(c.b0, c.b1 - n),
          b1: c.b1,
        },
      ];
    } else {
      group.push(c);
    }
  }
  if (group.length > 0 && !(group.length === 1 && group[0].tag === "equal")) {
    groups.push(group);
  }
  return groups;
}

function unifiedDiff(
  a: string[],
  b: string[],
  from: string,
  to: string,
): string[] {
  const ops = opcodes(a, b);
  if (ops === null) return [];
  const out = [`--- ${from}`, `+++ ${to}`];
  for (const group of groupedOpcodes(ops)) {
    const first = group[0];
    const last = group[group.length - 1];
    out.push(
      `@@ -${formatRange(first.a0, last.a1)} +${formatRange(first.b0, last.b1)} @@`,
    );
    for (const c of group) {
      if (c.tag === "equal") {
        for (let i = c.a0; i < c.a1; i++) out.push(` ${a[i]}`);
      } else {
        if (c.tag === "replace" || c.tag === "delete") {
          for (let i = c.a0; i < c.a1; i++) out.push(`-${a[i]}`);
        }
        if (c.tag === "replace" || c.tag === "insert") {
          for (let i = c.b0; i < c.b1; i++) out.push(`+${b[i]}`);
        }
      }
    }
  }
  return out;
}

function contentDiff(p: string, oldE: Entry, newE: Entry): string | null {
  // Unified line diff for a modified file, from stored contents.
  const ha = oldE.type === "file" ? oldE.sha1 : undefined;
  const hb = newE.type === "file" ? newE.sha1 : undefined;
  if (!ha || !hb) return null;
  const oldLines = readStored(ha);
  const newLines = readStored(hb);
  if (oldLines === null || newLines === null) return null;
  // Skip minified/one-line blobs (e.g. 900KB JSON caches): a "line diff"
  // there is useless — the hash comparison above already flags the change.
  // (Empty files have no lines to measure — never minified.)
  const longest = (ls: string[]): number =>
    ls.length === 0 ? 0 : Math.max(...ls.map((l) => l.length));
  if (longest(oldLines) > MINIFIED_LEN || longest(newLines) > MINIFIED_LEN) {
    return "  (minified content — see hash change above)";
  }
  const d = unifiedDiff(oldLines, newLines, `${p} (old)`, `${p} (new)`);
  if (d.length === 0) return null;
  // difflib with identical inputs yields only headers; same here (two header
  // lines, no hunks) — also "no diff".
  if (d.length <= 2) return null;
  const shown =
    d.length > MAX_DIFF_LINES
      ? [
          ...d.slice(0, MAX_DIFF_LINES),
          `  ... (${d.length - MAX_DIFF_LINES} more diff lines)`,
        ]
      : d;
  return shown.map((l) => `  ${l}`).join("\n");
}

function shaOf(e: Entry): string | undefined {
  return e.type === "file" ? e.sha1 : undefined;
}

function entriesEqual(a: Entry, b: Entry): boolean {
  if (a.type !== b.type) return false;
  if (a.type === "symlink" && b.type === "symlink")
    return a.target === b.target;
  if (a.type === "file" && b.type === "file") {
    return (
      a.size === b.size &&
      shaOf(a) === shaOf(b) &&
      Math.abs(a.mtime_ns - b.mtime_ns) <= MTIME_TOLERANCE_NS
    );
  }
  return true; // both unreadable
}

function diffSnapshots(old: Snapshot, next: Snapshot): string {
  const lines: string[] = [];
  for (const [name, newt] of Object.entries(next.targets)) {
    const oldt = old.targets[name] ?? { exists: false, entries: {} };
    const oe = oldt.entries ?? {};
    const ne = newt.entries;
    const added = Object.keys(ne)
      .filter((p) => !(p in oe))
      .sort();
    const removed = Object.keys(oe)
      .filter((p) => !(p in ne))
      .sort();
    const common = Object.keys(ne).filter((p) => p in oe);
    // Matches watch.py exactly: symlink target changes compare sha-less
    // (undefined !== undefined is false), so they surface nowhere — kept for
    // byte-identical reports; see contentDiff callers, not here.
    const modified = common
      .filter(
        (p) => !entriesEqual(oe[p], ne[p]) && shaOf(oe[p]) !== shaOf(ne[p]),
      )
      .sort();
    const touched = common
      .filter(
        (p) =>
          !entriesEqual(oe[p], ne[p]) &&
          oe[p].type === "file" &&
          shaOf(oe[p]) !== undefined &&
          shaOf(oe[p]) === shaOf(ne[p]),
      )
      .sort();
    if (
      added.length === 0 &&
      removed.length === 0 &&
      modified.length === 0 &&
      touched.length === 0 &&
      oldt.exists === newt.exists
    ) {
      continue;
    }
    lines.push(`\n## ${name}  (${newt.root})`);
    if (!oldt.exists && newt.exists) {
      lines.push(`  + <dir created>`);
    } else if (oldt.exists && !newt.exists) {
      lines.push(`  - <dir removed>`);
    }
    for (const p of added) {
      const e = ne[p];
      const detail =
        e.type === "symlink"
          ? e.target
          : `${e.type === "file" ? e.size : "?"}B`;
      lines.push(`  + ${p}  [${e.type} ${detail}]`);
    }
    for (const p of removed) {
      const e = oe[p];
      const detail =
        e.type === "symlink"
          ? e.target
          : `${e.type === "file" ? e.size : "?"}B`;
      lines.push(`  - ${p}  [was ${e.type} ${detail}]`);
    }
    for (const p of touched) {
      const e = ne[p];
      lines.push(
        `  · ${p}  touched, content unchanged (${e.type === "file" ? e.size : "?"}B)`,
      );
    }
    for (const p of modified) {
      const a = oe[p];
      const b = ne[p];
      if (a.type === "symlink") {
        const at = a.target;
        const bt = b.type === "symlink" ? b.target : "?";
        lines.push(`  ~ ${p}  link: ${at} -> ${bt}`);
      } else {
        const ha = (shaOf(a) ?? "?").slice(0, 8);
        const hb = (shaOf(b) ?? "?").slice(0, 8);
        const asize = a.type === "file" ? a.size : "?";
        const bsize = b.type === "file" ? b.size : "?";
        lines.push(`  ~ ${p}  (${asize}B/${ha} -> ${bsize}B/${hb})`);
        const cd = contentDiff(p, a, b);
        if (cd) lines.push(cd);
      }
    }
  }
  return lines.length > 0 ? lines.join("\n") : "\nNo changes detected.";
}

function homeify(p: string): string {
  // String-pattern replace hits the first occurrence only — same as
  // Python's str.replace(old, new, 1).
  return p.startsWith(HOME + "/") ? p.replace(HOME + "/", "~/") : p;
}

function report(
  newSnap: Snapshot,
  oldSnap: Snapshot | null,
  oldName: string | null,
  isInitial: boolean,
): void {
  console.log(`Snapshot: ${newSnap.taken_at}  (${newSnap.label || "unnamed"})`);
  const total = Object.values(newSnap.targets).reduce(
    (acc, t) => acc + Object.keys(t.entries).length,
    0,
  );
  console.log(
    `Tracked entries: ${total} across ${WATCH_TARGETS.length} targets`,
  );
  for (const [name, t] of Object.entries(newSnap.targets)) {
    const status = t.exists ? "ok" : "MISSING";
    console.log(
      `  ${status.padEnd(8)} ${name.padEnd(14)} ${homeify(t.root)}  (${Object.keys(t.entries).length} entries)`,
    );
  }
  if (isInitial) {
    console.log("\nBaseline recorded. Say 'go' to diff against this snapshot.");
  } else {
    console.log(`\nDiffs vs previous snapshot (${oldName}):`);
    console.log(diffSnapshots(oldSnap as Snapshot, newSnap));
  }
}

function main(): void {
  fs.mkdirSync(STATE_DIR, { recursive: true });
  const argv = process.argv.slice(2);
  const isInitial = argv.includes("--initial");
  let label = "";
  for (const a of argv) {
    if (a.startsWith("--label=")) label = a.slice("--label=".length);
  }
  const newSnap = takeSnapshot(label);
  const fname = path.join(STATE_DIR, `snap-${fileStamp()}.json`);
  fs.writeFileSync(fname, JSON.stringify(newSnap));
  // load the previous snapshot (listing excludes the file we just wrote)
  let oldName: string | null = null;
  let oldSnap: Snapshot | null = null;
  const files = fs
    .readdirSync(STATE_DIR)
    .filter(
      (f) =>
        f.startsWith("snap-") &&
        f.endsWith(".json") &&
        path.join(STATE_DIR, f) !== fname,
    )
    .sort();
  if (files.length > 0 && !isInitial) {
    oldName = files[files.length - 1];
    oldSnap = JSON.parse(
      fs.readFileSync(path.join(STATE_DIR, oldName), "utf-8"),
    );
  }
  if (oldSnap === null && !isInitial) {
    console.log("No previous snapshot found — treating this as baseline.");
  }
  report(newSnap, oldSnap, oldName, isInitial || oldSnap === null);
}

main();
