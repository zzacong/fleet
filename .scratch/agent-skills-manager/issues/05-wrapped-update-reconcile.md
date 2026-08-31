# 05: Wrapped update + post-update reconcile

**What to build:** `fleet skill update` keeps the `skills` CLI as the update backend but makes it safe for disabled skills: run it non-interactively, then auto-sync so disabled skills stay disabled and cleaned links stay clean, no matter what the skills CLI re-created. Fleet reports results from its own state, not the skills CLI's prose.

**Blocked by:** 02, 04.

**Status:** ready-for-agent

- [ ] `fleet skill update` shells out to `skills update -g -y` with fully explicit flags; stdin is piped closed so an unexpected prompt fails fast instead of hanging; output is captured and shown on failure.
- [ ] After the wrapped call, sync runs automatically: disabled markers re-applied, redundant links re-removed.
- [ ] Results are reported from fleet's own post-run state (state file + lockfile + configs), not by parsing the skills CLI's text. Raw skills CLI output is available on failure.
- [ ] Fleet never writes the skills CLI lockfile (read-only) and never deletes entries it doesn't own; running `skills` by hand stays fully functional.
- [ ] Works when upstream didn't change anything (no-op path is clean, not an error).
- [ ] Integration test in a fake home: disable a skill, simulate a skills-CLI run that re-creates its link/marker, run `fleet skill update`, verify the skill stays disabled and the link is gone.
