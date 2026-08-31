#!/usr/bin/env node
import { Command } from "commander";
import { loadFixture, expandRows, type SkillRow } from "./store.ts";

const fixture = loadFixture();

function findSkillOrDie(name: string): SkillRow {
  const row = fixture.skills.find((s) => s.name === name);
  if (!row) {
    console.error(`skillctl: unknown skill '${name}'`);
    process.exit(1);
  }
  return row;
}

function resolveAllOrDie(names: string[]): SkillRow[] {
  return names.map((n) => findSkillOrDie(n));
}

function parseRows(value: string): number {
  const n = Number(value);
  if (!Number.isInteger(n) || n < 1) {
    console.error(`skillctl: --rows expects a positive integer, got '${value}'`);
    process.exit(1);
  }
  return n;
}

function printTable(rows: SkillRow[]) {
  for (const row of rows) {
    const glyph = row.enabled ? "●" : "○";
    const roots = row.roots.join(", ");
    console.log(`${glyph} ${row.name}  ${row.description}  ${roots}`);
  }
}

const program = new Command();
program
  .name("skillctl")
  .description("prototype skill activation manager (dummy, fixture data only)")
  .version("0.1.0");

program
  .command("list")
  .description("print the fixture as a table (same glyphs as the TUI)")
  .option("--json", "print the fixture as JSON")
  .option("--rows <n>", "stress fixture: expand to N entries", parseRows)
  .action((opts: { json?: boolean; rows?: number }) => {
    const rows = expandRows(fixture.skills, opts.rows ?? fixture.skills.length);
    if (opts.json) {
      console.log(JSON.stringify({ ...fixture, skills: rows }, null, 2));
    } else {
      printTable(rows);
    }
  });

program
  .command("enable")
  .description("simulate enabling skills")
  .argument("<skills...>")
  .action((skills: string[]) => {
    const rows = resolveAllOrDie(skills);
    for (const row of rows) console.log(`would enable ${row.name}`);
  });

program
  .command("disable")
  .description("simulate disabling skills")
  .argument("<skills...>")
  .action((skills: string[]) => {
    const rows = resolveAllOrDie(skills);
    for (const row of rows) console.log(`would disable ${row.name}`);
  });

program
  .command("toggle")
  .description("print the resolved action per skill based on fixture state")
  .argument("<skills...>")
  .action((skills: string[]) => {
    const rows = resolveAllOrDie(skills);
    for (const row of rows) {
      const action = row.enabled ? "disable" : "enable";
      console.log(`would ${action} ${row.name} (currently ${row.enabled ? "enabled" : "disabled"})`);
    }
  });

const root = program
  .command("root")
  .description("inspect fixture roots");

root
  .command("list")
  .description("print root names and paths")
  .action(() => {
    for (const r of fixture.roots) console.log(`${r.name}  ${r.path}`);
  });

program
  .command("doctor")
  .description("print canned findings derived from the fixture")
  .action(() => {
    const enabled = fixture.skills.filter((s) => s.enabled).length;
    console.log(`${fixture.roots.length} roots configured: ${fixture.roots.map((r) => r.name).join(", ")}`);
    console.log(`${fixture.skills.length} skills: ${enabled} enabled, ${fixture.skills.length - enabled} disabled`);
    const multi = fixture.skills.filter((s) => s.roots.length > 1);
    console.log(
      `${multi.length} skills installed in multiple roots (${multi.map((s) => s.name).join(", ")})`,
    );
  });

program
  .command("tui")
  .description("open the interactive manager")
  .option("--rows <n>", "stress fixture: expand to N entries", parseRows)
  .action(async (opts: { rows?: number }) => {
    const { runTui } = await import("./main.tsx");
    runTui({ rows: opts.rows });
  });

if (process.argv.slice(2).length === 0) {
  program.outputHelp();
} else {
  // pnpm run forwards a literal '--' separator into argv; Commander does not
  // want it, so strip a leading one before parsing.
  const argv = process.argv.slice(2);
  if (argv[0] === "--") argv.shift();
  await program.parseAsync(argv, { from: "user" });
}
