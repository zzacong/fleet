# 01: Prefactor - highest-precedence home wins the managed link

**What to build:** When two custom homes hold a skill with the same name, the managed harness link points at the same home `fleet skill ls` shows as the winner, instead of the lowest-precedence home as it does today. This fixes a live bug and makes the collision policy of the new verbs trustworthy.

**Blocked by:** None (can start immediately).

**Status:** resolved

- [x] A name present in two custom homes appears once in the listing, from the first tracked home.
- [x] The managed link for that name in every installed harness points at the first tracked home.
- [x] Removing the winning home makes the next home's link take over on the next sync.
- [x] A second sync reports no link changes (idempotent).
