# CLI/TUI stack comparison: Ink vs OpenTUI vs Bubble Tea

Date: 2026-08-31. Question: which stack for an agent-skills manager (filesystem ops, list/table UI, big ASCII hero banner), macOS arm64, solo dev from a TypeScript/web background, code mostly AI-written and rarely re-read, possibly open sourced.

Method: primary sources (release notes, npm registry, official docs, GitHub issues), plus a startup benchmark run on this machine. Notes saved here because the repo has no research-notes convention; `.scratch/<feature-slug>/` is the closest existing pattern.

## TL;DR recommendation

**TypeScript + Ink 7 on Node, distributed with `npm i -g`.** Develop and run under Bun for speed, ship compiled JS to npm. Skip the Bun single-binary; it costs 60MB per install, a macOS code-signing tax, and has been the source of two launch-killing regressions in 2026.

Runner-up: Go + Bubble Tea if you later want a single 10MB binary for a non-Node audience. Its API is the most stable of the three and the Go corpus is the largest, but you give up reading your own code (React → Elm-style Go), and Bubble Tea v2 is too new to be well represented in training data.

Avoid OpenTUI for this tool today. It is genuinely good and opencode proves it scales, but it is 0.x with 317 published versions in about a year and it is Bun-first. That is the wrong foundation for code nobody will re-read.

## Comparison table

| Criterion | TS + Ink | TS + OpenTUI | Go + Bubble Tea |
|---|---|---|---|
| Current version | v7.1.1 (Jul 16, 2026) | @opentui/core 0.5.x | v2.0.0 stable (Feb 24, 2026); v1 still maintained |
| Age / track record | 2017, 81 releases, 39.4k stars | first 0.1.x mid-2025 | v1 since 2021, v2 since Feb 2026 |
| Adoption | 6.48M weekly npm downloads; Claude Code, Copilot CLI, Wrangler, Shopify | 809k weekly (@core) + 268k (@react); opencode (201k stars) | "more than 25,000 open-source applications" per Charm |
| Maintainer | sindresorhus (took over from vadimdemedes, May 2024) | Anomaly (the opencode/SST team) | Charmbracelet team |
| API stability | ESM-only since v4; majors are Node/React bumps (v6→v7 was a smooth ride); pinned to React 19.2+ | 0.x, near-daily snapshot releases, internal FFI still moving | v1 never broke; v2 is one planned migration with an official guide "for humans and LLMs" |
| Runtime needs | Node 22+ (Bun works too; Claude Code runs Ink under Bun) | Bun-first; Node support landed Aug 2026 and needs Node 26.4+ | none, static binary |
| Distribution | `npm i -g`, zero signing friction; single binary possible but 60MB + codesign work | bun-compile (opencode precedent: npm wrapper + brew tap + release binaries) | `go install`, goreleaser, Homebrew tap; ~10-20MB binary |
| Cold start (measured here, hello world incl. spawn overhead) | Node ~82ms, Bun ~33ms | Bun ~33ms; compiled binary ~80ms | ~20ms |
| List/table components | ink-ui, ink-table, ink-virtual-list (community); selection is hand-rolled | built-in scrollable/selectable constructs, tree-sitter highlighting | Bubbles v2 list, table, viewport are first-class |
| ASCII banner | trivial (figlet string in `<Text>`) | trivial | trivial (figurine/lipgloss) |
| Known gotchas | full-line redraws get expensive on big or fast-changing output; focus mgmt is basic; alt-screen only arrived in v7 | churn, Bun-only assumptions in docs/examples, young troubleshooting surface | LLMs still emit v1 idioms; v2 import path moved to charm.land |
| Risk for a long-lived, rarely-read tool | low-medium | high | lowest |
| Fit for a React-brained solo dev | best. You can read the diffs | good on the surface, but core is imperative renderables | weakest personally, strongest for the AI |

## Findings per stack

### 1. TypeScript + Ink

State as of Aug 2026: latest is v7.1.1 (2026-07-16). v6.0.0 (2025-05-29) required Node 20 and React 19; v7.0.0 (2026-04-08) requires Node 22 and React 19.2+. v7 added the features people had been asking about for years: alternate screen buffer (`render({alternateScreen})`, issue #263 from 2019), bracketed paste (`usePaste`), `useWindowSize`, `useBoxMetrics`, `useAnimation`, Kitty keyboard protocol detection, and `activeId` on the focus manager.

Maintenance: Vadim Demedes stepped down on 2024-05-21 and transferred Ink, Ink UI, and Pastel to Sindre Sorhus, who has led releases since. Cadence stayed healthy (two majors in 15 months, 30 open issues, 170 contributors). Claude Code was added to Ink's "Who's Using" list in March 2025, and per reverse-engineering writeups Claude Code runs a customized fork with 200+ components on a Bun runtime. Codex CLI's original TS version also used Ink. So the "who ships this in production" answer is the strongest of the three.

Ecosystem: 6.48M weekly downloads. React means you can reach for real React patterns, devtools, and `ink-testing-library`. Component needs for a skills manager are covered: `ink-ui` (tables, spinners, select), `ink-table`, `ink-virtual-list` (Dec 2025) for long lists.

Gotchas that matter for this tool:

- Render pipeline cost. Ink rewrites whole lines as diffed strings. Benchmarks (nathan-cannon/tui-benchmarks) show a one-character change in a 500-message view writing ~84KB per frame where a cell-diff renderer writes 34 bytes, and streaming frame times climbing from ~22ms at 10 messages to ~63ms at 500. assistant-ui needed windowing plus memoization to take a 1000-message thread from 608ms peak frame time to 252ms. For a skills manager (lists of dozens of skills, occasional keystrokes) this is a non-issue if you window long lists. For a chat-streaming UI it would bite.
- ESM-only since Ink 4, and JSX needs a transpile step for Node distribution. Standard fix: `bun run src/cli.tsx` in dev (Bun executes TSX directly), esbuild/tsc for the npm build.
- React coupling is real: v7 pins React 19.2+. You will ride React's upgrade cadence. So far the majors have been boring (Node/React bumps, one backspace-key semantics fix in v7).

### 2. TypeScript + OpenTUI (Anomaly, formerly SST)

What it is: a Zig terminal core with a C ABI, TypeScript bindings, and React/Solid reconcilers. Cell-level diffing in native code, so it does not have Ink's line-rewrite cost, and it does fancy things (tree-sitter syntax highlighting, image protocols, embedded terminal runtime).

Production use: opencode v1.0's TUI runs on it (the repo's own issue label says "now that opencode uses opentui"; npm readme says "powers OpenCode in production today and will also power terminal.shop"). opencode is at 201k stars, so the real-world stress testing is serious. Tobi Lutke has publicly praised the core. The early-2025 launch messaging ("not ready for production") is outdated.

Maturity and churn: @opentui/core is at 0.5.x with 317 published versions since mid-2025, including dated snapshot releases (`0.0.0-20260812-897d859a`) pushed almost daily. Node support only landed in Aug 2026 (relocatable Node runtime assets in v0.4.5, Node ESM import fixes in v0.5.0, "support Node.js 26.4 and later" in the most recent release). Docs at opentui.com are good and unusually agent-friendly (`npx skills add anomalyco/opentui --skill opentui` publishes the docs as a skill), but the API surface (renderables vs constructs) is still moving, and the opencode v1 migration to opentui produced a large collected-feedback issue (#3232).

Risk read: for a tool you will not re-read, a 0.x dependency whose FFI layout and Node support are months old is the risk item. When opencode upgrades OpenTUI, upstream fixes arrive on their schedule, not yours. Revisit at 1.0 with a frozen API promise.

### 3. Go + Bubble Tea (+ Bubbles / Lip Gloss / Huh)

Stability: the strongest story here. Charm says v1 never had a breaking change across five years, and v2.0.0 shipped stable on 2026-02-24 after a year of public betas, with the new renderer ("Cursed Renderer", ncurses-style diffing), Kitty keyboard protocol, OSC 52 clipboard, declarative `tea.View`, and synchronized updates (mode 2026) all enabled by default. Bubbles v2 (list, table, viewport, spinner) is at v2.2.1 (Aug 2026); Huh v2 (forms) shipped March 2026. Crush, Charm's own agent, has run v2 since the betas.

Notably, the v2 release notes open with "If you (or your LLM) are just looking for technical details on migrating from v1, please check out the Upgrade Guide." Charm is explicitly designing for AI-maintained code.

Ecosystem: 25,000+ open-source apps per Charm's v2 post; used at NVIDIA, GitHub, Slack, Microsoft Azure. goreleaser has first-class Homebrew tap and cask publishing (v2.10+). Distribution is the best of the three: `go install` for Go users, a single ~10-20MB static binary for everyone else, and the Go linker already ad-hoc signs darwin/arm64 output so macOS launches it without ceremony.

Caveats for you specifically:

- Training data is v1-heavy. Bubble Tea v2 has existed for six months, so models will sometimes emit v1 API (`tea.EnterAltScreen` commands, string-returning `View()`). The official upgrade guide and updated examples mostly cover this, but expect to correct import paths to `charm.land/bubbletea/v2`.
- The Elm architecture (Model/Update/View) is fine for AI to write but means you lose React literacy as a review tool. Everything is just Go functions, which is at least easy to read top to bottom.
- Startup and runtime performance are the best of the three (measured below).

## Distribution deep-dive: npm global vs `bun build --compile`

**npm global install.** Users need Node 22+ (Ink 7's floor). Package is a few MB of JS plus deps; Claude Code's npm package is ~50MB for comparison. No code-signing or notarization involved: Gatekeeper only blocks executables carrying the `com.apple.quarantine` attribute, which browsers and mail clients set; files npm writes to disk do not carry it (Apple Platform Security, "Gatekeeper and runtime protection"). Updates are `npm update -g`. This is exactly how claude-code and opencode-ai ship, so it is the path of least surprise for your audience.

**`bun build --compile`.** Embeds the runtime into one Mach-O per target and cross-compiles all five targets (including `bun-darwin-arm64`) from any host OS. The costs:

- Size: real-world projects land at 55-70MB per binary (rulesync: 62.8MB darwin-arm64; gg2: ~55-60MB; a Bun/Deno size comparison on HN: 59MB darwin-arm64). Bun 1.4.0 (Aug 2026) made some binaries smaller but they remain an order of magnitude above Go.
- Startup: gg2's compiled binary takes ~80ms just to run `--version` (hyperfine, peterbe.com). You save nothing over plain Node; Bun's runtime boot dominates.
- Signing: Bun ad-hoc signs darwin binaries, and that signer has broken twice in 2026. Bun 1.3.12 (Apr 2026) produced truncated `LC_CODE_SIGNATURE` sections; macOS Sequoia 15.4+ SIGKILLed the binaries at launch outside `/tmp` (issues #29120, #29270, #29306, fixed in 1.3.13). Bun 1.4.0 shipped an invalid final-page hash that SIGKILLs compiled binaries on macOS 27 (issue #39764, fix PR #39837 landed after release). If you distribute by direct download you also need a Developer ID signature plus notarization (Apple developer program, $99/year) per Bun's own codesign guide, with JIT entitlements. Homebrew or curl distribution avoids the quarantine attribute but still needs a valid signature on hardened macOS.

Verdict: for a skills manager whose users all run AI CLIs (and therefore have Node or Bun already), npm wins. Add a compiled binary later only if non-Node users show up, and if you do, version-pin Bun's toolchain and smoke-test the signature on macOS in CI, because opencode's team had to do exactly that after the 1.3.12 regression.

## Startup benchmark (this machine)

macOS arm64 (Apple Silicon), Node 22.22.3, Bun 1.4.0, Go toolchain. Hello-world, 30 runs after 5 warmups, invoked through a shell so every number includes ~10-15ms of process-spawn overhead. Treat the deltas, not absolutes, as signal:

| Runtime | Time per run |
|---|---|
| Go static binary | ~20ms |
| Bun 1.4.0 (TS) | ~33ms |
| Node 22 (ESM) | ~82ms |

Consistent with published numbers (Node 50-80ms vs Bun 5-15ms excluding spawn overhead; compiled Bun binary ~80ms for `--version`). Practical read: a Node-delivered Ink app costs ~100ms to first paint, which is fine for a tool humans open a few times a day. If startup ever bothers you, running the same Ink code under Bun gets you back 50ms for free.

## Why Ink fits this project, specifically

1. You can still read the code. Even AI-maintained code gets skimmed at 2am when a release breaks. JSX that mirrors a component tree is the fastest thing for a web-brained human to audit, and `ink-testing-library` gives you snapshot-style tests an agent can generate and you can eyeball.
2. The training prior is enormous. React plus five years of Ink usage plus Claude Code's own ecosystem means models write correct Ink without much steering. That is your main maintenance budget.
3. The workload is easy mode for Ink. A skills manager is low-frequency redraws and modest list sizes, precisely the region where the visulima analysis says Ink is fine. Window or virtualize anything that can exceed a few hundred rows and the known perf ceiling never appears.
4. Distribution matches the audience and needs no Apple developer account, no notarization pipeline, and no Bun version pinning.
5. The failure modes are shallow. Ink's majors so far are boring (Node floor bumps, React version bumps). Pin `ink@7` / `react@19.2`, and upgrades are annual chores rather than rewrites.

The honest case for Go + Bubble Tea: if this tool gets popular enough that Homebrew install and a 10MB binary matter, or if you want the option of reading the whole program someday, Bubble Tea v2 plus Bubbles covers list/table/forms better out of the box than anything in the Ink world. The cost is your own fluency, and six months of thin v2 training data. I would not switch now; the switch is cheap to make never, because a skills manager is a small surface.

The honest case against OpenTUI today: nothing about its tech is the problem. The problem is betting a rarely-read codebase on a dependency that ships daily snapshots, has no stable-API promise, and treats Node as a secondary target. Check again at 1.0.

## Citations

1. Ink releases and npm registry: v6.0.0 (2025-05-29, Node 20/React 19), v7.0.0 (2026-04-08, Node 22/React 19.2+), v7.1.1 latest (2026-07-16), 6.48M weekly downloads. https://github.com/vadimdemedes/ink/releases and https://registry.npmjs.org/ink
2. Vadim Demedes, "Moving on from Ink" (2024-05-21): handover to Sindre Sorhus. https://vadimdemedes.com/posts/moving-on-from-ink
3. Charm, "v2" blog (2026-02-23) and Bubble Tea v2.0.0 release (2026-02-24): Cursed Renderer, 25,000 apps, "never pushed a breaking change", upgrade guide "for humans and LLMs". https://charm.land/blog/v2 and https://github.com/charmbracelet/bubbletea/releases/tag/v2.0.0
4. @opentui/core on npm (0.5.x, 317 versions, 809k weekly, "powers OpenCode in production"), opentui.com docs, and anomalyco/opentui releases showing Aug 2026 Node support PRs. https://www.npmjs.com/package/@opentui/core and https://opentui.com
5. Bun single-file executables (cross-compile targets), official macOS codesign guide, and the 1.3.12 / 1.4.0 signature regressions (issues #29120, #29270, #39764). https://bun.com/docs/bundler/executables, https://bun.com/guides/runtime/codesign-macos-executable, https://github.com/oven-sh/bun/issues/29120, https://github.com/oven-sh/bun/issues/39764

Supporting: nathan-cannon/tui-benchmarks (Ink per-frame byte and latency scaling), assistant-ui PR #3966 (Ink long-list windowing benchmarks), peterbe.com Bun binary benchmark (~80ms startup, sizes), Apple Platform Security "Gatekeeper and runtime protection" (https://support.apple.com/guide/security/sec5599b66df/web), goreleaser v2.10 Homebrew tap/cask support.
