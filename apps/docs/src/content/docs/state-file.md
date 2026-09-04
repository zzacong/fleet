---
title: State File Schema
description: Versioned, forward-compatible format.
---

# State file schema

Fleet's state file is the single source of truth for per-harness skill enablement. Harness config files are outputs derived from it; sync projects it, doctor compares against it, and nothing else writes it.

- **Location:** `~/.config/fleet/state.json` (`FLEET_HOME` overrides the home root)
- **Companion:** `~/.config/fleet/config.json` sits beside it and holds the machine-local customs settings (`{"skillsRepos": ["/abs/repo"], "adoptTarget": "/abs/repo/skills"}`, `FLEET_HOME`-aware): the explicit non-fleet-home repo-root list in precedence order (managed by `fleet skill pull`) and the adopt-target collection dir (absent means the fleet-home fallback). It is not part of the state file — see `fleet config get/set/unset/list` in the [command reference](cli.md#fleet-config). Enablement stays in `state.json`; the tracked set and adopt default stay in `config.json`.
- **Version:** 1

## Example

```json
{
  "version": 1,
  "skills": {
    "tdd": {
      "harnesses": {
        "claude": "off",
        "codex": "off",
        "opencode": "off",
        "pi": "off"
      }
    }
  }
}
```

## Shape

| Field                     | Type             | Meaning                                                                     |
| ------------------------- | ---------------- | --------------------------------------------------------------------------- |
| `version`                 | number, required | Schema version. Fleet reads and writes `1`.                                 |
| `skills`                  | object, optional | One entry per skill the state mentions. Absent = nothing disabled anywhere. |
| `skills.<name>.harnesses` | object, optional | Per-harness values. Fleet writes only `"off"`.                              |
| `skills.<name>.<other>`   | any              | Unknown per-skill fields (see forward compatibility).                       |
| `<other>` (top level)     | any              | Unknown top-level fields.                                                   |

Design rules:

- **Sparse, disable-only.** An entry exists only to record a disable. A skill with no entry is on everywhere, and once a skill's last disable is removed the whole entry disappears. There are no "on" records.
- **Keyed by skill name** (the frontmatter name, falling back to the directory name), not by directory, because harness rules target the name. Harness keys are fleet's harness IDs: `opencode`, `pi`, `codex`, `claude`, `cursor`, `bob`.

## Forward compatibility

The schema is built so newer or foreign writers lose nothing:

- **Unknown fields are preserved verbatim** on every round-trip: unknown top-level fields, unknown fields inside a skill entry, and harness values fleet doesn't recognize. They are rendered after fleet's own fields, in sorted order.
- **Enabling never destroys what it doesn't understand.** Removing a disable deletes only the exact `"off"` value it wrote; a future value (say, an object with scheduling metadata) stays. A skill entry with unknown fields survives even when its `harnesses` map empties.
- **Version is a hard stop, not a guess.** A file with a version newer than fleet's is an error (`~/.config/fleet/state.json: state file version 2 is newer than fleet's (want 1)`), as is a file with no version at all. Fleet never silently reinterprets content it doesn't recognize; upgrade the binary instead.
- **A missing file is an empty state**, not an error. Every fleet command works on a fresh machine.

## Write behavior

- **Atomic.** The file is written to a temp file and renamed, so a crash cannot truncate it.
- **Canonical formatting.** Two-space indent, fleet's fields first in fixed order (`version`, then `skills`), unknown fields after in sorted order. Hand-editing is fine — the next command re-reads the file as truth — but the formatting normalizes on the next write.
- **Fixed key order, sorted skill names.** Output is deterministic, which keeps diffs between runs meaningful.

## Relationship to the skills CLI

Fleet never writes the skills CLI's lockfile (`~/.agents/.skill-lock.json`), and the skills CLI knows nothing about this file — the two formats coexist. The schema keeps the shape of the upstream skills CLI's enable/disable proposal (PR #641's `enabled` field) in mind, so a later convergence with upstream stays possible.
