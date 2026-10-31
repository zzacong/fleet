# 02: Refuse unsafe archives

**What to build:** `fleet skill archive` refuses four cases with a clear error and leaves the disk and state untouched: a name that is not found, an installed skill, a name that shadows a canonical skill, a name already present in `archived/`, and a name that matches more than one custom copy. The shadow and installed errors name the canonical skill, and the ambiguity error lists the paths of the matching copies.

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] Archiving an unknown name fails and says the skill was not found
- [ ] Archiving an installed skill fails, names the canonical skill, and points at the `skills` CLI
- [ ] Archiving a custom skill whose name matches a canonical skill fails and names that canonical skill
- [ ] Archiving when `archived/` already holds that name fails and tells the user to rename or remove the existing folder
- [ ] Archiving a name that matches several custom copies fails and lists each path
- [ ] Every refusal leaves the folder, links, and state file exactly as they were
