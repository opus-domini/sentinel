# Changelog

## [0.4.2](https://github.com/opus-domini/sentinel/compare/v0.4.1...v0.4.2) (2026-02-20)


### Bug Fixes

* extract orphan error constant to satisfy goconst lint ([46be998](https://github.com/opus-domini/sentinel/commit/46be998903a2e771d3b9ddffcfc395779a3fca8a))
* reconcile orphaned runbook runs stuck in running/queued on startup ([7326125](https://github.com/opus-domini/sentinel/commit/7326125a0808cda0eff69648ad149f52e9f02cea))
* use inline confirm for runbook job delete, matching alerts pattern ([ca5f9bf](https://github.com/opus-domini/sentinel/commit/ca5f9bff7441670d56bfc5f9f64ed11eeead6720))

## [0.4.1](https://github.com/opus-domini/sentinel/compare/v0.4.0...v0.4.1) (2026-02-20)


### Bug Fixes

* reconcile orphaned runbook runs stuck in running/queued on startup ([80f81b9](https://github.com/opus-domini/sentinel/commit/80f81b9a34f356f1f282398fc7953232a8ad52ba))

## [0.4.0](https://github.com/opus-domini/sentinel/compare/v0.3.12...v0.4.0) (2026-02-20)


### ⚠ BREAKING CHANGES

* remove command scope from guardrails, replace pattern input with action picker

### Features

* remove command scope from guardrails, replace pattern input with action picker ([2bbc165](https://github.com/opus-domini/sentinel/commit/2bbc1659d118e3c056f728c5a19357a2c59c684b))


### Bug Fixes

* add pointer cursor to dialog close button ([b6b63e1](https://github.com/opus-domini/sentinel/commit/b6b63e10d3d1f05de030d763f7a82acd9cebdf24))
* add pointer cursor to tmux header action buttons ([24f0df0](https://github.com/opus-domini/sentinel/commit/24f0df0a440a180bb4b1462894c25d15c29d3c73))
* mobile UX — overflow fixes, alert timestamps, dismiss confirmation, sparkline height ([66ad4d3](https://github.com/opus-domini/sentinel/commit/66ad4d3349df2e036c030e48681448011984e6cb))
* position tmux dialogs at top of viewport instead of centered ([e244da4](https://github.com/opus-domini/sentinel/commit/e244da4554469eb9a218d636b9fb412228112695))

## [0.3.12](https://github.com/opus-domini/sentinel/compare/v0.3.11...v0.3.12) (2026-02-20)


### Features

* add cookie_secure policy for auth cookie hardening ([85547e7](https://github.com/opus-domini/sentinel/commit/85547e7213ddc637510f230e9accad40e0be4b73))
* UX feature audit — guardrails, alerts, timeline, recovery ([909cae9](https://github.com/opus-domini/sentinel/commit/909cae9ea5575d1634bb579b09613246eb16d3d1))
* UX polish — guardrails dialog, icon-only headers, standardized dialogs ([0f9b436](https://github.com/opus-domini/sentinel/commit/0f9b436d1f761c6d77a2ded3f26041044ca72aa8))
* UX polish — snapshots button, activities rename, guardrails cleanup, nav reorder ([24a1598](https://github.com/opus-domini/sentinel/commit/24a1598ff07a00deabf88c9e40f0b84c9b3a986c))


### Bug Fixes

* add loop observability, HealthChecker guards, and ticker doneCh ([63349cf](https://github.com/opus-domini/sentinel/commit/63349cf39a1d7258f64b39bf0351f436f06a188e))
* **auth:** migrate token flow to HttpOnly cookie and lock unauthenticated UI ([ee54480](https://github.com/opus-domini/sentinel/commit/ee54480f31e29985c02402bea302d853908cc8ea))
* enforce cookie_secure policy and block insecure remote startup ([8b996b2](https://github.com/opus-domini/sentinel/commit/8b996b231d1ebf2308ca37b1aa005d814f111651))
* ensure deterministic terminal state for async jobs on cancellation ([ed62a67](https://github.com/opus-domini/sentinel/commit/ed62a67d83725c48ef3f1eba09533e8423f524f3))
* harden domain invariants and close safety gaps ([53a2f1d](https://github.com/opus-domini/sentinel/commit/53a2f1dd31325c71e4886af6a29c761aa47c306c))
* implement http.Hijacker on statusRecorder for WebSocket upgrade ([c2d95c0](https://github.com/opus-domini/sentinel/commit/c2d95c0da5053c0aa983ec71bdb3841431fb359e))
* improve mobile responsiveness and add empty state placeholders ([6b3ab31](https://github.com/opus-domini/sentinel/commit/6b3ab3191c3fe1ca22edb775ccbcf76e5cff249e))
* initialize recovery runCtx in constructor to prevent CLI panic ([0643411](https://github.com/opus-domini/sentinel/commit/064341135619e80cd2bed2a43a0cf7b73f3471c4))
* limit scheduler goroutine burst and validate unit action inputs ([3b97c9d](https://github.com/opus-domini/sentinel/commit/3b97c9de89c3ace2ab847758faaa5b298694ed64))
* preserve once schedule disabled state and bound catch-up burst ([573959a](https://github.com/opus-domini/sentinel/commit/573959a3a8ea487832df76730b25212fea79e23c))
* refocus terminal after tmux window tab switch ([ccd3cce](https://github.com/opus-domini/sentinel/commit/ccd3cce890e6802a005d472e3aead6abc639cd11))
* resolve ESLint duplicate import and type-only import errors ([30797aa](https://github.com/opus-domini/sentinel/commit/30797aab11ee54d631d2356a9868e45d746fecd1))
* resolve ESLint duplicate import and type-only import errors ([62d796e](https://github.com/opus-domini/sentinel/commit/62d796e4da1099e515cfdb42434299595600b11a))
* resolve ESLint duplicate import and type-only import errors ([9f5e524](https://github.com/opus-domini/sentinel/commit/9f5e524d7330b1901101b2e60951bbf50c477544))
* resolve exhaustive switch and goconst lint issues ([e349620](https://github.com/opus-domini/sentinel/commit/e34962066d8780562b5b6d70512c2fe47610a4ba))
* scheduler recurrence, manual trigger completion, and CLI restore contract ([7a2018f](https://github.com/opus-domini/sentinel/commit/7a2018feba855985f3fbff12792e71a28a4c5c8f))
* wrap SideRail e2e test with LayoutContext provider ([8ffde77](https://github.com/opus-domini/sentinel/commit/8ffde77a5148ee71109d6df2a3bdd3fba853a1c7))


### Refactors

* add lifecycle context propagation and scheduler concurrency control ([19e315e](https://github.com/opus-domini/sentinel/commit/19e315e1c5ac4035a7e1083614ebf6f140da52bc))
* consolidate WS auth checks and propagate request context to PTY ([03dc652](https://github.com/opus-domini/sentinel/commit/03dc6521258a9059b629c83df3482e2d2836b5e4))
* create alerts/timeline domain packages and rename ops to services ([b939fd0](https://github.com/opus-domini/sentinel/commit/b939fd08266ed1e414d2aa99f85fddbd94b03021))
* create single opsManager in composition root ([59019bc](https://github.com/opus-domini/sentinel/commit/59019bcd5758e5fff6efb9f442a6040fcb0da52d))
* decompose store/ops.go into domain-specific files ([e9318f4](https://github.com/opus-domini/sentinel/commit/e9318f4715778007dee15fffbd8a8834aadde511))
* decouple guardrails, runbook, scheduler from *store.Store ([8caaeef](https://github.com/opus-domini/sentinel/commit/8caaeef270939570a79bee17f400965cb1c99f3b))
* decouple recovery and watchtower from *store.Store ([680074a](https://github.com/opus-domini/sentinel/commit/680074aef5c93e12bb1ec6754963f2e37f91e29b))
* extract orchestrations from handlers into opsOrchestrator ([1ffaf3a](https://github.com/opus-domini/sentinel/commit/1ffaf3a6bc7afb48fca975d25c78d8b2deebc36d))
* improve request logging and remove dead security code ([d9ee213](https://github.com/opus-domini/sentinel/commit/d9ee21351886cfc88fa20d5625a24a53625b13f8))
* remove *store.Store from handler structs in api and httpui ([7f674ee](https://github.com/opus-domini/sentinel/commit/7f674eec53219ab28676dab7e95ffcbdc5eb5bc1))
* rename internal/service to internal/daemon ([16ea46a](https://github.com/opus-domini/sentinel/commit/16ea46a337c6ad296445c15c8c57c92ff0a3193b))

## [0.3.11](https://github.com/opus-domini/sentinel/compare/v0.3.10...v0.3.11) (2026-02-19)


### Bug Fixes

* simplify remote security validation and improve startup logs ([d5f9886](https://github.com/opus-domini/sentinel/commit/d5f9886e0dd7ed7f889efbee895ccb93b613ae93))
* simplify remote security validation and improve startup logs ([df73c21](https://github.com/opus-domini/sentinel/commit/df73c21f73581c1cbccb50104a521ab1dbb6351a))

## [0.3.10](https://github.com/opus-domini/sentinel/compare/v0.3.9...v0.3.10) (2026-02-18)


### Bug Fixes

* bump Go to 1.25.7 to resolve stdlib vulnerabilities ([aa9224b](https://github.com/opus-domini/sentinel/commit/aa9224b14447db88f6ad9d5d7aef620fff8b85b6))


### Refactors

* gold standard audit — security, code quality, CI, frontend, tests, and API consistency ([28a3128](https://github.com/opus-domini/sentinel/commit/28a3128c3d688f5b5921352cf8dede3b90bc2c8f))


### CI

* bump GitHub Actions to latest major versions ([7b5259d](https://github.com/opus-domini/sentinel/commit/7b5259d130138c6e3d13c0641d3bc876775101a4))

## [0.3.9](https://github.com/opus-domini/sentinel/compare/v0.3.8...v0.3.9) (2026-02-18)


### Features

* add built-in Apply Update runbook ([ab4aa39](https://github.com/opus-domini/sentinel/commit/ab4aa394aa94c6c152ea75849dbf427c550dc1a3))


### Bug Fixes

* add return after t.Fatal in nil guards to satisfy staticcheck SA5011 ([fc21299](https://github.com/opus-domini/sentinel/commit/fc212990b7ff0018b8d60ec6eeb4ebb0edd45466))
* clipboard crash and silent failure on plain HTTP ([02c94e2](https://github.com/opus-domini/sentinel/commit/02c94e2ce89818a7d9cc7f90243231aec0893c24))
* replace crypto.randomUUID with fallback for non-HTTPS contexts ([a8b6b93](https://github.com/opus-domini/sentinel/commit/a8b6b939de090024da40e9e6c7947f7c15f05ced))

## [0.3.8](https://github.com/opus-domini/sentinel/compare/v0.3.7...v0.3.8) (2026-02-18)


### Features

* add rich log viewer with streaming for services page ([37cecdb](https://github.com/opus-domini/sentinel/commit/37cecdb0dacd92aa001478f23de469cbb2289977))
* add sparklines with hover tooltips to metrics page ([268f213](https://github.com/opus-domini/sentinel/commit/268f213d301ff90885c86e554632cbb3c59fd120))

## [0.3.7](https://github.com/opus-domini/sentinel/compare/v0.3.6...v0.3.7) (2026-02-18)


### Bug Fixes

* enable clipboard copy for mouse-selected text in web terminal ([6b0ab83](https://github.com/opus-domini/sentinel/commit/6b0ab83c40f2f348d77faed700581987c8531c2c))
* enable clipboard copy for mouse-selected text in web terminal ([8a18c74](https://github.com/opus-domini/sentinel/commit/8a18c74d85a810a06941e035b5e52b22a2ddcc77))

## [0.3.6](https://github.com/opus-domini/sentinel/compare/v0.3.5...v0.3.6) (2026-02-18)


### Features

* add runbook scheduling and live indicators for timeline/alerts ([2f38a78](https://github.com/opus-domini/sentinel/commit/2f38a786c343f42ce5414a7d1ba025659c41f459))
* push metrics over WebSocket and fix settings dialog on mobile ([411a078](https://github.com/opus-domini/sentinel/commit/411a07841c1541b164a479296ba547ae7b2e8bb4))


### Bug Fixes

* align live indicators with backend 5s push tickers ([a4b8427](https://github.com/opus-domini/sentinel/commit/a4b84278a4c3cacef64f2986c5b5c7565ab93d10))
* **client:** replace timestamps with static refresh rate indicator ([c1a597f](https://github.com/opus-domini/sentinel/commit/c1a597fde8bb6869e4cba5b3d09d12318b1892e4))
* **client:** show relative time for metrics timestamps ([78b34c5](https://github.com/opus-domini/sentinel/commit/78b34c53ee440cc2ade2e5777717b4851f1db029))


### Documentation

* add screenshots and captions across all feature docs and README ([bfc374e](https://github.com/opus-domini/sentinel/commit/bfc374e5830e62b743e15be53516ea210fd1c301))
* prefix ops control plane captions with feature names ([7311a21](https://github.com/opus-domini/sentinel/commit/7311a21a56bb769076eec74906aac6de582c08a4))
* rewrite README facecard and remove legacy /ops references ([8ddda33](https://github.com/opus-domini/sentinel/commit/8ddda33d899249977f35ccdc8f550a1df1860ee3))

## [0.3.5](https://github.com/opus-domini/sentinel/compare/v0.3.4...v0.3.5) (2026-02-18)


### Features

* **client:** improve mobile responsiveness and runbook consistency ([9b2f984](https://github.com/opus-domini/sentinel/commit/9b2f984cb6e1d23339cd7dd931e3dfe3718fc76d))

## [0.3.4](https://github.com/opus-domini/sentinel/compare/v0.3.3...v0.3.4) (2026-02-18)


### Bug Fixes

* resolve lint issues in new test files ([763e99a](https://github.com/opus-domini/sentinel/commit/763e99a58c9a6c5e36d253a30714bd55cdf3d9be))

## [0.3.3](https://github.com/opus-domini/sentinel/compare/v0.3.2...v0.3.3) (2026-02-18)


### Features

* **client:** redesign SideRail with lucide icons and active indicator ([83b5c3c](https://github.com/opus-domini/sentinel/commit/83b5c3c69ca848b69d67ef70864884cbef625e6e))

## [0.3.2](https://github.com/opus-domini/sentinel/compare/v0.3.1...v0.3.2) (2026-02-18)


### Features

* **client:** add help dialog to tmux, services, and runbooks sidebars ([27a87e5](https://github.com/opus-domini/sentinel/commit/27a87e54c2f212c6c75669343b8a45ab08f52bf8))
* **client:** split ops into dedicated alerts, timeline, and metrics pages ([d53233b](https://github.com/opus-domini/sentinel/commit/d53233b7b012b36986ec1ea04e749cdf9d9106d9))
* **ops:** persist runbook step output and add dedicated pages ([aaf1a88](https://github.com/opus-domini/sentinel/commit/aaf1a88de404e02dfa92981a860fd117965fcdba))


### Documentation

* update documentation for ops page breakup and new features ([83979fd](https://github.com/opus-domini/sentinel/commit/83979fd46140931698348746282b542dc6f1f217))

## [0.3.1](https://github.com/opus-domini/sentinel/compare/v0.3.0...v0.3.1) (2026-02-15)


### Features

* **client:** refine ops and session sidebar UX ([0941f21](https://github.com/opus-domini/sentinel/commit/0941f21f140957c0674c6f9b53e17e3e74841de8))
* **ops:** add browsed services and unit-level controls ([b8e7e23](https://github.com/opus-domini/sentinel/commit/b8e7e23f1168c6b57ca00bd903e999fcd6e81383))

## [0.3.0](https://github.com/opus-domini/sentinel/compare/v0.2.13...v0.3.0) (2026-02-15)


### ⚠ BREAKING CHANGES

* removed /api/terminals* and /ws/terminals endpoints, plus /terminal and /terminals routes.

### Features

* **client:** cache tmux state with react-query ([c98d6a0](https://github.com/opus-domini/sentinel/commit/c98d6a02a8e43f848be11001e85c4d0c017752dc))
* **client:** expand react-query caching across ops and settings ([bc7fde4](https://github.com/opus-domini/sentinel/commit/bc7fde44028ca38a6104a993591cda4de341f1e6))
* **client:** persist tmux tabs and add react-query dependency ([1d5de25](https://github.com/opus-domini/sentinel/commit/1d5de25f7217bf80d8bcc3550570c3d1494a04d5))
* **metrics:** add host metrics collection for CPU, memory, disk, and load averages ([ef81bf4](https://github.com/opus-domini/sentinel/commit/ef81bf4d3085eb982d76d68d52463ecb204e9629))
* **ops:** add service status inspect and resilient updater actions ([b06df3b](https://github.com/opus-domini/sentinel/commit/b06df3bfef006914105815e3ff3a444eb36c20ae))
* **ops:** discover and register host services from sidebar ([5e41c96](https://github.com/opus-domini/sentinel/commit/5e41c9686085ca9398fa16e08f35197495106afe))
* replace standalone terminals with ops control plane ([8a291bd](https://github.com/opus-domini/sentinel/commit/8a291bd5191d96b2bfc7bad9bdc6f77a705a0d4f))
* **runbook:** implement runbook executor for sequential step execution ([ef81bf4](https://github.com/opus-domini/sentinel/commit/ef81bf4d3085eb982d76d68d52463ecb204e9629))
* **store:** add CRUD operations for custom services and runbooks ([ef81bf4](https://github.com/opus-domini/sentinel/commit/ef81bf4d3085eb982d76d68d52463ecb204e9629))
* **watchtower:** enhance timeline event publishing for significant changes ([ef81bf4](https://github.com/opus-domini/sentinel/commit/ef81bf4d3085eb982d76d68d52463ecb204e9629))


### Bug Fixes

* **client:** polish ops panel affordances and header layout ([87a289e](https://github.com/opus-domini/sentinel/commit/87a289ea09afa25a8761dde7d043111bf01b9c5b))
* **client:** prevent tmux inspector loading race on route return ([507baf6](https://github.com/opus-domini/sentinel/commit/507baf6c247a6031aecbccf7294b1e0c6ba05ec1))

## [0.2.13](https://github.com/opus-domini/sentinel/compare/v0.2.12...v0.2.13) (2026-02-15)


### Refactors

* reduce backend cyclomatic complexity ([18862e0](https://github.com/opus-domini/sentinel/commit/18862e027e4a0e18b0d59731c8b0b6ab35df77ee))

## [0.2.12](https://github.com/opus-domini/sentinel/compare/v0.2.11...v0.2.12) (2026-02-15)


### Features

* expand timeline ops and add storage controls ([029f40d](https://github.com/opus-domini/sentinel/commit/029f40d43e15cffff4305238352aa5f416d499ca))


### Documentation

* restructure docsify guide and expand product documentation ([62f2b5d](https://github.com/opus-domini/sentinel/commit/62f2b5db716924198ab3124d8379f054422ab566))
* simplify root readme and point to detailed docs ([6139803](https://github.com/opus-domini/sentinel/commit/61398034c51ee5d6b611bab1e1fecd5da9d1f102))

## [0.2.11](https://github.com/opus-domini/sentinel/compare/v0.2.10...v0.2.11) (2026-02-15)


### Bug Fixes

* **service:** restart active unit on install ([9cbc6d0](https://github.com/opus-domini/sentinel/commit/9cbc6d0903b6285db6c7ceea19b7d09344fa459d))

## [0.2.10](https://github.com/opus-domini/sentinel/compare/v0.2.9...v0.2.10) (2026-02-15)


### Bug Fixes

* **service:** preserve home/config resolution for system installs ([5501e5f](https://github.com/opus-domini/sentinel/commit/5501e5f907ff94b7088237c8538039641adb14c2))

## [0.2.9](https://github.com/opus-domini/sentinel/compare/v0.2.8...v0.2.9) (2026-02-15)


### Bug Fixes

* **install:** unify service install behavior across scopes ([8d99587](https://github.com/opus-domini/sentinel/commit/8d99587cc012664171eff187317e9ca5cf11255a))
* **install:** use sentinel service name for root installs ([eb76405](https://github.com/opus-domini/sentinel/commit/eb76405224e1a4b76102ac8b303469de5db8944c))

## [0.2.8](https://github.com/opus-domini/sentinel/compare/v0.2.7...v0.2.8) (2026-02-15)


### Refactors

* **cli:** unify cross-platform service semantics and scope flags ([91a40c2](https://github.com/opus-domini/sentinel/commit/91a40c2e4a0950eb52849b05db2345a7e8214528))

## [0.2.7](https://github.com/opus-domini/sentinel/compare/v0.2.6...v0.2.7) (2026-02-15)


### Features

* **ui:** show runtime version in settings and harden status parsing ([178ef2f](https://github.com/opus-domini/sentinel/commit/178ef2fc9d8be570f39f8c19220e3ec0c15eb489))

## [0.2.6](https://github.com/opus-domini/sentinel/compare/v0.2.5...v0.2.6) (2026-02-15)


### Features

* **service:** add seamless auto scope across launchd and updater restart ([945046f](https://github.com/opus-domini/sentinel/commit/945046f8ed29c6015e3c4a7ee25bc6ab396ec320))


### Refactors

* **cli:** standardize long option style in help and docs ([51c3d7f](https://github.com/opus-domini/sentinel/commit/51c3d7f368ee3f3af23a7cbeb6d50ad7b4201943))

## [0.2.5](https://github.com/opus-domini/sentinel/compare/v0.2.4...v0.2.5) (2026-02-15)


### Features

* add launchd service and autoupdate support for macos ([5d08b34](https://github.com/opus-domini/sentinel/commit/5d08b34b7a4eb108b2580f120ec78be11822e0bd))
* add secure autoupdate workflow for service installs ([25b7774](https://github.com/opus-domini/sentinel/commit/25b77740c984980013eb53acf786347f5d810e27))
* improve cli output readability with pretty status formatting ([07f87b1](https://github.com/opus-domini/sentinel/commit/07f87b141fa2c7720711e41ae26f61d11b1a5ee5))


### CI

* publish checksums and embed release version in binaries ([a1d508d](https://github.com/opus-domini/sentinel/commit/a1d508dae06c05ed08971040aae3546234907e04))


### Documentation

* document autoupdate commands and install options ([f968733](https://github.com/opus-domini/sentinel/commit/f968733d0b72f9676ac8c78afcb27ffb4f1e4e56))

## [0.2.4](https://github.com/opus-domini/sentinel/compare/v0.2.3...v0.2.4) (2026-02-14)


### Features

* **client:** add mobile touch-to-scroll bridge for tmux ([df7db0d](https://github.com/opus-domini/sentinel/commit/df7db0d0fb31d63b80a0d7f579927f673c31e8ff))
* **client:** make session rename optimistic and stable ([a03f7a3](https://github.com/opus-domini/sentinel/commit/a03f7a3705b41cda694ac606c54421447b7eabdf))
* **client:** standardize optimistic tmux mutations ([836cd9f](https://github.com/opus-domini/sentinel/commit/836cd9f0b4c29d9a02f34ff43382fac2ea07c358))
* **events:** enrich tmux updates with inspector patches and event IDs ([226e62e](https://github.com/opus-domini/sentinel/commit/226e62eb82f53726999fb1fd58694bd50e2b5225))
* **pwa:** add install/update flow with local app shell caching ([fafe9d1](https://github.com/opus-domini/sentinel/commit/fafe9d18abdde46057f6a118a6d8433588d4087d))
* **tmux:** auto-name new windows and panes ([41a066b](https://github.com/opus-domini/sentinel/commit/41a066bbebdb95f988bb90a42a7cba51a1866437))
* **tmux:** keep window naming monotonic and create on right ([30f6ec3](https://github.com/opus-domini/sentinel/commit/30f6ec3111dfafc7491d17a8005d4311e8e1185b))


### Bug Fixes

* **api:** name new windows by session size ([6717427](https://github.com/opus-domini/sentinel/commit/6717427e7e9c717d5ae6e0eff8bcebfd72b4703b))
* **client:** harden optimistic tmux create/kill flows ([b49854c](https://github.com/opus-domini/sentinel/commit/b49854c2e493cd2f39ecc31f98cb076c9c70f777))
* **client:** make split panes optimistic and right-aligned ([a56ac4c](https://github.com/opus-domini/sentinel/commit/a56ac4c70a75d1d4f40c3e1ceedc00709e263fe5))
* **client:** prevent ghost windows after optimistic close/create ([6431ae8](https://github.com/opus-domini/sentinel/commit/6431ae8834d73d768cf94f812847a63814cebb34))
* **client:** stabilize mobile touch scroll with keyboard open ([7f9edef](https://github.com/opus-domini/sentinel/commit/7f9edef0573bbc23173da26489fcae504b61eec9))
* **tmux:** keep optimistic session creation attached until backend confirms ([7982c83](https://github.com/opus-domini/sentinel/commit/7982c835816126cd67cd18e6df3b67e6f9482df8))
* **tmux:** make session kill optimistic with rollback ([75cdedd](https://github.com/opus-domini/sentinel/commit/75cdedd7c2a924a4aca432dca0b29fa1f61c306a))


### Performance

* **client:** reduce activity-driven session refresh fallback ([11d6e79](https://github.com/opus-domini/sentinel/commit/11d6e79de38ce7da275449dc55d822c4b0723bb3))


### Documentation

* **readme:** document frontend realtime metrics diagnostics ([2a688f1](https://github.com/opus-domini/sentinel/commit/2a688f1232af684c56343d2b9f007597d0052f71))

## [0.2.3](https://github.com/opus-domini/sentinel/compare/v0.2.2...v0.2.3) (2026-02-14)


### Features

* implement watchtower activity projections and unread UX ([cb16068](https://github.com/opus-domini/sentinel/commit/cb160680258e45a73080b9864611bdfa7cd175f4))
* support seen acknowledgements over events websocket ([8aabb72](https://github.com/opus-domini/sentinel/commit/8aabb7251a7f84ab361488c809bdff3ac65d45b9))


### Bug Fixes

* keep idle session metadata visible in sidebar ([21d02bd](https://github.com/opus-domini/sentinel/commit/21d02bda04e68b7001843cfd8f8b8d030f907174))
* prevent session list cards from causing horizontal overflow ([efb6349](https://github.com/opus-domini/sentinel/commit/efb6349def2a6e25c80977bf90b13bdbc4b71273))
* refresh window unread state without pane refetch storms ([8b580c0](https://github.com/opus-domini/sentinel/commit/8b580c01fcadd7c0e1b2db2e249bfb98258fe0c3))
* simplify unread indicators in tmux ui ([05ab75a](https://github.com/opus-domini/sentinel/commit/05ab75a21d6701575bce90b8953e9de19c2f7bcf))
* stabilize attached session grouping during attach transitions ([ed2906b](https://github.com/opus-domini/sentinel/commit/ed2906b6cfadd15fff80c17829b09a3f54789ccb))


### Performance

* apply watchtower session deltas over events websocket ([10c5ead](https://github.com/opus-domini/sentinel/commit/10c5ead7a9db9d24928e4e35b56a9426d468f115))
* eliminate inspector refresh leaks across sessions ([e5a17d6](https://github.com/opus-domini/sentinel/commit/e5a17d617a297ba72f63de10fc025996fd949106))
* move tmux presence heartbeat to websocket ([e824c39](https://github.com/opus-domini/sentinel/commit/e824c3903b7146b737163ee20c6a54cad598e398))
* reduce activity event fan-out to inspector fetches ([e969c06](https://github.com/opus-domini/sentinel/commit/e969c066b59f8684671f213425132080ff1407b3))
* reduce sessions polling with seen session patches ([05a192d](https://github.com/opus-domini/sentinel/commit/05a192d1ea290bba43b8be8d29d7f4e98f6f30b6))
* skip redundant tmux select and keep optimistic selection ([1a54be6](https://github.com/opus-domini/sentinel/commit/1a54be655d2fcaf62fdfd027a311af5581afd07c))
* stop inspector polling loop on stable unread output ([478f772](https://github.com/opus-domini/sentinel/commit/478f772b52fc6960e4b0db29c0b85e686d34f22c))
* stop inspector refetch on recovery activity ([59795b7](https://github.com/opus-domini/sentinel/commit/59795b75e4d366d249c62970114fb2a764928b9e))
* throttle session refreshes from activity events ([a98f919](https://github.com/opus-domini/sentinel/commit/a98f919fa9296b3a4dd537c1f447bb86b441d749))


### Refactors

* build recovery snapshots from watchtower projections ([e4eb050](https://github.com/opus-domini/sentinel/commit/e4eb050a1f4e6c567fc9628736e9b7e65ad119f9))


### CI

* add release-please token fallback and permissions docs ([f0c5ec9](https://github.com/opus-domini/sentinel/commit/f0c5ec9cefeda54cdfd53e6551448b4d0df09376))
* **release:** automate versioning and release notes ([fa5e337](https://github.com/opus-domini/sentinel/commit/fa5e337d747b2f67b6e0cabf9b0068cbe7100c41))
* use github token only for release-please ([2a79860](https://github.com/opus-domini/sentinel/commit/2a79860100ef7a72279302ea634d0a3090581981))
