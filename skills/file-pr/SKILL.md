---
name: file-pr
description: File a concise pull request. Use when the user asks to file, open, or create a PR.
---

# File PR

1. Prep the branch

Rebase onto latest `main`, then review the diff against `origin/main` to confirm it matches the goal.

2. Check for an existing PR

Check for an existing PR first. If one exists, update it.

3. Write the title

Match repo convention from recent merges. State why the change matters in plain words.

Bad:
> perf(server): negotiate permessage-deflate on the websocket

Good:
> perf(server): cut websocket frame size by 70%+ with gzipping

4. Write the description

Start with a clear, simple explanation of the problem drawn from the user’s original prompt, followed by a brief overview of the solution. Don’t begin by listing implementation details.

Bad:
> Removed implicit workspace carry-over from all "new thread" entry points. Deleted buildContextualThreadOptions, startNewThreadInProjectFromContext, and the sidebar seed-context machinery.

Good:
> My "new worktree" default was ignored when starting new threads on existing worktrees. Super unintuitive. Now your preferences always apply.

End with:

> Built with [model] via [harness].

5. Open the PR

Open ready for review so bots run. Use draft only if the user explicitly asks.
