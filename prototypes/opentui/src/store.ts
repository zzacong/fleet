// store.ts — fixture loading + the useSkillctl hook, shared by cli.ts and main.tsx.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { readFileSync } from "node:fs";
import { join } from "node:path";

export interface Root {
  name: string;
  path: string;
}

export interface Skill {
  name: string;
  description: string;
  enabled: boolean;
  roots: string[];
}

export interface Fixture {
  roots: Root[];
  skills: Skill[];
}

export type Phase = "idle" | "applying" | "refreshing";

const FIXTURE_PATH = join(import.meta.dir, "../../fixture/skills.json");

export function loadFixture(): Fixture {
  return JSON.parse(readFileSync(FIXTURE_PATH, "utf8")) as Fixture;
}

/** Duplicate fixture rows with suffixed names up to `n` entries (stress mode). */
export function expandRows(skills: Skill[], n: number): Skill[] {
  if (n <= skills.length) return skills.slice(0, n);
  const out: Skill[] = [];
  for (let i = 0; out.length < n; i++) {
    const base = skills[i % skills.length];
    const copy = i + 1 <= skills.length ? base.name : `${base.name}-${Math.floor(i / skills.length) + 1}`;
    out.push({ ...base, name: copy });
  }
  return out;
}

export interface RowParts {
  glyph: string;
  stagedMark: string;
  name: string;
  description: string;
  roots: string;
}

/** Lay out one row into fixed-width columns; description truncates with "…", roots last. */
export function formatRowParts(
  skill: Skill,
  staged: boolean,
  nameWidth: number,
  descWidth: number,
  fixedRootsWidth?: number,
): RowParts {
  const name = skill.name.length > nameWidth
    ? skill.name.slice(0, nameWidth - 1) + "…"
    : skill.name;
  const roots = skill.roots.join(", ");
  const rootsWidth = Math.min(roots.length, fixedRootsWidth ?? 24);
  const shownRoots = roots.length > rootsWidth
    ? roots.slice(0, rootsWidth - 1) + "…"
    : roots;
  const available = Math.max(8, descWidth - rootsWidth - 2);
  const description = skill.description.length > available
    ? skill.description.slice(0, available - 1) + "…"
    : skill.description;
  return {
    glyph: skill.enabled ? "●" : "○",
    stagedMark: staged ? "*" : " ",
    name: name.padEnd(nameWidth),
    description: description.padEnd(available),
    roots: shownRoots.padStart(rootsWidth),
  };
}

const delay = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

const SPINNER_FRAMES = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];

export interface SkillctlState {
  rows: Skill[];
  filtered: Skill[];
  filter: string;
  selectedName: string | null;
  cursor: number;
  staged: ReadonlySet<string>;
  phase: Phase;
  preview: string | null;
  message: string | null;
  counts: { total: number; enabled: number; disabled: number; stagedCount: number };
  move: (delta: number) => void;
  stage: (name: string) => void;
  apply: () => void;
  cancelApply: () => void;
  refresh: () => void;
  setFilter: (value: string | ((prev: string) => string)) => void;
  clearTransient: () => void;
}

/**
 * All skillctl state: rows, filter, staged set, selection, phase.
 * Apply and refresh are async but simulated — nothing touches disk.
 */
export function useSkillctl(initialRows: Skill[], expandTo: number = initialRows.length): SkillctlState {
  const [rows, setRows] = useState<Skill[]>(initialRows);
  const [filter, setFilterValue] = useState("");
  const [staged, setStaged] = useState<ReadonlySet<string>>(new Set());
  const [selectedName, setSelectedName] = useState<string | null>(initialRows[0]?.name ?? null);
  const [phase, setPhase] = useState<Phase>("idle");
  const [preview, setPreview] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const applyTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (applyTimer.current) clearTimeout(applyTimer.current);
    };
  }, []);

  const filtered = useMemo(() => {
    const q = filter.toLowerCase();
    if (!q) return rows;
    return rows.filter(
      (r) => r.name.toLowerCase().includes(q) || r.description.toLowerCase().includes(q),
    );
  }, [rows, filter]);

  // Selection tracks the same skill across filter edits while it stays visible.
  // Falls back to the top row when the selection is filtered out.
  const cursor = useMemo(() => {
    if (selectedName) {
      const i = filtered.findIndex((r) => r.name === selectedName);
      if (i >= 0) return i;
    }
    return 0;
  }, [filtered, selectedName]);

  // Mirrors `cursor` synchronously so bursted key repeats (held j) accumulate
  // instead of all reading the same committed value.
  const cursorRef = useRef(cursor);
  cursorRef.current = cursor;

  const move = useCallback(
    (delta: number) => {
      if (filtered.length === 0) return;
      const next = Math.min(Math.max(0, cursorRef.current + delta), filtered.length - 1);
      cursorRef.current = next;
      setSelectedName(filtered[next]?.name ?? null);
      setMessage(null);
      setPreview(null);
    },
    [filtered],
  );

  const stage = useCallback((name: string) => {
    setStaged((s) => {
      const next = new Set(s);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
    setMessage(null);
  }, []);

  const apply = useCallback(() => {
    if (phase !== "idle" || staged.size === 0) return;
    const names = [...staged];
    const willEnable = names.filter((n) => !rows.find((r) => r.name === n)?.enabled);
    const willDisable = names.filter((n) => rows.find((r) => r.name === n)?.enabled);
    setPreview(
      `apply ${names.length} change(s): ${[
        ...willEnable.map((n) => `enable ${n}`),
        ...willDisable.map((n) => `disable ${n}`),
      ].join(", ")}`,
    );
    setPhase("applying");
    applyTimer.current = setTimeout(() => {
      setRows((rs) => rs.map((r) => (staged.has(r.name) ? { ...r, enabled: !r.enabled } : r)));
      setStaged(new Set());
      setPreview(null);
      setPhase("idle");
      setMessage(`applied ${names.length} change(s)`);
    }, 300); // simulated work
  }, [phase, staged, rows]);

  const cancelApply = useCallback(() => {
    if (applyTimer.current) clearTimeout(applyTimer.current);
    applyTimer.current = null;
    setPhase("idle");
    setPreview(null);
    setMessage("apply cancelled");
  }, []);

  const refresh = useCallback(() => {
    if (phase !== "idle") return;
    setPhase("refreshing");
    void (async () => {
      await delay(250); // brief spinner so the reload path is felt
      const fresh = expandRows(loadFixture().skills, expandTo);
      setRows(fresh);
      setStaged(new Set());
      setFilterValue("");
      setSelectedName(fresh[0]?.name ?? null);
      setPhase("idle");
      setMessage("reloaded fixture from disk");
    })();
  }, [phase]);

  const clearTransient = useCallback(() => {
    setPreview(null);
    setMessage(null);
  }, []);

  const enabled = rows.reduce((n, r) => n + (r.enabled ? 1 : 0), 0);

  return {
    rows,
    filtered,
    filter,
    selectedName,
    cursor,
    staged,
    phase,
    preview,
    message,
    cancelApply,
    counts: {
      total: rows.length,
      enabled,
      disabled: rows.length - enabled,
      stagedCount: staged.size,
    },
    move,
    stage,
    apply,
    refresh,
    setFilter: setFilterValue,
    clearTransient,
  };
}

/** Simple hand-rolled spinner text (caller re-renders on an interval). */
export function spinnerFrame(tick: number): string {
  return SPINNER_FRAMES[tick % SPINNER_FRAMES.length];
}
