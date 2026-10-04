# doctor-group-remaining-sections

Status: in progress

Group the remaining `fleet skill doctor` sections that still print one full
sentence per finding.

## Problem

Four sections already collapse shared prose: state drift (grouped by harness
and direction), stale config rules and stale state entries (grouped by harness,
capped), and double presence (grouped by harness and pair of homes). Five
sections still print one near-identical line per finding — the cause and the
fix repeat, and the skill name sits mid-sentence:

| Kind | Title | What repeats |
| --- | --- | --- |
| `KindRedundantLink` | redundant links | the target path, "scans the canonical store natively — this link double-covers the skill", "sync removes it" |
| `KindBrokenLink` | broken symlinks | the target path and the reason ("the symlink target is missing", a loop, or a link-only harness) |
| `KindUnknownEntry` | unknown entries | the reason ("a real directory, not a symlink — left alone", and the foreign-link variants) |
| `KindManualEdit` | manual edits fleet can't manage | the cause and "edit the config by hand if that's wrong" |
| `KindStaleLock` | stale lockfile entries | the longest message in the report: home, lockfile path, the moved-skill explanation, and the remediation |

The remaining kinds are singletons or already one finding per situation and
stay as they are: `KindIncompleteScan` (one finding naming every blocked home),
`KindUnscannedAdoptTarget` (one per config), `KindNonGitRepo`, `KindMissingDir`,
and `KindBrokenConfig`.

## Solution

- **Cause-grouped sections.** Redundant links, broken symlinks, unknown
  entries, and manual edits render as one line per harness and cause, then the
  skill names comma-joined under it. This mirrors the drift section: the
  harness is the column, the cause prints once, and the names hang below.
- **Stale lockfile entries.** Grouped by custom home: one line per home (the
  collection path shortened to `~`), names under it, and the shared
  remediation hoisted into the section note.
- **Structured fields, not message parsing.** `harness.Entry` gains `Cause`
  (the classification explanation without the harness or entry name) and
  `doctor.Finding` gains `Cause` (for the four cause-grouped sections) plus
  `Home`/`HomeLabel` (for stale lock). `Message` stays for the doctor package's
  tests and any non-grouped reader.

## Shape

Before:

```
⚠ redundant links (4) · sync removes them
  opencode  link "alpha" → ~/.agents/skills/alpha — opencode scans the canonical store natively — this link double-covers the skill; sync removes it
  opencode  link "bravo" → ~/.agents/skills/bravo — opencode scans the canonical store natively — this link double-covers the skill; sync removes it
```

After:

```
⚠ redundant links (4) · sync removes them
  opencode  scans the canonical store natively — this link double-covers the skill
            alpha, bravo
```

Stale lockfile entries:

```
⚠ stale lockfile entries (3) · fleet never writes the lockfile — remove each entry by hand; the skills CLI keeps updating these moved skills
  the explicit repo  ~/repos/skills
                     lock-a, lock-b, lock-c
```

## Testing Decisions

- The doctor unit tests assert `Cause` on redundant, broken, unknown, and
  manual-edit findings, and `Home`/`HomeLabel` on stale-lock findings.
- A CLI integration test builds a home with redundant links on two scanners,
  two broken links, two unknown entries, and a pattern rule covering two
  skills, then asserts one cause line per harness and the names under it.
- A CLI integration test groups stale-lock entries across a tracked repo and
  the fallback, asserting one home line each, `~` shortening, and that the old
  per-finding prose is gone.
- Docs: the `www` command reference bullet list and sample, and the architecture
  note.
