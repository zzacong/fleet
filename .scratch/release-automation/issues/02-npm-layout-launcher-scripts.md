# 02: npm layout, launcher, and build/stamp scripts

**What to build:** Five versioned npm packages exist under `npm/` with a launcher that runs the right prebuilt binary on any supported machine — proven end-to-end locally without touching any registry.

**Blocked by:** 01 (Root-module and www move).

**Status:** ready-for-agent

- [ ] Wrapper `@zzacong/fleet` plus four `fleet-<os>-<arch>` platform packages (darwin-arm64, darwin-amd64, linux-arm64, linux-x64); `0.0.0-dev` placeholders; wrapper depends on the platforms via `workspace:*`; platform packages declare `os`/`cpu` and publish only `bin/`.
- [ ] `build.mjs` cross-compiles all four targets with `CGO_ENABLED=0` and ldflags version stamping into each platform `bin/` dir; the launcher normalizes `process.platform`/`process.arch` to Go names, spawns the matching binary with args/stdio/exit-code passthrough, and errors clearly on unsupported platforms.
- [ ] A stamp script sets all five `version` fields from a given `X.Y.Z` and rewrites the wrapper's `workspace:*` ranges to that exact version; it is designed to run after install/build and immediately before publish, and its output is never committed.
- [ ] Local proof with no registry: build, pack the tarballs, install into a scratch dir, and `fleet --version` agrees through every platform binary the local machine can execute; pack dry-runs and publint are clean.
- [ ] Work stays on a local branch; nothing is pushed (owner's standing instruction).
