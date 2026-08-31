// cli.ts — skillctl dummy CLI (Commander). No filesystem work; fixture data only.
// The TUI is only ever launched by `skillctl tui`.
import { Command } from "commander";
import {
  expandRows,
  formatRowParts,
  loadFixture,
  type Root,
  type Skill,
} from "./store";

const fixture = loadFixture();

function fail(message: string): never {
  console.error(`error: ${message}`);
  process.exit(1);
}

function resolveSkill(name: string): Skill {
  const skill = fixture.skills.find((s) => s.name === name);
  if (!skill) fail(`unknown skill "${name}"`);
  return skill;
}

function nameWidth(skills: Skill[]): number {
  return Math.min(30, Math.max(...skills.map((s) => s.name.length)));
}

function printTable(skills: Skill[], staged: ReadonlySet<string> = new Set()): void {
  const width = process.stdout.columns ?? 100;
  const nw = nameWidth(skills);
  const rootsWidth = Math.max(
    ...skills.map((s) => Math.min(s.roots.join(", ").length, 24)),
  );
  // One global roots width keeps every column aligned across rows.
  const descWidth = Math.max(24, width - nw - 3);
  for (const skill of skills) {
    const p = formatRowParts(skill, staged.has(skill.name), nw, descWidth, rootsWidth);
    const dim = (s: string) => `\x1b[2m${s}\x1b[0m`;
    const line = [
      skill.enabled ? `\x1b[32m●\x1b[0m` : dim("○"),
      p.stagedMark.trim() ? `\x1b[33m*\x1b[0m` : " ",
      p.name,
      skill.enabled ? p.description : dim(p.description),
      dim(p.roots),
    ].join(" ");
    console.log(line.trimEnd());
  }
}

const program = new Command();
program
  .name("skillctl")
  .description("prototype skill activation manager (no real filesystem work)")
  .version("0.0.1");

program
  .command("list")
  .description("print the fixture as a table (same glyphs as the TUI)")
  .option("--json", "print the fixture as JSON")
  .option("--rows <n>", "stress fixture: expand to N rows", "45")
  .action((opts: { json?: boolean; rows: string }) => {
    if (opts.json) {
      console.log(JSON.stringify(fixture, null, 2));
      return;
    }
    const n = Number.parseInt(opts.rows, 10);
    if (Number.isNaN(n) || n < 1) fail(`invalid --rows value "${opts.rows}"`);
    printTable(expandRows(fixture.skills, n));
  });

type Action = "enable" | "disable" | "toggle";

function stagedAction(action: Action, names: string[]): void {
  for (const name of names) {
    const skill = resolveSkill(name);
    const target = action === "toggle" ? (skill.enabled ? "disable" : "enable") : action;
    console.log(`would ${target} ${skill.name}`);
  }
}

program
  .command("enable")
  .description("stage enabling of one or more skills")
  .argument("<skills...>")
  .action((skills: string[]) => stagedAction("enable", skills));

program
  .command("disable")
  .description("stage disabling of one or more skills")
  .argument("<skills...>")
  .action((skills: string[]) => stagedAction("disable", skills));

program
  .command("toggle")
  .description("print the resolved action per skill based on fixture state")
  .argument("<skills...>")
  .action((skills: string[]) => stagedAction("toggle", skills));

program
  .command("root")
  .description("manage skill roots")
  .command("list")
  .description("print the fixture's root names and paths")
  .action(() => {
    for (const root of fixture.roots) {
      console.log(`${root.name.padEnd(12)} ${root.path}`);
    }
  });

program
  .command("doctor")
  .description("print canned findings derived from the fixture")
  .action(() => {
    const multiRoot = fixture.skills.filter((s) => s.roots.length > 1);
    const disabled = fixture.skills.filter((s) => !s.enabled);
    const usedRoots = new Set(fixture.skills.flatMap((s) => s.roots));
    const emptyRoots = fixture.roots.filter((r: Root) => !usedRoots.has(r.name));
    console.log(`doctor: ${fixture.skills.length} skills across ${fixture.roots.length} roots`);
    console.log(`  - ${multiRoot.length} skills are defined in more than one root (e.g. ${multiRoot[0]?.name})`);
    console.log(`  - ${disabled.length} skills are disabled`);
    console.log(
      emptyRoots.length > 0
        ? `  - ${emptyRoots.length} root(s) contain no skills: ${emptyRoots.map((r) => r.name).join(", ")}`
        : "  - every root contains at least one skill",
    );
  });

program
  .command("tui")
  .description("open the interactive manager")
  .option("--rows <n>", "stress fixture: expand to N rows", "45")
  .option("--bench", "print startup→first frame ms to stderr, then exit")
  .action(async (opts: { rows: string; bench?: boolean }) => {
    const n = Number.parseInt(opts.rows, 10);
    if (Number.isNaN(n) || n < 1) fail(`invalid --rows value "${opts.rows}"`);
    const { runTui } = await import("./main");
    await runTui({ rows: n, bench: opts.bench ?? false });
  });

// No args prints help (Commander would exit 1; the spec wants plain help).
if (process.argv.length <= 2) {
  program.help();
}

program.parseAsync();
