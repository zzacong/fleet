#!/usr/bin/env node
// Launcher for the `@zzacong/fleet` npm wrapper: runs the prebuilt fleet
// binary matching this machine, passing args/stdio/exit code through. The
// four platform packages (`@zzacong/fleet-<os>-<arch>`) are
// optionalDependencies of the wrapper, so npm installs only the matching one
// on any machine. Platform packages are scoped under `@zzacong/` so all five
// packages share one OIDC trusted-publishing setup (one npm scope, one repo,
// one workflow); the on-disk dirs stay unscoped (`npm/fleet-<os>-<arch>`).
import { spawnSync } from "node:child_process";
import { chmodSync, realpathSync } from "node:fs";
import { createRequire } from "node:module";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

// Node and Go spell 64-bit x86 differently (x64 vs amd64); the package
// suffixes follow the ticket contract verbatim (note linux uses `x64`,
// darwin uses `amd64`) and the asymmetry lives only in this table. Keys are
// Node `platform:arch` spellings; values are the scoped npm package names.
const PLATFORMS = {
  "darwin:arm64": "@zzacong/fleet-darwin-arm64",
  "darwin:x64": "@zzacong/fleet-darwin-amd64",
  "linux:arm64": "@zzacong/fleet-linux-arm64",
  "linux:x64": "@zzacong/fleet-linux-x64",
};

export function packageFor(platform, arch) {
  return PLATFORMS[`${platform}:${arch}`] ?? null;
}

export function supportedList() {
  return Object.keys(PLATFORMS)
    .map((key) => key.replace(":", "/"))
    .join(", ");
}

function main() {
  const name = packageFor(process.platform, process.arch);
  if (name === null) {
    console.error(
      `fleet: unsupported platform "${process.platform}/${process.arch}" ` +
        `(supported: ${supportedList()})`,
    );
    process.exit(1);
  }
  let binPath;
  try {
    binPath = createRequire(import.meta.url).resolve(`${name}/bin/fleet`);
  } catch {
    console.error(
      `fleet: binary package "${name}" for ` +
        `"${process.platform}/${process.arch}" is not installed; ` +
        "reinstall @zzacong/fleet without --no-optional",
    );
    process.exit(1);
  }
  // The binary can land without its exec bit (the 0.1.0 tarballs were
  // packed from a 0644 source) — restore owner exec before spawning.
  // Idempotent, survives pnpm store hardlinks.
  try {
    chmodSync(binPath, 0o755);
  } catch (error) {
    console.error(`fleet: cannot chmod "${binPath}": ${error.message}`);
    process.exit(1);
  }
  const child = spawnSync(binPath, process.argv.slice(2), {
    stdio: "inherit",
  });
  if (child.error) {
    console.error(`fleet: cannot run "${binPath}": ${child.error.message}`);
    process.exit(1);
  }
  process.exit(child.status ?? 1);
}

// Importable (for mapping tests) without side effects: `node -e
// "import(...)"` leaves argv[1] unset, while the .bin symlink resolves via
// realpath to this file when run as the `fleet` command.
const invokedAsBin =
  process.argv[1] !== undefined &&
  realpathSync(resolve(process.argv[1])) === fileURLToPath(import.meta.url);
if (invokedAsBin) {
  main();
}
