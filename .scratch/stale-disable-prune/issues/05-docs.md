# 05: Docs for dormant disables and prune

**What to build:** The docs site describes that a disable survives an uninstall as dormant intent and is no longer written into harness configs, what doctor reports for the two stale cases, and how to use `fleet skill prune` including its flags and the warning that pruning a state entry loses the disable-on-reinstall behavior.

**Blocked by:** 02 (Dormant disables in sync and update), 03 (Doctor reports stale disables), 04 (`fleet skill prune`).

**Status:** resolved

- [x] Docs state that a disable survives an uninstall as dormant intent and is no longer written into harness configs.
- [x] Docs describe the two doctor findings and what they point at.
- [x] Docs document `fleet skill prune`, its flags, and the reinstall warning.
- [x] The docs site builds.
