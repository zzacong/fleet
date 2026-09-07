# Changelog

## [0.3.0](https://github.com/zzacong/fleet/compare/v0.2.1...v0.3.0) (2026-09-07)


### Features

* **update:** detect newer fleet binary and print update notice ([#9](https://github.com/zzacong/fleet/issues/9)) ([0aaa420](https://github.com/zzacong/fleet/commit/0aaa4207bee7de63ab6d2b3d37522517550a9aab))

## [0.2.1](https://github.com/zzacong/fleet/compare/v0.2.0...v0.2.1) (2026-09-07)


### Bug Fixes

* **state:** distinguish older from newer state file versions in error ([c59116a](https://github.com/zzacong/fleet/commit/c59116a045df5574121313a96deb3e9e3d56e28d))

## [0.2.0](https://github.com/zzacong/fleet/compare/v0.1.1...v0.2.0) (2026-09-07)


### Features

* **install:** default INSTALL_DIR to ~/.local/bin ([b5bfe01](https://github.com/zzacong/fleet/commit/b5bfe0101dc6241a73d6325e6633040ed1e2bd06))

## [0.1.1](https://github.com/zzacong/fleet/compare/v0.1.0...v0.1.1) (2026-09-07)


### Bug Fixes

* **npm:** launcher restores exec bit on platform binary ([d2b0276](https://github.com/zzacong/fleet/commit/d2b0276f02c87a9c51e4d07a2dd792d7ed3eca51))
* **release:** delete orphaned release and tag on publish failure ([2eb842c](https://github.com/zzacong/fleet/commit/2eb842cc526ae7382d636449f4132dea1aaf2ec0))
* **release:** pnpm/setup owns node+npm, publish before goreleaser ([806e016](https://github.com/zzacong/fleet/commit/806e016bdfeb87ef09e4af345ea95e48eb7449f9))
* **release:** setup-node owns node+npm in publish job ([4b696dd](https://github.com/zzacong/fleet/commit/4b696ddc53af8e7cbc2426717afa1936d9954608))
* **release:** skip published versions, warn on devEngines mismatch ([2b7c7c5](https://github.com/zzacong/fleet/commit/2b7c7c52f55a2edcea8e72ae4db9be9866749e2c))

## 0.1.0 (2026-09-06)


### Features

* **cli:** add fleet harness ls to list supported and installed harnesses ([3e65f71](https://github.com/zzacong/fleet/commit/3e65f7176b590db555655c7d1ed3f044e9fee370))
* **cli:** fleet skill sync verb for explicit drift repair (spec story 6) ([5a2b45a](https://github.com/zzacong/fleet/commit/5a2b45aca8bac7ca2f9af73d2cba7274ae705ac5))
* **cli:** give sync's change lines the on/off outcome weight ([8334356](https://github.com/zzacong/fleet/commit/83343566c24656344ab3e4b3e6204b87b1ba8a73))
* **cli:** lead skill on/off with an outcome line, quiet ambient flags ([45e7e0e](https://github.com/zzacong/fleet/commit/45e7e0e41c214d5890949c2e7d93217a8bf9df2d))
* **cli:** style adopt output like sync with greyed prefix and paths ([af6246d](https://github.com/zzacong/fleet/commit/af6246d7030b110ca36bf767a00cd0e511ff1960))
* **cli:** unify on/off and update output with skill: and update: prefixes ([9c53152](https://github.com/zzacong/fleet/commit/9c5315202714bf32340dce0f711cd978c5e4e415))
* **cli:** wrap help descriptions at the description column ([e9723d1](https://github.com/zzacong/fleet/commit/e9723d17c202b3f724b1e3ed22fd25788de18e5c))
* **config:** add fleet config file and fleet config verbs (closes 01) ([8e27d57](https://github.com/zzacong/fleet/commit/8e27d57b8ee6f29e2b074aa5fc05880f504019a4))
* **config:** surface allowed keys in help and shell completion ([e9b2bf6](https://github.com/zzacong/fleet/commit/e9b2bf60fe2b83b6c82026fe6db66c86976fc506))
* **customs:** retarget adopt to resolved custom home (closes 04) ([b9b0484](https://github.com/zzacong/fleet/commit/b9b048497aa72688e75529d20e684cccac9fea80))
* **docs:** migrate user docs to Starlight at apps/docs (closes 09) ([eaafdb0](https://github.com/zzacong/fleet/commit/eaafdb060ac7c88c431bc2213d34c4f996621a5a))
* **doctor:** flag double presence across canonical, fleet-home and skills-repo (closes 05) ([164c7dd](https://github.com/zzacong/fleet/commit/164c7dd76b53e6f1bb2e3c2d05eaa3cfce1e62e8))
* **doctor:** flag store+repo double-presence and stale lock entries for adopted skills (ticket 03 follow-ups) ([29d02f7](https://github.com/zzacong/fleet/commit/29d02f74b6bba82dc8b66550b07cdb3cfec72c22))
* **doctor:** keep-all and skip-all for one harness's conflicts ([d4e2caf](https://github.com/zzacong/fleet/commit/d4e2cafcdc6150c0b0939e176aced899e4d1af55))
* **doctor:** report everything at once by default, add --interactive ([33630be](https://github.com/zzacong/fleet/commit/33630bea7311f171a4209aed792897ea20e24865))
* **ls:** readable table, colors, and grouped sync flags ([1be5358](https://github.com/zzacong/fleet/commit/1be5358934b3cbdaab038ccde54ae192529718ae))
* **monorepo:** scaffold apps/cli with go.work, pnpm-workspace, desktop and skills placeholders (closes 08) ([2bb6f03](https://github.com/zzacong/fleet/commit/2bb6f03d20ea68e07c14f655afe6fad58b69a832))
* **npm:** five-package bundled-binary layout with launcher and scripts (closes 02) ([7d9db1f](https://github.com/zzacong/fleet/commit/7d9db1ff2a19183cdc7618acdc8aaf7d448616b1))
* **paths:** explicit skills-repo resolution, no walk-up (closes 02) ([ec7edc9](https://github.com/zzacong/fleet/commit/ec7edc9b707ac8116085cfa50447cb5a56d9a18b))
* **prototypes:** build skillctl TUI prototypes for stack comparison ([5c8ed28](https://github.com/zzacong/fleet/commit/5c8ed2898ff3740eb9a0200b834104cc9f28debe))
* **pull-adopt:** multi-repo tracked set with skill pull, drop, and adopt target ([b2edefa](https://github.com/zzacong/fleet/commit/b2edefa9e64515711039cecd63f193826a633873))
* **release:** release-please, tag workflows, install script (closes 03) ([4551a8d](https://github.com/zzacong/fleet/commit/4551a8dfc34d3b16ce0d127f6971b678a9006d64))
* **scripts:** add fleet watcher for fleet-touched files ([3caa4ff](https://github.com/zzacong/fleet/commit/3caa4ff4ec6ffab0d0d41bc2ebfb0ff31e3a7bd9))
* **skill-adopt:** adopt moves customs into the repo and wires every harness (closes 03) ([52fb66e](https://github.com/zzacong/fleet/commit/52fb66e596edfd933659d54a49bedfa091f7a7e8))
* **skill-doctor:** redundant link cleanup and doctor with keep-or-restore (closes 04) ([7e5f331](https://github.com/zzacong/fleet/commit/7e5f33115ad948296216f71a91231c08eebff827))
* **skill-ls:** read-only fleet skill ls across six harnesses (closes 01) ([c157336](https://github.com/zzacong/fleet/commit/c157336a61f80ecf1fce8608cc07958442fdadda))
* **skill-ls:** update-available badge in table and --json (closes 06) ([a2d80ce](https://github.com/zzacong/fleet/commit/a2d80ceb51873a01e8ccb3e97c8adce9ec2cb0da))
* **skill-update:** wrapped skills update with post-run sync and own-state report (closes 05) ([1ecbee6](https://github.com/zzacong/fleet/commit/1ecbee695f8ab2c96ecf0607edc130caccb11c87))
* **skills:** adopt custom skills from canonical store ([ccfe6cf](https://github.com/zzacong/fleet/commit/ccfe6cfd0c17d969c2a295bfcb686745fd4bd24c))
* **skill:** state file, on/off writes through adapter seam, sync (closes 02) ([8971b4e](https://github.com/zzacong/fleet/commit/8971b4e8ef1f7e1a75b6a2dfd328a6f0afc956cf))
* **snapshot:** union fleet-home and skills-repo with precedence (closes 03) ([51a45ad](https://github.com/zzacong/fleet/commit/51a45ad585c25f55640bb9d83b1ca3fcfe18b77f))
* **tui:** abbreviate harness column labels with a help legend ([f81a404](https://github.com/zzacong/fleet/commit/f81a40404be7d62cffbb338f27020be90f2218be))
* **tui:** one-key update all through the wrapped skills CLI (spec story 14) ([aa1c092](https://github.com/zzacong/fleet/commit/aa1c092eeeaad582fc81d52d0dc1b0db7ff91fe2))
* **tui:** skill x harness matrix with staged toggling (closes 07) ([b924ba4](https://github.com/zzacong/fleet/commit/b924ba4e778dd5c1d0c4a1eb05954b475d24e281))
* **tui:** update single skill from TUI ([6e8fc24](https://github.com/zzacong/fleet/commit/6e8fc24f193dd905c25e2b2d2d3bbbd0d7d966bb))
* **watcher:** add project-scoped watcher skill ([184eaec](https://github.com/zzacong/fleet/commit/184eaec88445d759396e60b541fc40eef4409cfc))
* **watcher:** note fleet-home coverage in watch table and verify dir walk (closes 07) ([e8ece59](https://github.com/zzacong/fleet/commit/e8ece59e9cb6d714adc7bd8dcc6a6e559adf52ac))


### Bug Fixes

* **cli:** make skill update report obvious and match on/off styling ([c7ad77c](https://github.com/zzacong/fleet/commit/c7ad77c8b83a187ca4778676abf3e89a9cd01807))
* **cli:** quiet adopt output to match toggle styling ([e443d98](https://github.com/zzacong/fleet/commit/e443d98b9d84b93ff08dfcc2635a1cc57afb3c11))
* **cli:** restore adopt wiring output and make doctor handle broken symlinks ([debb5b3](https://github.com/zzacong/fleet/commit/debb5b3ea7b4d274b0a1f5309edb927ba37797b9))
* **cli:** suppress ambient flags in doctor restore receipt ([de236ec](https://github.com/zzacong/fleet/commit/de236ecd59e6e3fedc1ad88ded61a67b56090968))
* **harness:** skip hidden entries when scanning skills dirs ([83a22ca](https://github.com/zzacong/fleet/commit/83a22ca32c96727bfb8acf269f266b76449359c6))
* **review:** align comments and use bytes.TrimSpace for config ([480ea8c](https://github.com/zzacong/fleet/commit/480ea8c9466a3157693b0cb9aac195a904a36d2b))
* **review:** correct adopt re-wire, doctor stale-lock and managed links, deduplicate config parsing ([dbeecd5](https://github.com/zzacong/fleet/commit/dbeecd500cac784d5c8f5b6743efb40e2ea98113))
* **review:** update README install path, expand md fmt to Starlight, allow pnpm builds ([6cb3d16](https://github.com/zzacong/fleet/commit/6cb3d160be2b657a28b165647319ca09653f0c98))
* **test:** drop unused revs field in bareFailStub ([2b3a0cb](https://github.com/zzacong/fleet/commit/2b3a0cb41051cc49a28454820531cee851b52dd5))
* **test:** gofumpt formatting in pull_test ([db2a970](https://github.com/zzacong/fleet/commit/db2a970ae7ed35d4823a9f4b1fc993c0ec7d2a39))
* **tui:** hero banner spells FLEET, not FLFET ([aad9307](https://github.com/zzacong/fleet/commit/aad930711a5b3b258742cb8444091f844c51f1a2))
* **tui:** make the harness column cursor visible and fit narrow terminals ([f84ab15](https://github.com/zzacong/fleet/commit/f84ab15dc9c0f0e5661226e53ff3a2aac666a536))
* **tui:** wrap update-all notice instead of truncating to one line ([0956f78](https://github.com/zzacong/fleet/commit/0956f78d8ab1f3b629d1a5b7da6f471b910a0fbd))
* **watcher:** record directory symlinks ([16b0b62](https://github.com/zzacong/fleet/commit/16b0b628a88b6538d6815f31bd31c1c013102e1b))
* **watcher:** verify REPO depth still resolves to monorepo root after apps/cli move (closes 10) ([5541fdf](https://github.com/zzacong/fleet/commit/5541fdf6ecb6eee13a8b24373d6ebfd6e1fd0a85))


### Miscellaneous Chores

* set initial version ([af61733](https://github.com/zzacong/fleet/commit/af61733cca7b90844c8e4edb28defc147fd4e543))
