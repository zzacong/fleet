# 02: Dormant disables in sync and update

**What to build:** Sync stops projecting a disable for a skill that is installed nowhere, but keeps the state entry as dormant intent, so reinstalling the skill comes back disabled on the next command. A disable rule already written into a harness config is left byte-for-byte and not flagged as untracked, since the state still owns it. When the scan is incomplete, sync falls back to projecting every disable, because leaving an installed skill enabled is the worse failure.

The update report stops counting an uninstalled disable as held or lost, while an installed disabled skill is still verified.

**Blocked by:** 01 (Skill index completeness and installed names).

**Status:** ready-for-agent

- [ ] A state disable for a skill installed nowhere produces no new off entry in any config lever: OpenCode in both dialects, Pi, Codex, and Claude.
- [ ] A disable rule already present for an uninstalled skill is left byte-for-byte and not flagged as untracked.
- [ ] A config rule for a name the state does not track is still flagged as it is today.
- [ ] The state file is unchanged by sync and still holds the dormant entry.
- [ ] When the scan is incomplete, sync still projects the disable.
- [ ] `fleet skill update` no longer reports an uninstalled disabled skill as "did not stay disabled".
- [ ] An installed disabled skill is still verified as held by update.
- [ ] The existing disable-outlives-skill test is rewritten to assert dormancy.
- [ ] Sync tests drive the fake-home `sync.Run` seam.
