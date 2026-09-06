#!/usr/bin/env node
// Stamp the release version into all five npm `package.json` files, rewriting
// the wrapper's `workspace:*` platform ranges to that exact version. Runs
// after install/build and immediately before `npm publish`; its output is
// never committed — git keeps the `0.0.0-dev` placeholders forever.
//
// Usage: node npm/stamp.mjs X.Y.Z   (idempotent: reruns just re-set)
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const REPO = join(dirname(fileURLToPath(import.meta.url)), "..");
// On-disk dirs stay unscoped; published names are scoped under `@zzacong/`
// so all five packages share one OIDC trusted-publishing scope.
const PLATFORM_DIRS = [
  "fleet-darwin-arm64",
  "fleet-darwin-amd64",
  "fleet-linux-arm64",
  "fleet-linux-x64",
];
const depName = (dir) => `@zzacong/${dir}`;

const SEMVER = /^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$/;

const version = process.argv[2];
if (version === undefined || !SEMVER.test(version)) {
  console.error("fleet stamp: usage: node npm/stamp.mjs X.Y.Z");
  process.exit(1);
}

function stamp(dir, rewriteWorkspaceRanges) {
  const path = join(REPO, "npm", dir, "package.json");
  const pkg = JSON.parse(readFileSync(path, "utf8"));
  pkg.version = version;
  if (rewriteWorkspaceRanges) {
    for (const dir of PLATFORM_DIRS) {
      const dep = depName(dir);
      if (pkg.optionalDependencies?.[dep]?.startsWith("workspace:")) {
        pkg.optionalDependencies[dep] = version;
      }
    }
  }
  writeFileSync(path, `${JSON.stringify(pkg, null, 2)}\n`);
  console.log(`fleet stamp: ${pkg.name}@${version}`);
}

stamp("fleet", true);
for (const dir of PLATFORM_DIRS) {
  stamp(dir, false);
}
