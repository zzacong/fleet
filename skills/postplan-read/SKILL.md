---
name: postplan-read
description: Fetch and read a document from a postplan.dev URL. Use when the user supplies a postplan.dev URL to read, implement, or act on. For publishing documents to Postplan, use the postplan skill instead.
---

# Postplan (read)

When a user supplies a `postplan.dev` URL, fetch the uploaded HTML immediately with the shell. Do not use web search or a browser to retrieve it.

1. Remove a trailing slash, then append `/raw` unless the URL already ends in `/raw`.
2. Run `curl --fail --silent --show-error --location --max-time 30 --output /tmp/postplan.html '<raw-url>'`.
3. Read `/tmp/postplan.html` as the user's artifact and continue the requested task.

A web-search refusal is not evidence that Postplan rejected the request. If `curl` fails, report its actual status or network error; do not substitute search results.

## Viewer Behavior

Every Postplan URL serves the exact uploaded HTML, byte for byte, to every client — browsers, curl, and agent fetch tools alike. There is no wrapper page, sandbox, or consent step: fetching a Postplan URL always yields the draft content itself. The `/raw` suffix is an alias that returns the same bytes.
