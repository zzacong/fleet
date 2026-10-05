# 07: Watcher tracks collection dirs

**What to build:** Make the fleet watcher watch every tracked collection dir, so edits inside a collection show as a diff. The watcher reads the tracked list from config and adds one target per entry.

**Blocked by:** 05.

**Status:** resolved

- [x] The watcher takes one target per `skillsDirs` entry and diffs edits inside a tracked dir.
- [x] A missing tracked dir is reported as MISSING like the other targets.
- [x] The watcher skill document's target table and count match the code.
- [x] Running the watcher against a temp home shows each tracked dir as a target.

Verified by running `HOME=<temp> node scripts/watcher/watch.ts` with three
`skillsDirs` entries (two present, one missing): the run reported 16 targets
(13 fixed + 3 collections), `collection-1`/`collection-2` as `ok`,
`collection-3` as `MISSING`, and an edit inside `collection-1` produced a
unified diff. A missing or malformed config fell back to the 13 fixed targets.
`tsc -p scripts/watcher/tsconfig.json`, `oxlint .`, and `oxfmt --check` all
passed.
