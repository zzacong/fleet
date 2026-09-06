#!/usr/bin/env node
// Publish all five npm packages in dependency order: the four platform
// packages first, the `@zzacong/fleet` wrapper last (its
// `optionalDependencies` resolve at install time, so every platform tarball
// must already be live). Both CI (`release.yml`) and the one-time local
// bootstrap delegate here so the ordering lives in one place.
//
// Usage: node npm/publish.mjs [--dry-run] [--no-provenance]
//   --dry-run        pass through to `npm publish` (registry untouched)
//   --no-provenance  omit `--provenance` (local bootstrap has no OIDC token;
//                    CI always publishes with provenance)
// Prerequisite: `node npm/stamp.mjs X.Y.Z` must have run first — this script
// refuses when the five versions disagree or a `workspace:*` range remains.
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const REPO = join(dirname(fileURLToPath(import.meta.url)), "..");
// Same order as `stamp.mjs` PLATFORM_DIRS; wrapper is always last.
const ORDER = [
  "fleet-darwin-arm64",
  "fleet-darwin-amd64",
  "fleet-linux-arm64",
  "fleet-linux-x64",
  "fleet",
];

const flags = new Set(process.argv.slice(2));
for (const flag of flags) {
  if (flag !== "--dry-run" && flag !== "--no-provenance") {
    console.error(`fleet publish: unknown flag ${flag}`);
    process.exit(1);
  }
}
const dryRun = flags.has("--dry-run");
const provenance = !flags.has("--no-provenance");

const manifests = ORDER.map((dir) => {
  const path = join(REPO, "npm", dir, "package.json");
  return { dir, pkg: JSON.parse(readFileSync(path, "utf8")) };
});
const versions = new Set(manifests.map(({ pkg }) => pkg.version));
if (versions.size !== 1) {
  console.error(
    `fleet publish: version mismatch (${[...versions].join(", ")}); run node npm/stamp.mjs X.Y.Z first`,
  );
  process.exit(1);
}
const serialized = JSON.stringify(manifests.map(({ pkg }) => pkg));
if (serialized.includes("workspace:")) {
  console.error(
    "fleet publish: workspace:* range remains; run node npm/stamp.mjs X.Y.Z first",
  );
  process.exit(1);
}

for (const { dir, pkg } of manifests) {
  const args = ["publish", join(REPO, "npm", dir), "--access", "public"];
  if (provenance) args.push("--provenance");
  if (dryRun) args.push("--dry-run");
  console.log(
    `fleet publish: ${pkg.name}@${pkg.version}${dryRun ? " (dry run)" : ""}`,
  );
  execFileSync("npm", args, { cwd: REPO, stdio: "inherit" });
}
console.log(`fleet publish: 5 packages @ ${manifests[0].pkg.version}`);
