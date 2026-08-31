import { readFileSync } from "node:fs";
import { useEffect, useMemo, useRef, useState } from "react";

export interface Root {
  name: string;
  path: string;
}

export interface SkillRow {
  name: string;
  description: string;
  enabled: boolean;
  roots: string[];
}

export interface Fixture {
  version: number;
  roots: Root[];
  skills: SkillRow[];
}

const FIXTURE_URL = new URL("../../fixture/skills.json", import.meta.url);

export function loadFixture(): Fixture {
  return JSON.parse(readFileSync(FIXTURE_URL, "utf8")) as Fixture;
}

/** Duplicate fixture rows with suffixed names up to `target` entries (--rows N). */
export function expandRows(rows: SkillRow[], target: number): SkillRow[] {
  if (!Number.isInteger(target) || target <= rows.length) return rows;
  const out = rows.slice();
  let i = 0;
  while (out.length < target) {
    const base = rows[i % rows.length]!;
    i += 1;
    out.push({ ...base, name: `${base.name}-${i}` });
  }
  return out;
}

export type Phase = "idle" | "applying";

const APPLY_DELAY_MS = 300;
const REFRESH_DELAY_MS = 200;

export function useSkillctl(initialRows: SkillRow[], targetRows: number) {
  const [rows, setRows] = useState<SkillRow[]>(initialRows);
  const [filter, setFilter] = useState("");
  const [filterFocused, setFilterFocused] = useState(false);
  const [staged, setStaged] = useState<ReadonlySet<string>>(new Set());
  const [selectedId, setSelectedId] = useState<string | null>(
    initialRows[0]?.name ?? null,
  );
  const [phase, setPhase] = useState<Phase>("idle");
  const [preview, setPreview] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const applyTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastCursor = useRef(0);

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter(
      (r) =>
        r.name.toLowerCase().includes(q) ||
        r.description.toLowerCase().includes(q),
    );
  }, [rows, filter]);

  // Cursor index into `filtered`: keep the same skill selected while it is
  // still visible; otherwise fall back to the nearest previous position.
  const cursor = useMemo(() => {
    if (filtered.length === 0) return 0;
    const idx = filtered.findIndex((r) => r.name === selectedId);
    if (idx >= 0) return idx;
    return Math.min(lastCursor.current, filtered.length - 1);
  }, [filtered, selectedId]);

  useEffect(() => {
    lastCursor.current = cursor;
    if (filtered.length > 0 && filtered[cursor]!.name !== selectedId) {
      setSelectedId(filtered[cursor]!.name);
    }
  }, [cursor, filtered, selectedId]);

  useEffect(() => {
    return () => {
      if (applyTimer.current) clearTimeout(applyTimer.current);
    };
  }, []);

  const move = (delta: number) => {
    if (filtered.length === 0) return;
    const next = Math.min(Math.max(cursor + delta, 0), filtered.length - 1);
    setSelectedId(filtered[next]!.name);
    setMessage(null);
  };

  const stage = (id: string | undefined) => {
    if (!id) return;
    setStaged((s) => {
      const next = new Set(s);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
    setMessage(null);
  };

  const apply = () => {
    if (phase !== "idle" || staged.size === 0) return;
    const ids = [...staged];
    const label =
      ids.length <= 3 ? ids.join(", ") : `${ids.slice(0, 3).join(", ")} +${ids.length - 3} more`;
    setPreview(`apply ${ids.length} change(s): ${label}`);
    setPhase("applying");
    applyTimer.current = setTimeout(() => {
      setRows((rs) =>
        rs.map((r) =>
          staged.has(r.name) ? { ...r, enabled: !r.enabled } : r,
        ),
      );
      setStaged(new Set());
      setPreview(null);
      setPhase("idle");
      setMessage(`applied ${ids.length} change(s)`);
    }, APPLY_DELAY_MS);
  };

  const cancelApply = () => {
    if (applyTimer.current) clearTimeout(applyTimer.current);
    setPhase("idle");
    setPreview(null);
    setMessage("apply cancelled");
  };

  const refresh = () => {
    if (refreshing) return;
    setRefreshing(true);
    setTimeout(() => {
      const fresh = expandRows(loadFixture().skills, targetRows);
      setRows(fresh);
      setStaged(new Set());
      setRefreshing(false);
      setMessage("fixture reloaded");
    }, REFRESH_DELAY_MS);
  };

  return {
    rows,
    filtered,
    filter,
    setFilter,
    filterFocused,
    setFilterFocused,
    staged,
    cursor,
    move,
    stage,
    phase,
    preview,
    message,
    refreshing,
    apply,
    cancelApply,
    refresh,
  };
}
