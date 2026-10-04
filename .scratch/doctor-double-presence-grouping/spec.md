# doctor-double-presence-grouping

Status: in progress

Group `fleet skill doctor` double presence by harness and by the colliding copies, instead of printing one full paragraph per skill.

## Problem

Doctor's double-presence section prints one sentence per finding. When two tracked repos share a dozen skill names, the section becomes a dozen near-identical paragraphs, each naming both copy paths and repeating "opencode and pi would see it twice, and one copy's rules may shadow the other — resolve by hand". The only value that changes, the skill name, sits mid-sentence, and the absolute paths are long enough to wrap in a terminal.

The state drift and stale sections already solved this shape: group by harness, state the shared cause and fix once, then list skill names. Double presence never got the treatment.

## Solution

Render double presence like the other grouped sections. Each finding carries the copies it names and the installed native-scanning harnesses that would see the name twice. The CLI expands a finding under each affected harness, groups by harness and by the set of copies, prints the shared cause and the manual resolution once per group, then lists the skill names comma-joined. Copy paths shorten a leading home-directory prefix to `~`.

## Shape

Before:

```
⚠ double presence (10)
"babysit-pr" exists in both the explicit repo (/Users/zacong/Developer/projects/fleet/skills/babysit-pr) and the explicit repo (/Users/zacong/Developer/projects/agent-skills/skills/babysit-pr) — opencode and pi would see it twice, and one copy's rules may shadow the other — resolve by hand (remove one of the copies: /Users/zacong/Developer/projects/fleet/skills/babysit-pr, /Users/zacong/Developer/projects/agent-skills/skills/babysit-pr)
"choose-flow" exists in both …
```

After:

```
⚠ double presence (10)
  opencode  duplicated in ~/Developer/projects/fleet/skills/babysit-pr and ~/Developer/projects/agent-skills/skills/babysit-pr — remove one copy by hand
            babysit-pr, choose-flow, create-plan, file-pr, postplan, postplan-read, rebase-pr, ticket-sweep, worktree-finish, worktree-session
  pi        duplicated in ~/Developer/projects/fleet/skills/babysit-pr and ~/Developer/projects/agent-skills/skills/babysit-pr — remove one copy by hand
            babysit-pr, choose-flow, create-plan, file-pr, postplan, postplan-read, rebase-pr, ticket-sweep, worktree-finish, worktree-session
```

## Implementation Decisions

**Structured copies and harnesses.** `doctor.Finding` gains `Copies []DoublePresenceCopy` (each copy's `Home` collection dir and absolute `Path`, source order) and `Harnesses []string` (the installed native-scanning harnesses that see the collision twice, column order). The hardcoded "opencode and pi" prose is replaced by the computed set: the native scanners are OpenCode, Pi, Codex, Cursor, and Bob (`harness.SkillDirs` with `NativeScan`); Claude is link-only and does not scan the canonical store, so it never sees a name twice. The existing `Message` stays for the doctor package's own tests and any non-grouped reader.

**The group renderer.** `printFindings` special-cases `KindDoublePresence` and calls a new `printDoublePresenceSection`, mirroring the drift and stale special cases. A finding expands to one row per affected harness; rows group by harness plus the set of homes (the copy paths' leaf differs per skill, so the homes are the shared key), so a group's label is accurate for every name under it. Groups keep first-appearance order.

**Home shortening.** Reuses the existing `shortenHome` helper from the drift change.

**No cap.** Like drift, double presence lists every name; it is not prune-able, so the full list is the point.

## Testing Decisions

- A doctor unit test asserts `Copies` names each copy and `Harnesses` lists the native scanners (and excludes Claude).
- A CLI integration test builds two tracked repos that share a name, then asserts the harness column, the shared grouped label, the names under it, `~` shortening, and that the old per-finding prose is gone.
- The existing adoption-followups CLI test updates to the grouped shape.
- The docs sample and bullet in `www/src/content/docs/cli.md` are updated.
