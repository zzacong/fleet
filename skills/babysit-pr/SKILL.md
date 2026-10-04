---
name: babysit-pr
description: Watch a pull request's checks until they finish, then diagnose failures. Use after creating, updating, or pushing a PR.
argument-hint: "[PR number or URL]"
---

# Babysit PR

Monitor the specified PR, or the current branch's PR if none is given. Do not
rerun jobs or change the PR.

## 1. Identify the PR head

Use `gh pr view` to confirm the PR is open and record its URL, base repository
(`BASE_REPO`), head branch, and head SHA. Ask which PR to monitor if it's
ambiguous.

## 2. Wait for checks and Actions

Watch the PR checks until they reach a terminal state:

```sh
gh pr checks "$PR" --repo "$BASE_REPO" --watch --interval 10
```

Then list workflow runs for the same head branch and SHA:

```sh
gh run list --repo "$BASE_REPO" --branch "$HEAD_BRANCH" \
  --commit "$HEAD_SHA" --limit 100 \
  --json databaseId,name,status,conclusion,url,headSha
```

Use `gh run watch <run-id> --repo "$BASE_REPO"` for each unfinished run.
Refresh the list until all runs for that SHA finish. If the PR head changes,
repeat for its new SHA. If there are no checks or runs, report that.

Report failures, cancellations, skipped runs, and pending checks as
non-success. A nonzero watch result does not end the investigation.

## 3. Diagnose failures

Inspect failed jobs and logs:

```sh
gh run view <run-id> --repo "$BASE_REPO" --log-failed
```

Report the PR URL, head SHA, and final status of each check. For failures,
include the workflow/job, relevant log evidence, likely cause, possible fix,
and next step. Mark inference as such. If logs are inconclusive, say what
needs inspection.
