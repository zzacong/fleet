# 02: `skill pull` end-to-end

**What to build:** A user can run `skill pull <git-url> [path]` to clone a customs repo (defaulting into the auto-tracked fleet-home slot, registering only when outside fleet home) and bare `skill pull` to fast-forward every tracked repo, with a per-repo report, running against real git in production and a stubbed runner in tests.

**Blocked by:** 01 (Config + tracked-set foundation).

**Status:** resolved

- [x] Fresh pull with no path clones into the derived fleet-home slot with no config write; explicit inside-home path clones with no config write; explicit outside-home path clones and appends the repo root once without reordering.
- [x] Existing path with the same remote fast-forwards only; different remote fails unless forced; dirty/diverged working tree fails surfacing state; missing git fails cleanly; non-git entries skip with a warning on bare pull.
- [x] Post pull ensures the collection dir (creating with a warning when absent), wires/links the home, and syncs; output reports per-repo outcome (updated, current, skipped, failed).
