# 03: Doctor reports stale disables

**What to build:** `fleet doctor` reports two leftovers. First, a fleet-owned disable rule in a harness config whose skill is installed nowhere. Second, a state entry whose skill is installed nowhere. Each finding names the skill and harness and points at `fleet skill prune`; the state finding warns that pruning loses the disable-on-reinstall behavior.

The config-versus-state comparison learns that a missing rule for an uninstalled skill is expected, not drift. When the scan is incomplete, doctor suppresses both stale findings and names the home that blocked the scan, because a name that looks uninstalled might be installed in the unscanned home. Doctor stays read-only.

**Blocked by:** 01 (Skill index completeness and installed names).

**Status:** ready-for-agent

- [ ] A fleet-owned config rule for an uninstalled skill yields a stale-config finding.
- [ ] A state entry for an uninstalled skill yields a stale-state finding with the reinstall warning.
- [ ] Both findings name the skill and harness and point at `fleet skill prune`.
- [ ] An installed skill whose rule is missing still yields the existing drift or conflict, not a stale finding.
- [ ] An incomplete scan suppresses both stale findings and reports the blocking home.
- [ ] Doctor remains read-only.
- [ ] Doctor tests drive the `doctor.Analyze` seam over a fake home.
