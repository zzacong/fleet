---
name: ticket-sweep
description: Implement all local open tickets in dependency order.
disable-model-invocation: true
---

# Ticket sweep

Implement every sweepable ticket in a local feature series, not one. You are
the orchestrator, not the implementer: read the local ticket graph, work the
frontier, dispatch frontier tickets in parallel to sub-agents in separate Git
worktrees, and squash-merge successful branches serially, one commit per
ticket. Repeat until every ticket is resolved or the sweep stops with a
reported failure.

Use the local Markdown tracker only. Tickets live under
`.scratch/<feature-slug>/issues/`. A parent spec at
`.scratch/<feature-slug>/spec.md` is optional context. Ticket files are the
implementation and review requirements.

## Process

### 1. Load and validate the tickets

The user must provide `<feature-slug>` or the full `.scratch/<feature-slug>`
path. Ask for it before reading tickets if it was not provided.

Read the parent spec when it exists and every `issues/*.md` file in full,
including comments. If the issue directory is missing or contains no tickets,
stop and tell the user to run `/to-tickets`.

Validate the ticket set before creating worktrees:

- Every ticket has a unique numeric `NN`, a `What to build` description,
  acceptance criteria, a `Blocked by:` line, and a `Status:` line.
- Every blocker reference resolves to exactly one ticket number. Treat
  `None (can start immediately)` as no blockers.
- `ready-for-agent` is dispatchable and `resolved` is terminal. Any other
  status is held and stops the sweep. Do not dispatch or complete held tickets.

If all tickets are already `resolved`, report that there is no work and stop
before preparing the branch or running review. Report malformed tickets and
stop before creating worktrees.

### 2. Build the dependency graph

Resolve every `Blocked by:` entry to a ticket number. A ticket is in the
frontier when its status is `ready-for-agent` and every listed blocker has
status `resolved`.

Reject duplicate ticket numbers, missing blockers, self-blocks, and cycles. If
unresolved tickets remain but the frontier is empty, report each ticket and its
blocking status and stop. Never guess an ordering.

### 3. Prepare the current branch

Integrate onto the current branch. Require a non-detached `HEAD` and an empty
`git status --short`. If either check fails, report the paths or detached state
and stop. Do not stash, reset, revert, or overwrite user changes.

Ensure `.worktrees/` is ignored by `.gitignore`. If the rule is missing, add
exactly `.worktrees/` to `.gitignore`, commit that repository setup change, and
confirm the branch is clean. Leave `.scratch/` tracked and visible to Git.

Record `BASE_SHA=$(git rev-parse HEAD)` after preparation. Continue only with a
clean branch and a recorded fixed point.

### 4. Work the frontier in waves

A wave is the complete frontier at one branch tip. Dispatch the whole wave
before integrating any result, wait for every sub-agent in that wave, then
compute the next frontier.

While the frontier is non-empty:

1. Set `WAVE_BASE=$(git rev-parse HEAD)` once, before creating any worktrees.
   For each frontier ticket, create a branch and worktree from that same SHA:

   ```sh
   git worktree add -b ticket/<NN>-<slug> .worktrees/<NN>-<slug> "$WAVE_BASE"
   ```

2. Delegate each frontier ticket to a `general` sub-agent. Include its exact
   ticket path, worktree path, and `WAVE_BASE` in the prompt. Tell it:

   > Work only in `.worktrees/<NN>-<slug>`. Read the complete assigned ticket,
   > including comments and acceptance criteria, before changing code. Treat
   > it as the implementation contract. Read the repository's agent
   > instructions, `CONTEXT.md` when present, and relevant ADRs. Run
   > `/implement` for this ticket and use `/tdd` where possible. Run focused
   > tests and typechecking during implementation, then the full test suite
   > once at the end. Implement only this ticket. Mark every satisfied
   > acceptance criterion checked and set only this ticket's `Status:` to
   > `resolved` after its behavior and tests pass. Commit the implementation,
   > tests, checked criteria, and status change on this branch. Do not merge,
   > push, or modify another ticket. Return the branch, commit SHA, changed
   > files, and test commands and results. If the work is incomplete or tests
   > are red, leave the ticket unresolved and report the blocker.

3. Wait for every sub-agent in the wave to return. Validate each successful
   branch before merging. Require a commit after `WAVE_BASE`, a clean worktree,
   the assigned ticket marked `resolved`, every acceptance criterion checked,
   and passing test results. Keep any failed or invalid branch and worktree for
   inspection. If any branch fails validation, integrate only independent
   validated successes, then stop. Do not start another wave.

4. Integrate validated branches serially on the current branch:

   ```sh
   git merge --squash ticket/<NN>-<slug>
   git commit -m "<ticket title> (closes <NN>)"
   git worktree remove .worktrees/<NN>-<slug>
   git branch -D ticket/<NN>-<slug>
   ```

   Resolve mechanical conflicts against the moving branch tip. Stop and
   report a semantic conflict rather than guessing. Remove the worktree and
   branch only after the squash merge succeeds.

5. After the complete wave is integrated, reread ticket statuses from the
   current branch and recompute the frontier. A resolved status counts only
   after it is present on that branch. There is no separate close step.

### 5. Review and verify the result

Run this phase only when every ticket is `resolved` and every ticket worktree
has been removed. Run one final `/code-review` on the diff from `BASE_SHA` to
the current branch. Provide every ticket file as the requirements source and
the parent spec when it exists. When no spec exists, tell the review that the
tickets are authoritative.

Fix trivial findings on the current branch and commit each fix separately.
Handle non-trivial findings in a dedicated branch and worktree, squash-merge
each fix separately, then remove its worktree and branch. Do not fold review
fixes into ticket commits. Stop and report semantic conflicts rather than
guessing.

Run the full test suite on the current branch after review, whether or not
review fixes were needed. If it fails, fix the cause in a separate commit or
worktree, clean up that worktree and branch after merging, and rerun until the
suite is green. Do not mark the feature complete while review findings or
tests remain unresolved.

After review and tests pass, if `.scratch/<feature-slug>/spec.md` exists, set
or add `Status: resolved` and commit that metadata change separately.

### 6. Update user-facing documentation

Review `README*` and any user-facing docs affected by the completed tickets.
Update setup, commands, usage, or behavior when they changed. Planning records
such as `CONTEXT.md` and ADRs are out of scope unless a ticket explicitly
requires them. If the repository has a format script, run it after updating
the docs. Commit documentation changes separately. If no update is needed,
report that the user-facing docs were reviewed.

### 7. Finish

Confirm every ticket is `resolved`, the spec is `resolved` when present, the
current branch is clean, and no worktree or branch created by this sweep
remains. Run `git worktree prune` after removing those worktrees.

Report one line per ticket with its commit, wave, and verification result.
Report the separate setup, spec-status, review-fix, and documentation commits.
If no documentation commit was needed, report that the relevant documentation
was reviewed and already accurate.
If the sweep stopped, report the unresolved tickets, failure reasons, and
retained worktrees or branches. Never report completion while any ticket or
required verification is unresolved.
