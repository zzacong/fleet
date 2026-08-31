# Research: IBM Bob and Cursor skill support (2026-08-31)

Sources for the Bob/Cursor claims in the spec. Verified against official docs, source, and issue trackers on 2026-08-31.

## Summary verdict

| | Cursor | IBM Bob |
|---|---|---|
| Global skill dir | `~/.cursor/skills/` plus `~/.agents/skills/` natively | `~/.bob/skills/` only |
| Project skill dir | `.cursor/skills/`, `.agents/skills/` (plus `.claude/skills/`, `.codex/skills/` compat), recursive incl. monorepo subdirs | `<project>/.bob/skills/` only |
| Reads `~/.agents/skills` natively | Yes (documented) | Not documented; **user-verified empirically on this machine** |
| Per-skill disable | `disable-model-invocation: true` in SKILL.md frontmatter (soft: still `/`-invocable). No hard-disable documented | No documented file syntax; IDE UI toggle exists, persistence location undocumented |
| Extra skill dirs | None (hardcoded root list) | Only undocumented `context.includeDirectories`; unverified for skill discovery |
| Symlinks | Supported since IDE 2.5 (Feb 2026) and CLI build 2026.02.27; flaky reports on early 2.5.x | No skills-specific statement; `.bob/rules/` symlink fix in 2.0.1 (Jul 2026) is analogous |
| Docs quality | Good — adapter-buildable | Thin beyond "drop a folder in `~/.bob/skills/`" |

## Cursor

- **Scanned roots (official):** `.agents/skills/`, `.cursor/skills/` (project) and `~/.agents/skills/`, `~/.cursor/skills/` (user-level); compat roots `.claude/skills/`, `.codex/skills/` (and user-level equivalents). Recursive, category subfolders allowed, monorepo nested `.cursor/skills`/`.agents/skills` picked up and scoped to their subtree. Skills load per-conversation. https://cursor.com/docs/skills (fetched 2026-08-31); same table at https://cursor.com/help/customization/skills
- **Per-skill disable:** `disable-model-invocation: true` frontmatter → skill only loads on explicit `/skill-name` invocation. Only documented per-skill control; no full-removal setting, no per-skill UI toggle. Also documented: `paths` glob scoping (legacy `globs`). https://cursor.com/docs/skills#disabling-automatic-invocation
- **Extra skill dirs:** none. `~/.cursor/cli-config.json` / project `.cursor/cli.json` have no skills keys; no skills env var (`CURSOR_CONFIG_DIR` only relocates config). https://cursor.com/docs/cli/reference/configuration (fetched 2026-08-31)
- **Symlinks:** bug filed Jan 23 2026 (v2.4.21), staff-confirmed not followed; fixed in IDE 2.5 (Feb 17 2026, staff-confirmed Feb 20) and CLI build 2026.02.27 (user-verified Feb 28). Residual: 2.5.20/2.5.22 users reported symlinked skills vanishing from the Skills tab after restart (unresolved on record). Cloud Agents never see local user-level skills. https://forum.cursor.com/t/149693, https://forum.cursor.com/t/151014, https://forum.cursor.com/t/153603
- **vercel-labs/skills cross-check:** its table lists only `~/.cursor/skills/` for Cursor — incomplete; the Cursor docs page is authoritative.

## IBM Bob

- **Scanned roots (official):** `<project>/.bob/skills/` and `~/.bob/skills/` only. Project wins on collision. Skills need Advanced/agent mode; missing `description` → ignored. https://bob.ibm.com/docs/ide/features/skills (published 2025-10-06, current through Bob 2.0.3 Aug 2026); https://bob.ibm.com/docs/shell/features/skills
- **`~/.agents/skills`:** not documented as a root. Community workaround via undocumented `context.includeDirectories` in `~/.bob/settings/settings.json` (Ramon Wartala, IBM, Medium, Mar 29 2026: https://medium.com/@ramwar/skills-for-enterprise-ready-coding-with-ibm-bob-fe09f276b2b3) — but an IBM community post (Jul 14 2026) describes that key as a command-tool access gate, not skill discovery (https://community.ibm.com/community/user/blogs/randhir-singh/2026/07/14/using-ibm-bob-to-investigate-an-incident-and-propo). Unverified as a discovery mechanism. **Local override: this machine's Bob reads `~/.agents/skills` with no extra config (user-verified empirically; docs don't say so).**
- **Config files:** user `~/.bob/settings/settings.json`, workspace `<project>/.bob/settings.json`, trust `~/.bob/trustedFolders.json`. Bob Shell 2.x requires one-time explicit user approval for writes to `.bob/settings.json` and `~/.bob/*/settings.json`. https://bob.ibm.com/docs/shell/configuration/configuring
- **Per-skill disable:** no documented syntax. Global `"autoApprove": { "skills": true }` is documented (global, not per-skill). IDE has a per-skill toggle ("Allow Bob to use this skill") whose persistence location is undocumented; generated SKILL.md shows an undocumented `user-invocable` frontmatter field. https://bob.ibm.com/docs/ide/tutorials/use-skills
- **Unknown-key tolerance:** unknown (closed source; docs only cover invalid JSON).
- **Known bug:** #288 (open since May 15 2026): "Skills located in ~/.bob/skills do not appear to be working. Only project-level skills are being seen." Possibly resolved by the 2.0.0 Skills settings tab, but unconfirmed. https://github.com/IBM/ibm-bob/issues/288; sibling #204 (closed May 4). Repo is a stub (closed source): https://github.com/IBM/ibm-bob
- **Adapter implication:** Bob = link-presence toggle in `~/.bob/skills` (docs fully support writing/symlinking folders there); no config-level per-skill disable. NOTE: if Bob natively reads `~/.agents/skills` (as verified on this machine), link removal may NOT hide skills from Bob — the disable mechanism for Bob needs live verification before shipping.
