# archive-custom-skill

Status: ready-for-agent

Archive a custom skill: move it out of its collection dir into a sibling `archived/` folder, and unlink it everywhere so it disappears from every harness. One-way in v1.

## Problem Statement

A custom skill that the user no longer wants has no clean exit. Fleet can enable or disable it per harness, and it can stop tracking the collection dir it lives in, but the skill's files stay where they are. The user has to remember which harnesses still hold a link, which state entries and Claude overrides still mention the name, and then delete or move the folder by hand. If they miss one, a stale link or a leftover `skillOverrides` entry keeps the old skill alive, or silently disables a future skill of the same name.

## Solution

`fleet skill archive <name>` (and an `A` key on a custom skill's row in the TUI) moves that skill's folder into an `archived/` folder beside its collection dir, removes every managed link to it, drops its state entries, and removes the Claude `skillOverrides` entry that fleet wrote for it. Sync then runs as on every command. The archive is one-way in v1: restoring it is a manual move.

Archive refuses anything it cannot do cleanly, with a message that says why: an installed skill, a name that shadows a canonical skill, a name already in `archived/`, and, in the CLI only, a name that matches more than one custom copy.

## User Stories

1. As a skill author, I want to archive a custom skill from the CLI, so that it stops appearing in every harness without me hunting for its links.
2. As a skill author, I want to archive a custom skill from the TUI, so that I can do it while I'm already looking at the skill list.
3. As a skill author, I want the archived folder to sit beside my collection dir, so that it stays in the same repo and gets committed with the skill.
4. As a skill author whose collection dir is the fleet-home fallback, I want archived skills to go to the fleet home's `archived/` folder, so that the fallback behaves the same way as a tracked dir.
5. As a skill author, I want archive to unlink the skill from every harness that links custom skills, so that no harness keeps a dangling or live copy.
6. As a skill author, I want archive to drop the skill's enablement entries on every harness, so that archiving does not leave config behind that refers to a skill that no longer exists.
7. As a Claude Code user, I want archive to remove the `skillOverrides` entry fleet wrote for the skill, so that a future skill with the same name is not silently disabled.
8. As a Claude Code user, I want archive to leave other values in `skillOverrides` alone, so that my own overrides survive.
9. As a skill author, I want archive to refuse when the target name already exists in `archived/`, so that I never overwrite an archived skill I still have.
10. As a skill author, I want the refusal to tell me to rename or remove the existing archived folder, so that I know how to unblock myself.
11. As a skill author, I want archive to refuse a custom skill that shadows a canonical skill of the same name, so that archiving it doesn't expose the installed copy in every harness.
12. As a skill author, I want the shadow refusal to name the canonical skill, so that I can see what would take over.
13. As a skill author, I want the TUI to show the same shadow refusal inline on the row, so that the two entry points agree.
14. As a skill author, I want archive to refuse an installed skill and point me at the `skills` CLI, so that fleet never moves a skill it doesn't own.
15. As a skill author, I want the TUI to refuse archiving an installed row with a notice, so that pressing `A` on a canonical row does nothing destructive.
16. As a skill author, I want `fleet skill archive <name>` to refuse an ambiguous name and list the paths, so that I choose which copy to archive.
17. As a TUI user, I want `A` to ask for confirmation before it moves anything, so that a stray keypress can't archive a skill.
18. As a TUI user, I want the TUI to archive the copy the selected row displays, so that the action matches what I can see.
19. As a TUI user, I want typing `A` in the filter box to stay text, so that filtering still works.
20. As a TUI user, I want the help overlay to list `A`, so that I can discover the action.
21. As a skill author, I want `fleet skill ls` and `fleet doctor` to ignore the archived folder, so that archived skills don't reappear as broken or unlinked.
22. As a skill author, I want the command output to name the new location and list each link removed, following the `remove-dir` output style, so that I can see what changed.
23. As a skill author, I want archive to fail without touching anything when the move cannot complete, so that a failed archive never leaves a half-removed skill.
24. As a skill author, I want fleet to leave git alone after archiving, so that committing the move is my decision.

## Implementation Decisions

- **Core flow:** One archive operation in the customs layer, shared by the CLI and the TUI. The CLI resolves a name to exactly one custom copy and refuses if there is more than one. The TUI passes the copy the row displays. Both paths run the same refusals and the same effects.
- **Target rule:** The archived folder is a sibling of the copy's collection dir, named `archived`. The rule is the same for the fleet-home fallback, which yields the fleet home's `archived` folder. No special case.
- **Refusals (in order):** not found; installed (matches a canonical skill by directory or frontmatter name, which names the canonical skill); ambiguous across custom copies (CLI only); target already exists in `archived/`.
- **Move:** Reuse the existing move helper, which falls back to copy-then-delete across filesystems. A failed copy leaves the source intact.
- **Link removal:** Unlink this skill's managed links on every harness, using the per-skill removal path. The rest of the collection's links stay.
- **State:** Drop the skill's enablement entries on every harness. This needs a new state operation that removes a skill's entries outright; the existing set-enabled operation only removes `off` markers.
- **Claude override:** Remove the skill's `skillOverrides` entry only when its value is the fleet `off` marker. Other values stay. This is a new adapter helper, because the sync write path can only write `off` entries for names that are still in state, so it cannot clear one for an archived skill.
- **Sync:** Runs after the archive, as on every command.
- **Output:** One headline line naming the new path, then one line per unlinked link, following the `remove-dir` output style.
- **TUI:** `A` on a selected custom row asks for confirmation before acting. On an installed row it shows a notice and does nothing. It refreshes the snapshot afterwards. The help overlay gets one new line.
- **Read paths:** The skill index does not scan `archived/`, so `ls` and doctor don't see archived skills without any extra work.
- **Naming:** The CLI verb is `archive`, and the command lives beside `add-dir` and `remove-dir`.

## Testing Decisions

- **What makes a good test here:** Assert only on externally visible results: the folder layout on disk, the link state in each harness directory, the content of the fleet state file and the Claude settings file, the command output, and the error text on refusal. Don't assert on internal helper calls.
- **Primary seam:** The `fleet skill archive` command, run against a fake home with a collection dir, as `remove-dir` is tested. One test per refusal, one for the happy path with links on every harness, one for the Claude override cleanup (including preserving a non-`off` value), one for the fallback target, and one for the failed-move case that proves nothing was removed.
- **TUI seam:** One test in the existing TUI test style covering the `A` key, the confirmation step, the installed-row notice, and that typing `A` in the filter stays text. The TUI test doesn't re-test the refusal rules that the CLI tests cover.
- **Not tested separately:** The new state operation and the Claude helper. They are covered through the command tests. No unit tests for them.
- **Prior art:** `internal/cli/removedir_test.go` for the command-level flow and the fake home; `internal/customs/destination_test.go` for refusal tests on the index; `internal/tui/tui_test.go` for TUI key tests.

## Out of Scope

- Unarchive, in any form. Restoring an archived skill is a manual move in v1.
- Showing archived skills in `ls`, the TUI, or doctor.
- Committing or staging the moved folder in git. Fleet doesn't run git.
- Archiving installed skills. Those belong to the `skills` CLI.
- Archiving a skill from inside a project's `.agents/skills/`. That is a project skill, which fleet does not manage.
- Any ADR for the sibling-folder location. Declined in the grilling session.

## Further Notes

- In a git-tracked collection dir, the archive shows up as a deletion plus an untracked folder until the user commits it.
- Open risk, not yet verified: an `archived` folder sits beside a collection dir. If a user later tracks the parent of that collection, the archived skills would be scanned as skills of that parent. Check this when the doctor and add-dir tests are written.
- Decision record: the grilling session (rounds 1–3) is the source for every decision above. The domain terms are in `CONTEXT.md` under **Archive**, and the Enable/Disable line there was amended to match.
