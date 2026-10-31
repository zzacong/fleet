# 03: Clear Claude's override when archiving

**What to build:** When a custom skill is archived, fleet removes the `skillOverrides` entry it wrote for that skill in Claude Code, so a future skill with the same name is not silently disabled. Any other value Claude holds for that name stays.

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] After archiving a skill that Claude had disabled, its `skillOverrides` entry is gone from the Claude settings file
- [ ] An override with a value other than fleet's `off` marker for that name is left in place
- [ ] Other entries in `skillOverrides` are unchanged
- [ ] When removing the entry leaves `skillOverrides` empty, the object is removed as well
