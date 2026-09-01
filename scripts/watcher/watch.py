#!/usr/bin/env python3
"""
Fleet Watcher — snapshot & diff every file fleet touches.

What it does
------------
Takes a content-hashed snapshot of all fleet-managed locations (fleet state,
canonical skill store, and each harness's config + skills dir) and diffs it
against the previous snapshot.  Modified files get a unified line diff via a
deduped content store; identical-content rewrites are reported as "touched"
rather than "modified"; empty-dir creation/removal is reported explicitly;
minified blobs are summarized by hash.

Usage
-----
  python3 scripts/watcher/watch.py --initial --label=baseline   # record baseline
  python3 scripts/watcher/watch.py --label=go-1                 # snapshot + diff
  # chat shorthand "go" runs the second form

State lives in scripts/watcher/.state/ (gitignored) — snapshots as
snap-*.json and file contents deduped by SHA-1 under contents/.
"""

import hashlib
import json
import os
import sys
import time

HOME = os.path.expanduser("~")
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
# Fleet repo root is two levels above scripts/watcher/
REPO = os.path.abspath(os.path.join(SCRIPT_DIR, "..", ".."))
# State lives inside the repo (gitignored) so the watcher is self-contained.
STATE_DIR = os.path.join(SCRIPT_DIR, ".state")
CONTENT_DIR = os.path.join(STATE_DIR, "contents")

# (label, absolute path) — paths derived from fleet's internal/paths/paths.go
WATCH_TARGETS = [
    ("fleet-config",   os.path.join(HOME, ".config/fleet")),
    ("agents-store",   os.path.join(HOME, ".agents")),
    ("opencode-config", os.path.join(HOME, ".config/opencode/opencode.jsonc")),
    ("opencode-skills", os.path.join(HOME, ".config/opencode/skills")),
    ("pi-settings",    os.path.join(HOME, ".pi/agent/settings.json")),
    ("pi-skills",      os.path.join(HOME, ".pi/agent/skills")),
    ("codex-config",   os.path.join(HOME, ".codex/config.toml")),
    ("codex-skills",   os.path.join(HOME, ".codex/skills")),
    ("claude-skills",  os.path.join(HOME, ".claude/skills")),
    ("claude-config",  os.path.join(HOME, ".claude/settings.json")),
    ("cursor-skills",  os.path.join(HOME, ".cursor/skills")),
    ("bob-skills",     os.path.join(HOME, ".bob/skills")),
    ("bob-settings",   os.path.join(HOME, ".bob/settings.json")),
    ("repo-skills",    os.path.join(REPO, "skills")),
]

MAX_HASH = 4 * 1024 * 1024  # skip hashing files > 4MB (record size+mtime only)


def is_symlink_link_cycle_safe(root):
    return os.path.realpath(root)


def scan(target):
    """Return {abs_path: entry} for a file or directory target."""
    entries = {}
    if os.path.islink(target):
        entries[target] = {"type": "symlink", "target": os.readlink(target)}
        return entries
    if os.path.isfile(target):
        entries[target] = file_entry(target)
        return entries
    if not os.path.isdir(target):
        return entries  # missing — recorded implicitly by absence
    for dirpath, dirnames, filenames in os.walk(target):
        dirnames[:] = [d for d in dirnames if not os.path.islink(os.path.join(dirpath, d))]
        for d in dirnames:
            full = os.path.join(dirpath, d)
            if os.path.islink(full):
                entries[full] = {"type": "symlink", "target": os.readlink(full)}
        for f in filenames:
            full = os.path.join(dirpath, f)
            if os.path.islink(full):
                entries[full] = {"type": "symlink", "target": os.readlink(full)}
                continue
            try:
                entries[full] = file_entry(full)
            except OSError:
                entries[full] = {"type": "unreadable"}
    return entries


def file_entry(path):
    st = os.lstat(path)
    e = {"type": "file", "size": st.st_size, "mtime_ns": st.st_mtime_ns}
    if st.st_size <= MAX_HASH:
        h = hashlib.sha1()
        with open(path, "rb") as fh:
            for chunk in iter(lambda: fh.read(65536), b""):
                h.update(chunk)
        e["sha1"] = h.hexdigest()
    return e


def take_snapshot(label=""):
    snap = {"taken_at": time.strftime("%Y-%m-%dT%H:%M:%S"), "label": label, "targets": {}}
    for name, path in WATCH_TARGETS:
        snap["targets"][name] = {
            "root": path,
            "exists": os.path.exists(path),
            "entries": scan(path),
        }
    save_contents(snap)
    return snap


def save_contents(snap):
    """Store file contents under CONTENT_DIR/<sha1> (deduped) so later
    snapshots can show line-level diffs even after files change or vanish."""
    os.makedirs(CONTENT_DIR, exist_ok=True)
    for t in snap["targets"].values():
        for p, e in t["entries"].items():
            h = e.get("sha1")
            if not h:
                continue
            dest = os.path.join(CONTENT_DIR, h)
            if os.path.exists(dest):
                continue
            try:
                with open(p, "rb") as fh:
                    data = fh.read()
                with open(dest, "wb") as out:
                    out.write(data)
            except OSError:
                pass


def read_stored(sha1):
    try:
        with open(os.path.join(CONTENT_DIR, sha1), "rb") as fh:
            return fh.read().decode("utf-8", errors="replace").splitlines()
    except OSError:
        return None


def content_diff(path, old_e, new_e, max_lines=120):
    """Unified line diff for a modified file, from stored contents."""
    ha, hb = old_e.get("sha1"), new_e.get("sha1")
    if not ha or not hb:
        return None
    old_lines, new_lines = read_stored(ha), read_stored(hb)
    if old_lines is None or new_lines is None:
        return None
    import difflib
    # Skip minified/one-line blobs (e.g. 900KB JSON caches): a "line diff"
    # there is useless — the hash comparison above already flags the change.
    if max(len(l) for l in old_lines) > 500 or max(len(l) for l in new_lines) > 500:
        return "  (minified content — see hash change above)"
    d = list(difflib.unified_diff(
        old_lines, new_lines,
        fromfile=f"{path} (old)", tofile=f"{path} (new)", lineterm="",
    ))
    if not d:
        return None
    if len(d) > max_lines:
        d = d[:max_lines] + [f"  ... ({len(d) - max_lines} more diff lines)"]
    return "\n".join("  " + l for l in d)


def load_latest():
    files = sorted(
        f for f in os.listdir(STATE_DIR) if f.startswith("snap-") and f.endswith(".json")
    )
    if not files:
        return None, None
    latest = files[-1]
    with open(os.path.join(STATE_DIR, latest)) as fh:
        return latest, json.load(fh)


def diff_snapshots(old, new):
    lines = []
    for name, newt in new["targets"].items():
        oldt = old["targets"].get(name, {"exists": False, "entries": {}})
        oe, ne = oldt.get("entries", {}), newt["entries"]
        added = sorted(set(ne) - set(oe))
        removed = sorted(set(oe) - set(ne))
        modified = sorted(
            p for p in set(oe) & set(ne)
            if oe[p] != ne[p] and p not in added and p not in removed
            and oe[p].get("sha1") != ne[p].get("sha1")
        )
        touched = sorted(
            p for p in set(oe) & set(ne)
            if oe[p] != ne[p] and p not in added and p not in removed
            and oe[p].get("type") == "file" and oe[p].get("sha1")
            and oe[p].get("sha1") == ne[p].get("sha1")
        )
        if not (added or removed or modified or touched) and oldt.get("exists") == newt["exists"]:
            continue
        lines.append(f"\n## {name}  ({newt['root']})")
        if not oldt.get("exists") and newt["exists"]:
            lines.append(f"  + <dir created>")
        elif oldt.get("exists") and not newt["exists"]:
            lines.append(f"  - <dir removed>")
        for p in added:
            e = ne[p]
            detail = e.get("target", "") if e["type"] == "symlink" else f"{e.get('size', '?')}B"
            lines.append(f"  + {p}  [{e['type']} {detail}]")
        for p in removed:
            e = oe[p]
            detail = e.get("target", "") if e["type"] == "symlink" else f"{e.get('size', '?')}B"
            lines.append(f"  - {p}  [was {e['type']} {detail}]")
        for p in touched:
            lines.append(f"  · {p}  touched, content unchanged ({ne[p].get('size')}B)")
        for p in modified:
            a, b = oe[p], ne[p]
            if a["type"] == "symlink":
                lines.append(f"  ~ {p}  link: {a.get('target')} -> {b.get('target')}")
            else:
                ha, hb = a.get("sha1", "?")[:8], b.get("sha1", "?")[:8]
                lines.append(f"  ~ {p}  ({a.get('size')}B/{ha} -> {b.get('size')}B/{hb})")
                cd = content_diff(p, a, b)
                if cd:
                    lines.append(cd)
    return "\n".join(lines) if lines else "\nNo changes detected."


def homeify(path):
    return path.replace(HOME + "/", "~/", 1) if path.startswith(HOME) else path


def report(new_snap, old_snap, old_name, is_initial):
    print(f"Snapshot: {new_snap['taken_at']}  ({new_snap.get('label') or 'unnamed'})")
    total = sum(len(t["entries"]) for t in new_snap["targets"].values())
    print(f"Tracked entries: {total} across {len(WATCH_TARGETS)} targets")
    for name, t in new_snap["targets"].items():
        status = "ok" if t["exists"] else "MISSING"
        print(f"  {status:8} {name:14} {homeify(t['root'])}  ({len(t['entries'])} entries)")
    if is_initial:
        print("\nBaseline recorded. Say 'go' to diff against this snapshot.")
    else:
        print(f"\nDiffs vs previous snapshot ({old_name}):")
        print(diff_snapshots(old_snap, new_snap))


def main():
    os.makedirs(STATE_DIR, exist_ok=True)
    is_initial = "--initial" in sys.argv
    label = ""
    for a in sys.argv[1:]:
        if a.startswith("--label="):
            label = a[len("--label="):]
    new_snap = take_snapshot(label)
    fname = os.path.join(STATE_DIR, f"snap-{time.strftime('%Y%m%d-%H%M%S')}.json")
    with open(fname, "w") as fh:
        json.dump(new_snap, fh)
    # load the previous snapshot (listing excludes the file we just wrote)
    old_name, old_snap = None, None
    files = sorted(
        f for f in os.listdir(STATE_DIR)
        if f.startswith("snap-") and f.endswith(".json") and os.path.join(STATE_DIR, f) != fname
    )
    if files and not is_initial:
        with open(os.path.join(STATE_DIR, files[-1])) as fh:
            old_snap = json.load(fh)
        old_name = files[-1]
    if old_snap is None and not is_initial:
        print("No previous snapshot found — treating this as baseline.")
    report(new_snap, old_snap, old_name, is_initial or old_snap is None)


if __name__ == "__main__":
    main()
