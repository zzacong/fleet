---
name: postplan
description: Publish a plan, proposal, brief, architecture note, or similar document as a static HTML draft on Postplan, or update a draft published earlier. Use when the user asks to publish or upload to Postplan. To read a postplan.dev URL, use the postplan-read skill instead.
---

# Postplan

Publish one complete static HTML document as a Postplan draft and hand back the URL.

## Workflow

1. Copy `template.html` (next to this file) verbatim. It is the frozen skeleton: full CSS, header, main, footer. Do not restyle, resize, or restructure anything outside `<main>`.
2. Write only content inside `<main>`:
   - Replace the placeholder `<header>`: an `h1` document title plus a one-line subtitle.
   - Write the body sections. Tables, `pre`/`code`, lists, and the `note` / `warnbox` callout classes inherit the frozen styles. Link the first mention of any tool, project, or document that has a public HTTPS page (`<a href="…">pi's docs/packages.md</a>`), instead of leaving it as plain text.
   - If the document draws on sources, end `<main>` with an `<h2>Sources</h2>` section. Each source is a link when it has a public HTTPS page — descriptive link text plus what it contributed:

     ```html
     <li>
       <a href="https://…">pi's docs/packages.md</a> — tool schema cost figures
     </li>
     ```

     Fall back to a plain descriptive name only when no public page exists. Never raw local filesystem paths, secrets, or private URLs. Omit the section if there are no sources.

   - In the `<footer>`, keep the "Drafted with Postplan" stamp and fill in what the document was generated for. You may add one short contextual phrase. No date.
3. Save to `/tmp/postplan-<slug>.html`, where slug is the kebab-case document title trimmed to 40 characters. The path is deterministic: same title, same path.
4. Upload:

   ```sh
   pnpm dlx postplan upload /tmp/postplan-<slug>.html
   ```

5. Return the Postplan URL to the user, then delete the tmp file.

## Revisions

The CLI maps absolute file paths to draft IDs in `~/.postplan/drafts.json`, and the mapping survives file deletion. Re-uploading from the same path updates the existing draft (the CLI prints "Updated draft"). So:

- To revise a document, regenerate it to the same slug path and upload again — it updates in place.
- Never pass `--new` unless the user explicitly asks for a fresh draft.

## Working Files

Never write the HTML into the current workspace or project directory. The only local artifact is the slug-named tmp file, deleted immediately after upload.

## CSS Extensions

The template's CSS is the only sanctioned styling for the document itself. When the content genuinely cannot be expressed with the existing primitives (an unusual layout: timeline, comparison columns, step figure), you may add CSS under all of these constraints:

- In a separate `<style>` block placed after the frozen one. The frozen block is never edited.
- Additive only: never override the frozen rules or restyle `body`, `h1`–`h3`, `p`, `table`, `code`, `.note`, or `.warnbox`, and never change the page's colors, fonts, or measure.
- Build from the template's CSS variables (`var(--accent)`, `var(--line)`, `var(--code-bg)`, …) so new constructs match both light and dark schemes automatically.
- Scoped to new class names; no element-wide selectors beyond the frozen set.

Styling the document is frozen; laying out an unusual content shape is permitted under these rules. The default measure is 46rem — you may break out beyond it per-element (e.g. via `.wide` for tables, charts, or side-by-side comparisons) when you deem it necessary.

> `.wide` centering — the frozen rule is `width: min(92vw, 70rem); position: relative; left: 50%; transform: translateX(-50%)`. It uses `left` (not `margin-left`) so it composes safely with other classes that set `margin` (e.g. `.chart-card { margin: 0 0 16px }` would otherwise clobber a `margin-left: 50%` breakout and shift the element left by ~344 px). Never override `left`, `transform`, `position`, or `width` on a `.wide` element, and prefer `margin-block` for vertical spacing if you need custom margins.

## Document Rules

Allowed:

- Semantic HTML.
- A `<style>` block per the CSS Extensions rules above.
- Normal document metadata such as charset, viewport, and title.
- Links to ordinary HTTPS pages.
- Images from HTTPS or data URLs when necessary.

Do not include:

- JavaScript.
- `<script>` tags.
- Inline event handlers such as `onclick`, `onload`, or `onerror`.
- `javascript:` URLs.
- Forms.
- Iframes, embeds, objects, or applets.
- Meta refresh redirects.
- Secrets, tokens, private URLs, or local filesystem paths.

## CLI Notes

Postplan stores CLI auth and draft mappings in `~/.postplan`. The CLI prints both a draft URL and a `Raw HTML` URL. Either works for any client; hand the `Raw HTML` URL to another agent when you want the most explicit form.
