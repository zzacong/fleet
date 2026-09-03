# Skills collection

Versioned skills collection — what skills.sh publishes.

Each immediate child directory containing a `SKILL.md` file is a custom skill.
`skills/<name>/SKILL.md` frontmatter `name` falls back to the directory name.

Fleet code alongside this directory is ignored by the publisher.

Shipped skills:

- `choose-flow` — estimate work size, recommend next flow.
- `create-plan` — numbered implementation plan in `.plans/`.
- `postplan` — publish a doc as a Postplan draft.
- `postplan-read` — fetch and read a `postplan.dev` URL.
- `ticket-sweep` — implement all open local tickets in dependency order.
- `worktree-finish` — squash-merge a worktree branch into main, clean up.
- `worktree-session` — create a worktree and move the session there.

See `CONTEXT.md` and ADR 0001 for the skills-repo and fleet-home model.
