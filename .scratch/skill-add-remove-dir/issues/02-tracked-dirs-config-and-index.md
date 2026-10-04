# 02: Tracked collection dirs in config and the skill index

**What to build:** A `skillsDirs` list in the machine-local config, and a skill index that scans each listed collection directory directly. A collection dir is a directory whose immediate children are skill dirs, each with a `SKILL.md`. This is the tracked unit; the old repo-root plus `skills/` derivation is gone for these entries. The existing tracked repos keep working alongside while this lands, so the change can ship without a second model being user-visible yet.

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] Config accepts a `skillsDirs` array of absolute collection dirs, order significant, empty means none.
- [ ] `fleet skill ls` lists the skills of each listed dir, each child with a `SKILL.md`, scanned directly with no subdirectory derivation.
- [ ] Existing repo-root entries still resolve as before during this ticket.
- [ ] A listed dir that is missing from disk scans empty and does not error the listing.
- [ ] Precedence among `skillsDirs` entries and the older sources is deterministic and documented in the ADR.
- [ ] The config round-trip preserves order and unknown fields; a malformed `skillsDirs` value errors.
