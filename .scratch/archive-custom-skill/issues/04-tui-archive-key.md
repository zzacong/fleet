# 04: Archive from the TUI with the A key

**What to build:** In the TUI, `A` on a selected custom skill asks for confirmation, then archives the copy the row shows, using the same rules as the CLI. Installed rows show a notice and do nothing. Refusals appear inline on the row. The help overlay lists the key. Typing `A` while the filter is focused stays text.

**Blocked by:** 02, 03

**Status:** ready-for-agent

- [ ] `A` on a custom row shows a confirmation prompt and changes nothing until confirmed
- [ ] Confirming archives the copy the row displays and refreshes the list
- [ ] Cancelling the prompt leaves everything unchanged
- [ ] `A` on an installed row shows a notice and does nothing
- [ ] A refusal from the core shows inline on the row
- [ ] Typing `A` in the filter box adds the letter to the filter
- [ ] The help overlay lists `A`
