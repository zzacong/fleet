---
name: worktree-session
description: Create a Git worktree for the requested task and move the current OpenCode V2 session there before implementation begins.
argument-hint: "Task slug or short description"
---

# Worktree session

Use this skill as the first step for work that should happen on a new branch. It creates the worktree, moves this session to it, and only then starts the requested task.

## Process

### 1. Establish the task and repository

1. Read the invocation argument. If it does not provide a clear task or short slug, ask the user for one before running commands.
2. Normalize the task into a lowercase ASCII slug containing only letters, numbers, and hyphens. Use the slug for the worktree directory.
3. Read any applicable `AGENTS.md` or `CLAUDE.md` instructions before choosing the branch name. Follow any documented branch naming rule. Otherwise use `opencode/<slug>`.
4. Resolve the current repository and require a clean source worktree:

   ```sh
   repo_root="$(git rev-parse --show-toplevel)"
   git status --short
   ```

   If `git status --short` prints anything, stop and ask the user to commit, stash, or otherwise account for those changes. `git worktree add` starts from `HEAD`; it does not copy uncommitted changes.
5. Confirm that `opencode2` is available and that the current session can be read through the V2 API. Use the current conversation session ID supplied in the session context, not a newly created session or an arbitrary session from the list:

   ```sh
   command -v opencode2
   command -v jq
   session_id="<current session ID>"
   opencode2 api v2.session.get --param "sessionID=$session_id"
   ```

   Stop before changing Git state if any check fails.

### 2. Choose the worktree location

Always use the shared `~/Developer/worktrees/<repository>/<slug>` layout. This keeps new worktrees outside the current checkout, including when the current checkout is already a linked worktree.

```sh
main_root="$(git worktree list --porcelain | sed -n '1s/^worktree //p')"
repo_name="$(basename "$main_root")"
worktree_root="$HOME/Developer/worktrees/$repo_name"
mkdir -p "$worktree_root"

slug="<normalized slug>"
destination="$worktree_root/$slug"
branch="<branch name selected under repository rules>"
```

Check both the destination path and branch name before creating anything:

```sh
if [ -e "$destination" ] || [ -L "$destination" ]; then
  printf 'Worktree path already exists: %s\n' "$destination" >&2
  exit 1
fi

if git show-ref --verify --quiet "refs/heads/$branch"; then
  printf 'Branch already exists: %s\n' "$branch" >&2
  exit 1
fi
```

If either exists, stop and ask for a different slug or branch. Do not reuse an existing worktree for a new task.

### 3. Create and move

Create the branch and worktree from the current `HEAD`:

```sh
git worktree add -b "$branch" "$destination" HEAD
```

After that command succeeds, move the current session to the new directory through the OpenCode V2 session API. The destination is another checkout of the same Git project, so the move transfers the session location without creating a second session:

```sh
payload="$(jq -n --arg directory "$destination" '{directory: $directory}')"
opencode2 api v2.session.move \
  --param "sessionID=$session_id" \
  --data "$payload"
```

If the API call fails, leave the new worktree in place, report its path and the error, and stop. Do not begin the task in the old worktree.

### 4. Verify before implementation

Require all of these checks to pass:

```sh
test "$(git -C "$destination" rev-parse --show-toplevel)" = "$destination"
test "$(git -C "$destination" branch --show-current)" = "$branch"
test -z "$(git -C "$destination" status --short)"

opencode2 api v2.session.get --param "sessionID=$session_id" \
  | jq -e --arg directory "$destination" '.data.location.directory == $directory' >/dev/null
```

The session move is complete only when the API reports the destination and the new checkout reports the expected branch and clean status. Use the destination as the working directory for every later command. If the host still reports the old working directory, stop and resolve that session-context problem before editing files.

Once these checks pass, continue with the requested task in the new worktree. Report the branch and absolute worktree path when the setup is complete.
