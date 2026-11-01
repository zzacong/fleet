# ADR 0008: Hidden harnesses live in fleet config and gate through a managed list

Date: 2026-11-01
Status: Accepted

## Context

Fleet detects a harness by its config directory and manages every installed one. A user can keep a harness on disk they no longer use (IBM Bob, for one) and want fleet to stop showing it and stop touching it. Deleting the directory is not an option, so hiding has to be a fleet-side setting.

## Decision

1. A hidden harness is a machine-local list, `hiddenHarnesses` in `~/.config/fleet/config.json`. It is not a field in `state.json`: hiding describes this machine's directories, while the state file records per-skill enablement.
2. `fleet harness hide <name>` and `fleet harness unhide <name>` maintain the list. Hide refuses a name that is not a supported harness or is not installed, so a typo cannot write a dead key. Both are idempotent.
3. A managed list (installed and not hidden) is the one gate for writes and checks: sync, doctor, prune, toggle, customs, the TUI, and `skill ls`. `harness.All` stays the full detected list for `harness ls`, and `Installed` keeps meaning detection only.
4. The read model behind `skill ls` and the TUI (`snapshot`) builds its harness columns from the managed list, so a hidden harness has no column. The watcher is a separate script that reads fixed paths and keeps watching a hidden harness's files.
5. Hiding writes and deletes nothing. Existing links and state entries stay on disk. Unhide syncs, so they come back in the same run.

## Consequences

- Two lists differ on purpose. A future call site must choose between managed (acts on the harness or shows it to the user) and installed (detection, as `harness ls` reports it).
- Archive skips a hidden harness's state entries and Claude `skillOverrides`. A custom skill archived while its harness is hidden leaves a dangling link and a stale state entry there. Doctor does not report either until unhide. Accepted: fleet does not write into a harness it has been told to ignore.
- Toggle and skill commands that name a hidden harness fail with a hint to run `fleet harness unhide <name>`, rather than skipping silently.

## Alternatives considered

- **Store hidden state in `state.json`.** Rejected: that file holds per-skill enablement, and hiding describes one machine's directories.
- **Fold hidden into `Installed(p)`.** Rejected: the `harness.All`-based paths (prune, doctor, toggle) would still act on hidden harnesses, and `harness ls` would lose real detection.
- **Check hidden at each call site.** Rejected: the check would be repeated at many sites with nothing shared, so the next site added would miss it.
- **Remove a hidden harness's links on hide.** Rejected: a hide that deletes files is destructive, and unhide would have nothing to restore.
