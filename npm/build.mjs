#!/usr/bin/env node
// Cross-compile the fleet Go binary for every npm platform package. Binaries
// land in `npm/fleet-<os>-<arch>/bin/fleet`, stamped with the given version
// via ldflags (`internal/buildinfo.Version` — the same knob `make build` and
// GoReleaser use). The npm tarballs publish those dirs; GoReleaser's `dist/`
// layout is never parsed.
//
// Usage: node npm/build.mjs [X.Y.Z]   (or FLEET_VERSION=X.Y.Z in env)
// The default `0.0.0-dev` matches the committed `package.json` placeholders;
// the release workflow passes the tag version instead.
import { execFileSync } from "node:child_process";
import { chmodSync, mkdirSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const REPO = join(dirname(fileURLToPath(import.meta.url)), "..");
const BUILDINFO = "github.com/zzacong/fleet/internal/buildinfo.Version";

// Package names follow the ticket contract verbatim (note linux uses `x64`,
// darwin uses `amd64`); GOARCH is always the Go spelling. Platform packages
// are scoped (`@zzacong/fleet-<os>-<arch>`) so all five share one OIDC
// trusted-publishing scope; `pkg` is the on-disk dir (`npm/<pkg>/bin`), which
// stays unscoped, while `name` is the published npm name.
export const TARGETS = [
  {
    goos: "darwin",
    goarch: "arm64",
    pkg: "fleet-darwin-arm64",
    name: "@zzacong/fleet-darwin-arm64",
  },
  {
    goos: "darwin",
    goarch: "amd64",
    pkg: "fleet-darwin-amd64",
    name: "@zzacong/fleet-darwin-amd64",
  },
  {
    goos: "linux",
    goarch: "arm64",
    pkg: "fleet-linux-arm64",
    name: "@zzacong/fleet-linux-arm64",
  },
  {
    goos: "linux",
    goarch: "amd64",
    pkg: "fleet-linux-x64",
    name: "@zzacong/fleet-linux-x64",
  },
];

const version = process.argv[2] ?? process.env.FLEET_VERSION ?? "0.0.0-dev";
if (version.length === 0) {
  console.error("fleet build: version must not be empty");
  process.exit(1);
}

for (const { goos, goarch, pkg } of TARGETS) {
  const outDir = join(REPO, "npm", pkg, "bin");
  mkdirSync(outDir, { recursive: true });
  const out = join(outDir, "fleet");
  execFileSync(
    "go",
    [
      "build",
      "-trimpath",
      "-ldflags",
      `-s -w -X ${BUILDINFO}=${version}`,
      "-o",
      out,
      "./cmd/fleet",
    ],
    {
      cwd: REPO,
      stdio: "inherit",
      env: { ...process.env, CGO_ENABLED: "0", GOOS: goos, GOARCH: goarch },
    },
  );
  chmodSync(out, 0o755);
  const { size } = statSync(out);
  console.log(
    `fleet build: ${pkg} (${goos}/${goarch}) -> ` +
      `${(size / 1048576).toFixed(1)} MB @ ${version}`,
  );
}
console.log(`fleet build: 4 targets stamped ${version}`);
