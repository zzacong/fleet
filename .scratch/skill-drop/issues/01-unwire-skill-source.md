# 01: Unwire a collection from opencode and pi

**What to build:** `UnwireSkillSource(p, dir)` in `internal/harness`, removing a dropped collection dir from every installed config-path harness (opencode `skills`/`skills.paths`, pi `skills` array). Mirror of `WireSkillSource`; idempotent; empty removal leaves the config valid.

**Blocked by:** none.

**Status:** resolved

- [x] Removes dir from opencode V1 object shape (`skills.paths`) and V2 array shape (`skills`), no-op when absent.
- [x] Removes dir from pi `skills` array, no-op when absent.
- [x] Only installed harnesses touched; uninstalled configs never created or modified.
- [x] Unit tests over fixture configs for both dialects plus the no-op case.
