# doctor-drift-grouping

Status: in progress

Group `fleet skill doctor` state drift by harness and direction instead of printing one full sentence per skill.

## Problem

Doctor's state drift section prints one sentence per finding. A batch of the same drift, say nine custom skills enabled in the state but unlinked on two native-scanning harnesses, becomes eighteen near-identical lines. Each line repeats the whole explanation, and the only value that changes, the skill name, sits mid-sentence. The reader scans past the same hundred characters to find the part that differs.

The stale config and stale state sections already solved this shape: `printStaleSection` groups by harness, states the shared remediation once, then lists skill names. State drift never got the treatment, so it is the noisiest section.

## Solution

Render state drift like the stale sections. Group findings by harness and by drift direction, print the cause and the fix once per group, then list every skill name on its own line under the group. List all names; do not cap. Shorten a home-directory prefix in the directory annotation to `~`.

Drift has three directions:

- enabled in the state but not linked. Sync links it on the next command.
- disabled in the state but the managed link is still present. Sync removes the link.
- disabled in the state and not discoverable. The disable is moot until the link returns.

A group key is harness plus direction, so one harness can produce more than one group.

## Shape

Before:

```
⚠ state drift (2)
opencode  "my-notes" is enabled in fleet's state, but opencode cannot discover it (no link in ~/.config/opencode/skills) — sync links it on the next command
opencode  "my-docs" is enabled in fleet's state, but opencode cannot discover it (no link in ~/.config/opencode/skills) — sync links it on the next command
```

After:

```
⚠ state drift (2)
  opencode  enabled but not linked in ~/.config/opencode/skills — sync links on the next command
            my-notes, my-docs
```

## Implementation Decisions

**A structured drift reason.** The report currently carries only the fully formed sentence, so the CLI cannot group by cause without parsing prose. `doctor.Finding` gains a `Reason` field of a new `DriftReason` type with one constant per direction, and a `Dir` field for the harness skills directory the drift concerns. The existing `Message` stays for the doctor package's own tests and any non-grouped reader; the CLI ignores it for drift.

**The group renderer.** `printFindings` special-cases `KindDrift` and calls a new `printDriftSection`, mirroring how it already special-cases the two stale kinds. Groups keep first-appearance order, matching `printStaleSection`.

**Home shortening.** A small `shortenHome` helper replaces a leading user-home prefix with `~` for display. It is presentation-only and lives in the CLI.

**No cap.** Unlike the stale sections, drift lists every name. Sync fixes drift automatically, so the full list is short-lived, but the user approved showing all of it.

## Testing Decisions

- Doctor unit tests assert the new `Reason` and `Dir` on each of the three drift paths.
- A CLI integration test builds custom skills in a tracked repo against the native-scanning harnesses, with one harness carrying two directions, and asserts one group line per harness and direction, the skill names listed under each, and that the old per-finding boilerplate is gone.
- A unit test covers `shortenHome`: a path under home, the home itself, a path outside home, and the empty string.
- The docs sample in `www/src/content/docs/cli.md` is updated to the grouped shape.
