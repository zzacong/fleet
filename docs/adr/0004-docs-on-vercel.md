# ADR 0004: Docs site hosted on Vercel, Git-connected, root www

Date: 2026-09-07
Status: Accepted

## Context

`www/` is an Astro 7 + Starlight static site (`site: https://fleet.zzacong.com`, build `astro build`, output `dist/`). It is a pnpm workspace package (`pnpm-workspace.yaml` packages `www`, lockfile at repo root). One build-time read reaches outside `www/`: `src/components/SkillsCatalog.astro:27-32` resolves `../skills` (repo-root `skills/`) and degrades to an empty state when missing.

## Decision

Host the Docs site on Vercel as a new project (`fleet`, fallback `fleet-docs`) under the personal scope, Git-connected to this repo. Ship to `*.vercel.app` first; attach `fleet.zzacong.com` after DNS is confirmed. Framework preset Astro, package manager pnpm, Node >=22.18 per root `engines`. Build wiring is pinned in root `vercel.json` (Root Directory stays repo root): install `pnpm install`, build `pnpm --filter www build`, output `www/dist`. Verified the filter build passes from repo root.

## Consequences

- Vercel clones the full repo, so the `../skills` read keeps working with Root Directory `www`. A sparse/single-dir checkout would render an empty skills catalog instead of failing the build.
- Root `pnpm-lock.yaml` stays the single lockfile; no copy in `www/`. Root `vercel.json` keeps Root Directory at repo root so the lockfile is always visible; `pnpm --filter www` still runs the build with cwd `www/`, so the `../skills` read is unaffected.
- **CLI-only pushes skip the build.** Root `vercel.json` sets `ignoreCommand: git diff --quiet HEAD^ HEAD -- www/ skills/ vercel.json pnpm-lock.yaml pnpm-workspace.yaml` (exit 0 = no docs paths touched = skip; exit 1 = build). `skills/` is included because `SkillsCatalog.astro` reads repo-root `skills/` at build time; lock/workspace files are included because they shape the www install. Go CLI (`cmd/`, `internal/`), `npm/`, and root docs never trigger a site build.

## Alternatives considered

- One-shot CLI `vercel deploy --prebuilt` from `www/dist/`. Rejected: instant but manual, no auto-deploys.
- Separate docs repo. Rejected: per ADR 0003, no split without measured pain; would also break the `../skills` build read.
