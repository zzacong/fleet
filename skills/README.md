# Skills collection

Versioned skills collection — what skills.sh publishes.

Each immediate child directory containing a `SKILL.md` file is a custom skill.
`skills/<name>/SKILL.md` frontmatter `name` falls back to the directory name.

Fleet code alongside this directory is ignored by the publisher. This
placeholder exists so `skillsRepo/skills` resolution has a target right after
the monorepo move. No skills are shipped yet.

See `CONTEXT.md` and ADR 0001 for the skills-repo and fleet-home model.
