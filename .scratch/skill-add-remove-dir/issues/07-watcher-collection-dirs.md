# 07: Watcher tracks collection dirs

**What to build:** Make the fleet watcher watch every tracked collection dir, so edits inside a collection show as a diff. The watcher reads the tracked list from config and adds one target per entry.

**Blocked by:** 05.

**Status:** ready-for-agent

- [ ] The watcher takes one target per `skillsDirs` entry and diffs edits inside a tracked dir.
- [ ] A missing tracked dir is reported as MISSING like the other targets.
- [ ] The watcher skill document's target table and count match the code.
- [ ] Running the watcher against a temp home shows each tracked dir as a target.
