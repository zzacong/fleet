---
name: worktree-finish
description: Squash merge a worktree branch into main, then remove
  the worktree and the branch.
disable-model-invocation: true
argument-hint: "[slug or branch name, defaults to the current worktree]"
---

# Worktree finish

Counterpart to `worktree-session`. Squash merge the worktree branch into
main, move this session back to the main checkout, then delete the
worktree and branch. Never push.

## 1. Identify the worktree and branch

```sh
main_root="$(git worktree list --porcelain | sed -n '1s/^worktree //p')"
current_root="$(git rev-parse --show-toplevel)"
```

If the session runs in a linked worktree, that worktree is the target.
Otherwise resolve the invocation argument (a slug or branch name; take
its last path segment) against the layout `worktree-session` created:
`$HOME/Developer/worktrees/$(basename "$main_root")/<slug>`. With no
argument, list the worktrees under that directory and ask. Stop if the
path is missing or absent from `git worktree list`.

```sh
destination="<resolved path>"
branch="$(git -C "$destination" branch --show-current)"
main_branch="$(git symbolic-ref --short refs/remotes/origin/HEAD \
  2>/dev/null | sed 's|^origin/||')"
[ -n "$main_branch" ] || main_branch=main
git show-ref --verify --quiet "refs/heads/$main_branch" || main_branch=master
```

## 2. Preflight

Require all of these before changing anything:

```sh
test -z "$(git -C "$destination" status --short)"
test -z "$(git -C "$main_root" status --short)"
test "$(git -C "$main_root" branch --show-current)" = "$main_branch"
test "$branch" != "$main_branch"
git -C "$main_root" log --oneline "$main_branch..$branch"
# log must print at least one commit
```

On a failed check, stop and let the user fix it (commit, stash, check
out the right branch). If the log prints nothing there is nothing to
merge; ask whether to just remove the worktree and branch.

## 3. Rebase onto main

Rebase the branch first so the squash merge lands on a current main:

```sh
git -C "$destination" fetch origin 2>/dev/null
target="origin/$main_branch"
git show-ref --verify --quiet "refs/remotes/$target" || target="$main_branch"
git -C "$destination" rebase "$target"
```

Resolve conflicts yourself, preferring the branch's intent, then
`git rebase --continue`. If a conflict cannot be resolved confidently,
`git rebase --abort` and report.

Bring main to the same tip:

```sh
git -C "$main_root" merge --ff-only "$target"
```

If that fails, stop and report; do not merge onto a diverged main.

## 4. Squash merge

```sh
git -C "$main_root" merge --squash "$branch"
git -C "$main_root" commit -m "<subject>" -m "<body>"
```

Write the message from the branch's own commits:

```sh
git log --reverse --format='%s' "$main_branch..$branch"
```

Use a semantic subject, `<type>(<scope>): <imperative description>` per
AGENTS.md, with the individual commits as body bullets.

## 5. Move session, remove, verify

When `opencode2` and `jq` exist, move the session home before the
worktree disappears (the reverse of `worktree-session`'s move). If the
move fails, continue but say plainly that the session still points at
the removed path.

```sh
session_id="<current session ID>"
payload="$(jq -n --arg directory "$main_root" '{directory: $directory}')"
opencode2 api v2.session.move \
  --param "sessionID=$session_id" --data "$payload"

git -C "$main_root" worktree remove "$destination"
# -d refuses: squash merges never count as merged
git -C "$main_root" branch -D "$branch"
```

Verify the worktree path is gone, the branch is gone, and main is
clean. Report the squash commit (`git log -1 --oneline`) and that
cleanup is done.
