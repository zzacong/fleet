---
name: rebase-pr
description: Rebase an open pull request onto main, push it, then monitor its checks. Use when a PR is behind main before merge.
argument-hint: "[PR number or URL]"
---

# Rebase PR

Rebase an open PR onto `main`, then invoke `babysit-pr`. If no PR is
specified, use the current branch's PR.

## 1. Identify the PR and check the checkout

Use `gh pr view` to confirm the PR is open and targets `main`; record its
URL, head branch, base and head repositories, and head SHA. If it targets
another branch, stop and confirm.

Require a clean working tree. If needed, check out the PR head with
`gh pr checkout <PR>`. Verify the branch and use `git remote -v` to identify
remotes for the base and head repositories. Stop if either remote is
ambiguous or the head repository is not writable.

## 2. Check whether the PR is behind

Fetch the latest `main` from the PR's base repository, then compare it with
the local PR head:

```sh
git fetch "$BASE_REMOTE"
git rev-list --left-right --count "HEAD...$BASE_REMOTE/main"
```

If the right-hand count is zero, skip the rebase and push.

## 3. Rebase and push

If the PR is behind, rebase onto the fetched `main`:

```sh
git rebase "$BASE_REMOTE/main"
```

Resolve conflicts only when the PR's intent is clear. Otherwise abort and
report without pushing.

Run the repository's documented local check command if one exists. Report
failures and continue; don't change code to fix them.

If you rebased, stop if the PR head SHA changed since step 1. Otherwise push
to the verified head remote with the recorded SHA as the lease:

```sh
git push --force-with-lease="refs/heads/$HEAD_BRANCH:$HEAD_SHA" \
  "$HEAD_REMOTE" "HEAD:refs/heads/$HEAD_BRANCH"
```

If the lease rejects the push, stop and report that the remote changed.

## 4. Babysit the PR

Invoke `babysit-pr` with the PR URL and follow it to completion.
