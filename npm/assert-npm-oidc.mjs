#!/usr/bin/env node
// Fail fast when the ambient npm cannot do OIDC trusted publishing (npm
// gained it in 11.5.1; the runner image's npm 10.x silently cannot
// authenticate — how v0.1.0 and v0.2.0 died). Shared by release.yml (real
// publish) and release-readiness.yml (dry run) so the check lives in one
// place.
import { execFileSync } from "node:child_process";

const raw = execFileSync("npm", ["--version"], { encoding: "utf8" }).trim();
console.log(`npm ${raw}`);
const [maj, min] = raw.split(".").map(Number);
if (maj < 11 || (maj === 11 && min < 5)) {
  console.error(
    "fleet release: npm >= 11.5.1 required for OIDC trusted publishing",
  );
  process.exit(1);
}
