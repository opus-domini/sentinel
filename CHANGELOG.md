# Changelog

## [0.9.4](https://github.com/opus-domini/sentinel/compare/v0.9.3...v0.9.4) (2026-07-03)


### Features

* **tmux:** launcher dropdown/dialog UX and shared directory autocomplete ([#214](https://github.com/opus-domini/sentinel/issues/214)) ([b3d7345](https://github.com/opus-domini/sentinel/commit/b3d7345c3e62fae0ab21f8fe1ea97a3504791b67))

## [0.9.3](https://github.com/opus-domini/sentinel/compare/v0.9.2...v0.9.3) (2026-06-28)


### Bug Fixes

* **tmux:** refine launcher dropdown and dialog UX ([#210](https://github.com/opus-domini/sentinel/issues/210)) ([d71a4a8](https://github.com/opus-domini/sentinel/commit/d71a4a862734614b84b8b3da17ec5cf916d4a6f2))

## [0.9.2](https://github.com/opus-domini/sentinel/compare/v0.9.1...v0.9.2) (2026-06-16)


### Bug Fixes

* avoid broken pipe when resolving installer release ([0fe1c88](https://github.com/opus-domini/sentinel/commit/0fe1c88d4ee6bc42b836f223b8cc9c44818d7cd2))
* surface token auth endpoint errors ([bed22a7](https://github.com/opus-domini/sentinel/commit/bed22a7b61f605a3d6a58927d270fed5213eb836))


### CI

* **deps:** bump github-actions to latest ([770a2e9](https://github.com/opus-domini/sentinel/commit/770a2e9c6a0fd02bbf47218a3542e41782e8bdcc))

## [0.9.1](https://github.com/opus-domini/sentinel/compare/v0.9.0...v0.9.1) (2026-06-03)


### Bug Fixes

* **security:** bump Go to 1.26.4 to patch two stdlib CVEs ([c0e9a6b](https://github.com/opus-domini/sentinel/commit/c0e9a6b5f8ceb4471f01f6a4b010b39b9d32f92c))
* **tmux:** persist open session tabs across a PWA close/reopen ([ed52d94](https://github.com/opus-domini/sentinel/commit/ed52d94abbbde215203735a6e00bd3ca64d90bfb))

## [0.9.0](https://github.com/opus-domini/sentinel/compare/v0.8.3...v0.9.0) (2026-06-02)


### ⚠ BREAKING CHANGES

* alerts, guardrails, tmux timeline/markers, the global activity feed, related storage tables/migrations, and alert webhook settings are removed. Existing local databases/configs with legacy tables or [alerts] config should be reset or cleaned; run/restart Sentinel after migration, and use sentinel db reset if you can discard local operational history.

### Features

* hard-cut alerts, guardrails, timeline, and activity feed ([55458a2](https://github.com/opus-domini/sentinel/commit/55458a2d313986c9d67dc46bd0c060a5ca04869f))


### Bug Fixes

* **alerts:** drop the full-height bordered box from the empty state ([2e3c493](https://github.com/opus-domini/sentinel/commit/2e3c4939f8d2c0cfbea7c93267cce6dd36252b43))
* **api,store:** make runbook approval transition atomic ([45a8d09](https://github.com/opus-domini/sentinel/commit/45a8d093f202c296c8324f8c6363253365054ff4))
* **api,validate:** block tmux flag injection and probe with request context ([76ce189](https://github.com/opus-domini/sentinel/commit/76ce189ef6df7a236b3fb264834c1abd982c72cf))
* **api:** deliver alert.created webhooks for service-action alerts ([106aa2e](https://github.com/opus-domini/sentinel/commit/106aa2ece82c63089da2a1a8b82b3134c20fbdc3))
* **api:** don't clobber session owner when preset session already exists ([5d700bc](https://github.com/opus-domini/sentinel/commit/5d700bc60c334ea6486a72b771295704a318deb4))
* **api:** serialize config-file writes under a dedicated mutex ([1336c04](https://github.com/opus-domini/sentinel/commit/1336c04bba9e31127858a313b02a0e447617021a))
* **cli:** redact webhook URLs in config show output ([40203d8](https://github.com/opus-domini/sentinel/commit/40203d8dcc1e06bc3d1a69549ae37e6e4effe845))
* **events:** deliver and close subscriber channels under one lock ([5067647](https://github.com/opus-domini/sentinel/commit/5067647661d5707f9153b95a9c8470ba1e5fb583))
* **mobile:** collapse the bottom-nav gap when the keyboard opens ([524dbb3](https://github.com/opus-domini/sentinel/commit/524dbb34eb629b9251f1ffa14e01da407cd82aee))
* **mobile:** flatten bottom navigation ([c5136dd](https://github.com/opus-domini/sentinel/commit/c5136ddd7c733fd9ac66c3cf54a7b6bf8facad84))
* **mobile:** full-width the orphaned runbook summary card ([8d4a905](https://github.com/opus-domini/sentinel/commit/8d4a905428206431434d23ea3313fe4da2914c4e))
* **mobile:** let the runbook detail scroll and reveal it on select ([4a455de](https://github.com/opus-domini/sentinel/commit/4a455decf30bd599cf07ec29ebf7cdcee583f765))
* **mobile:** open sidebar from active bottom nav item ([8e2e1da](https://github.com/opus-domini/sentinel/commit/8e2e1dae3540abaf367e83649abf91c9363f4c91))
* **mobile:** tighten the services screen so more units fit ([cb7aa34](https://github.com/opus-domini/sentinel/commit/cb7aa347f83150573b4c8d91d35388f6025d39b7))
* **nav:** de-duplicate mobile navigation and stop truncating Activities ([dfbdfa2](https://github.com/opus-domini/sentinel/commit/dfbdfa21aa04f4ee4ab5ad1ef39cfa0f99893d43))
* **runbook,guardrails:** enforce guardrails on runbook execution ([9004706](https://github.com/opus-domini/sentinel/commit/9004706c0613f0d1805b5a2df075d7ad21f97a34))
* **runbook:** per-attempt step timeout, preserve results on resume, no orphan runs ([e3f6bfd](https://github.com/opus-domini/sentinel/commit/e3f6bfd20f1eae4a4bdb92a1cb4914dee4e0b715))
* **runbooks:** stack the operations summary into two columns on mobile ([b6e1b3c](https://github.com/opus-domini/sentinel/commit/b6e1b3cf81c342b1a153f14e557b25f5b825644f))
* **scheduler,report,security:** harden shutdown races and nil guards ([479cd27](https://github.com/opus-domini/sentinel/commit/479cd27320b4a8d0f702badcfb034b894999fc22))
* **scheduler:** advance next_run_at before creating the run ([022c45a](https://github.com/opus-domini/sentinel/commit/022c45a037fc2199bf8f2cee1c32886905bdfe6c))
* **scheduler:** on run completion update only last-run, not next_run_at ([5319c35](https://github.com/opus-domini/sentinel/commit/5319c35d42f5f1644c47cbb8dc90c6ceca42c229))
* **scheduler:** prevent overlapping runs of the same schedule ([d04116b](https://github.com/opus-domini/sentinel/commit/d04116b529111910536189b209cfabefe0b95087))
* **scheduler:** substitute runbook parameters on scheduled runs ([a1849d7](https://github.com/opus-domini/sentinel/commit/a1849d74e0d05225a54a57c8536c40e7a24b33b2))
* **security:** compare Origin host case-insensitively in CheckOrigin ([0f0a1fa](https://github.com/opus-domini/sentinel/commit/0f0a1fa243d91f0b27e9c7655c16145e6a0306de))
* **server,ui:** recover panics in request and WS/PTY goroutines ([77b2c9f](https://github.com/opus-domini/sentinel/commit/77b2c9f941687478d5cdd93c3b90e3a7591247e9))
* **services:** collapse mobile unit actions into a primary + overflow menu ([ccbf879](https://github.com/opus-domini/sentinel/commit/ccbf8793efe8a74103f1ca6c9eab094ac07fc18a))
* **services:** honor context in darwin metric collectors ([7defc56](https://github.com/opus-domini/sentinel/commit/7defc56a8ab2e6fb65d5eb5b800ef7ae3c37f139))
* **services:** sample CPU outside the metrics lock ([13b5ddd](https://github.com/opus-domini/sentinel/commit/13b5ddd65b3bdbf8f37e08fe25f33e4f3a32d765))
* **store:** clamp watchtower seen_revision so mark-seen is not lost ([fdca054](https://github.com/opus-domini/sentinel/commit/fdca0548bc0ffe23ebfbb08cd5b0111898282b49))
* **store:** close managed-window cursor on scan error ([09a1c4e](https://github.com/opus-domini/sentinel/commit/09a1c4e1701026668e5a98dbabeed86253ed5059))
* **store:** exclude already-resolved alerts from BulkAck re-fetch ([f022bb5](https://github.com/opus-domini/sentinel/commit/f022bb53d385de387b03c020de3a96395846a9d3))
* **terminal:** don't hijack typing in inputs outside the terminal ([8f23638](https://github.com/opus-domini/sentinel/commit/8f2363870faa8bb616db7a370d7496f7da2a9298))
* **tmux,runbooks:** mobile-aware empty states and idle terminal controls ([4d3ab3b](https://github.com/opus-domini/sentinel/commit/4d3ab3b42e77219cec3fccdd3912b847ac2375d0))
* **tmux:** propagate context cancellation through classifyError and CapturePane ([cedd041](https://github.com/opus-domini/sentinel/commit/cedd041288fde3263b7a97c603ceb2a2fc78322f))
* **tmux:** surface session activity in collapsed tabs ([ee4d440](https://github.com/opus-domini/sentinel/commit/ee4d4401785310ae0d9ead026683d22f503c1bd6))
* **ui:** add baseline security headers to SPA and asset responses ([57a94d4](https://github.com/opus-domini/sentinel/commit/57a94d43583a85b80566f5db2ce68b918d951ff3))
* **updater:** fsync binaries and use exclusive open for durability/TOCTOU ([3280b57](https://github.com/opus-domini/sentinel/commit/3280b57a2b376d691bbcadd138198b6880ee18e4))
* **ws,guardrails,daemon:** close broken WS writes, normalize rule mode, harden unit handling ([6e52ac1](https://github.com/opus-domini/sentinel/commit/6e52ac13f29c9cbf66ad6392809bcab91a8fa168))
* **ws:** bound idle and slow peers with read/write deadlines ([f729245](https://github.com/opus-domini/sentinel/commit/f729245707a514b79f25b7139a43d7bb196333dc))


### Performance

* **logs:** virtualize the LogViewer line list ([e30c729](https://github.com/opus-domini/sentinel/commit/e30c729ee64ad7b89d085c4aa5cf1805eee58b10))
* **watchtower:** batch per-tick metric writes into one transaction ([8e4a844](https://github.com/opus-domini/sentinel/commit/8e4a8442bd9bf9e4f9226e4310b496b36a43cd59))

## [0.8.3](https://github.com/opus-domini/sentinel/compare/v0.8.2...v0.8.3) (2026-06-01)

### Bug Fixes

- harden services and server config ([98f1225](https://github.com/opus-domini/sentinel/commit/98f12250fe9936b87354dea5d5d497c40e7f65b5))
- improve frontend accessibility and recovery UX ([113c2bc](https://github.com/opus-domini/sentinel/commit/113c2bce93719becc234927a312cab0941642e5a))
- stabilize ops streams and service actions ([efaf796](https://github.com/opus-domini/sentinel/commit/efaf796ae76383c3648aaf34647ca0ac6414f681))
- stabilize tmux state and runbook polling ([c18c31b](https://github.com/opus-domini/sentinel/commit/c18c31bb608e38ee9833f6ca6e967435e6201b6a))

## [0.8.2](https://github.com/opus-domini/sentinel/compare/v0.8.1...v0.8.2) (2026-05-31)

### Refactors

- **cli:** standardize command groups ([272fdee](https://github.com/opus-domini/sentinel/commit/272fdee5131c6101b92ca9892a58e552b8fea40c))

## [0.8.1](https://github.com/opus-domini/sentinel/compare/v0.8.0...v0.8.1) (2026-05-29)

### Bug Fixes

- address cli lint findings ([5b82fb3](https://github.com/opus-domini/sentinel/commit/5b82fb3e0747195d7c624b43c4ad2d78f64b5d1e))

### CI

- split fast and full workflow gates ([2bc6bb3](https://github.com/opus-domini/sentinel/commit/2bc6bb3556de38103efde98aef0c5b1d59682111))

## [0.8.0](https://github.com/opus-domini/sentinel/compare/v0.7.0...v0.8.0) (2026-05-27)

### ⚠ BREAKING CHANGES

- update apply no longer supports the legacy --systemd-scope alias; use --scope auto, --scope user, or --scope system.

### Features

- restart on update apply by default ([4f6ea9f](https://github.com/opus-domini/sentinel/commit/4f6ea9f24c7f4e3571c690236fdd45c5c8623746))

## [0.7.0](https://github.com/opus-domini/sentinel/compare/v0.6.4...v0.7.0) (2026-05-27)

### ⚠ BREAKING CHANGES

- Sentinel config now uses sectioned keys such as server.host, server.port, storage.path and log.path. Legacy flat keys like listen, token, timezone, locale and log_level are no longer accepted.

### Features

- restructure sentinel configuration ([3986179](https://github.com/opus-domini/sentinel/commit/39861794fbd43833f0944e5058ce3d1ef8525d3c))

## [0.6.4](https://github.com/opus-domini/sentinel/compare/v0.6.3...v0.6.4) (2026-05-27)

### Bug Fixes

- redact config show token ([ee122d4](https://github.com/opus-domini/sentinel/commit/ee122d4be35044696f8b0173d886c881f56f99b5))

## [0.6.3](https://github.com/opus-domini/sentinel/compare/v0.6.2...v0.6.3) (2026-05-27)

### Features

- add config management commands ([42a4138](https://github.com/opus-domini/sentinel/commit/42a413853601a96dad79f4fa03484f2dd5b27ad9))
- add config show command ([1767fbb](https://github.com/opus-domini/sentinel/commit/1767fbb19faefbf54078499953206ba6ed10ddff))
- add db setup commands ([6da5e07](https://github.com/opus-domini/sentinel/commit/6da5e07e4d91bca8c0fd9cff790c8f6de2f59fd1))

### Bug Fixes

- avoid false session create timeouts ([5687984](https://github.com/opus-domini/sentinel/commit/5687984af9c74f80fa67a9fe4c16551f6fa5c31b))

### CI

- retrigger codeql ([d9dcecb](https://github.com/opus-domini/sentinel/commit/d9dcecbd497e22fe34b977d5bb593dc7d3db1afb))
- run canonical make ci workflow ([e2d5721](https://github.com/opus-domini/sentinel/commit/e2d5721b683eaf061926d4e3facba2cca254d15e))

## [0.6.2](https://github.com/opus-domini/sentinel/compare/v0.6.1...v0.6.2) (2026-05-22)

### Bug Fixes

- **frontend:** load bundled font assets correctly ([69c2a74](https://github.com/opus-domini/sentinel/commit/69c2a74aa8c0df7fb459a5a14a4687c89a7ad75c))

### Refactors

- move embedded ui into internal ui package ([77eafca](https://github.com/opus-domini/sentinel/commit/77eafca4de90c500c7d0e7b70459a57cf33bd830))

## [0.6.1](https://github.com/opus-domini/sentinel/compare/v0.6.0...v0.6.1) (2026-05-21)

### Bug Fixes

- detect the installed service scope instead of assuming it ([e2c29c4](https://github.com/opus-domini/sentinel/commit/e2c29c421e2435e4962a3b8994f70099ac7031e7))

## [0.6.0](https://github.com/opus-domini/sentinel/compare/v0.5.38...v0.6.0) (2026-05-21)

### ⚠ BREAKING CHANGES

- `sentinel serve` is removed — use `sentinel daemon`. Service units installed by an earlier version run `sentinel` with no arguments, which no longer starts the server; regenerate them with `sentinel service install` after upgrading.

### Features

- add service lifecycle commands ([58e7e8f](https://github.com/opus-domini/sentinel/commit/58e7e8f31d623b5c58d6766b81ef4b3aeafceafa))
- replace serve with explicit daemon command ([95725f6](https://github.com/opus-domini/sentinel/commit/95725f67bf79682c78e67634226e93ba866dc7d1))
- unify CLI help output across all subcommands ([3f6dee4](https://github.com/opus-domini/sentinel/commit/3f6dee475bfc82c4130629cbecf2cd5d9e4dffbb))

### Refactors

- extract reusable tick loop in internal/server ([070fab6](https://github.com/opus-domini/sentinel/commit/070fab6d8020db5267e7599243842a017a29f5d7))
- rebuild the CLI on spf13/cobra ([53e0f59](https://github.com/opus-domini/sentinel/commit/53e0f59fb6d803992e7ac12e52bb50dd24b17c90))
- split cmd/sentinel into internal/cli and internal/server ([7b29a4c](https://github.com/opus-domini/sentinel/commit/7b29a4c0085c859c051ecbbad76004f04990a6a4))

## [0.5.38](https://github.com/opus-domini/sentinel/compare/v0.5.37...v0.5.38) (2026-05-21)

### Bug Fixes

- harden the autoupdate service ([bfac050](https://github.com/opus-domini/sentinel/commit/bfac050cd6bf1d7fdb1ddc8f91acc108dcb55fe2))

### Refactors

- rename client/ directory to frontend/ ([aa9f18c](https://github.com/opus-domini/sentinel/commit/aa9f18c60f171205de6540aff891db8a90eb910e))
- replace uninstall.sh with `sentinel service uninstall --purge` ([682db87](https://github.com/opus-domini/sentinel/commit/682db875e74ec13c21fa329fe708e5f77551dd0e))

### Build

- **deps:** bump frontend dependencies ([7280c52](https://github.com/opus-domini/sentinel/commit/7280c5296e170ef47fdcbc05c021cbc5c0b3f9fa))

### CI

- sign release artifacts with keyless cosign ([df3e032](https://github.com/opus-domini/sentinel/commit/df3e032af4b282b7b3bcd1f0c7b581db43cb85a2))

## [0.5.37](https://github.com/opus-domini/sentinel/compare/v0.5.36...v0.5.37) (2026-05-20)

### Features

- add shell completion and group root help by section ([c59a63c](https://github.com/opus-domini/sentinel/commit/c59a63cb3ab21c8a8ee4527674443f9f7747b3ba))

### Refactors

- import CreateSessionDialog statically in tmux route ([bef7dc8](https://github.com/opus-domini/sentinel/commit/bef7dc86797e85af88b0c9fce16bb0267119e097))

### CI

- keep client lockfile version in sync via release-please ([04e7486](https://github.com/opus-domini/sentinel/commit/04e74860872e25eb463385bc755151bcaa310a56))

## [0.5.36](https://github.com/opus-domini/sentinel/compare/v0.5.35...v0.5.36) (2026-05-20)

### Features

- add `sentinel service logs` command ([67c6ec2](https://github.com/opus-domini/sentinel/commit/67c6ec2fb5e1e48ec3db40685ce16f7e60455341))

### Bug Fixes

- kill session when its last tmux window closes ([db44474](https://github.com/opus-domini/sentinel/commit/db444743f3714ebbdf6f3c8d8dc416a05191f18f))

### Refactors

- extract repeated string literals into named constants ([2cb610f](https://github.com/opus-domini/sentinel/commit/2cb610fdad6540f20f17258742e6e449fbe090ef))

### Build

- **client:** update frontend dependencies ([d0da693](https://github.com/opus-domini/sentinel/commit/d0da693e6b00470d2a69f4481f3a0724d4780f15))
- delegate Makefile install to `sentinel service` ([205a17c](https://github.com/opus-domini/sentinel/commit/205a17c1f1d74cc31534c1cb0399ef7abf2ea400))

### CI

- adopt GoReleaser for release packaging ([ed27f68](https://github.com/opus-domini/sentinel/commit/ed27f684c347872cf03b2e1adfcd67515803ed46))
- upgrade golangci-lint to v2.12.2 ([9cab643](https://github.com/opus-domini/sentinel/commit/9cab643524456f86054e48478b62a74e270e2ff2))

## [0.5.35](https://github.com/opus-domini/sentinel/compare/v0.5.34...v0.5.35) (2026-05-14)

### Refactors

- consolidate ops resync controls ([2109659](https://github.com/opus-domini/sentinel/commit/2109659e4379107615e20b661aaa4d1939d66753))

## [0.5.34](https://github.com/opus-domini/sentinel/compare/v0.5.33...v0.5.34) (2026-05-14)

### Features

- add tmux window hotkeys ([7ff865c](https://github.com/opus-domini/sentinel/commit/7ff865c8bb76284f7f002f31efe14a4cb4eaac80))
- improve metrics dashboard sampling ([20f2e4b](https://github.com/opus-domini/sentinel/commit/20f2e4b3059141aaf2ea1baafe379b97e3e03f68))

### Bug Fixes

- align tmux tab activity states ([2c1ef36](https://github.com/opus-domini/sentinel/commit/2c1ef3648b589729cad56eb1bb3bb86569e6c197))

## [0.5.33](https://github.com/opus-domini/sentinel/compare/v0.5.32...v0.5.33) (2026-05-13)

### Bug Fixes

- refine sentinel pwa icon ([675a16a](https://github.com/opus-domini/sentinel/commit/675a16abf2efa9f5cc2a2efe3323324fb3eeb2c8))
- remove broad focus rings from scroll containers ([c9d0391](https://github.com/opus-domini/sentinel/commit/c9d03913e2ab931d30f1cf11aedb0172e95cf65b))
- stabilize terminal rendering and shortcuts ([7c91636](https://github.com/opus-domini/sentinel/commit/7c91636a2ce22b3a09f895259efa940f9f3314d9))
- standardize pwa icons ([2e469be](https://github.com/opus-domini/sentinel/commit/2e469be72ea2b186018b94d943a0a5bbedff98ec))

## [0.5.32](https://github.com/opus-domini/sentinel/compare/v0.5.31...v0.5.32) (2026-05-01)

### Features

- improve runbook approval workflow ([5ea7380](https://github.com/opus-domini/sentinel/commit/5ea7380224b569594eb93acd7d7897d20a45e687))
- improve runbooks operations UX ([3fd05df](https://github.com/opus-domini/sentinel/commit/3fd05dfb5f6cab0b534988aa355f43c4ac41a957))
- improve services operations ux ([bc64d6f](https://github.com/opus-domini/sentinel/commit/bc64d6f3a7f89bc817dfc900b87f616cc0314c74))

### Bug Fixes

- satisfy runbook lint constants ([c4b769f](https://github.com/opus-domini/sentinel/commit/c4b769fef8474e6b0f7a1361f2d2822814c982f1))

## [0.5.31](https://github.com/opus-domini/sentinel/compare/v0.5.30...v0.5.31) (2026-05-01)

### Features

- decouple session launchers from pinned sessions ([4d17ef8](https://github.com/opus-domini/sentinel/commit/4d17ef883b7323d51c3b99f4ce5644cf54558657))

### Bug Fixes

- keep systemd-run tmux sessions alive ([9722176](https://github.com/opus-domini/sentinel/commit/9722176b60eb3aedbb326811acaf5f44ef0038da))
- remove pinned state on session kill ([4cf952b](https://github.com/opus-domini/sentinel/commit/4cf952b97b69e8b686675246b5dfda229e4941cc))

## [0.5.30](https://github.com/opus-domini/sentinel/compare/v0.5.29...v0.5.30) (2026-05-01)

### Features

- use systemd-run for multi-user tmux ([1b4e701](https://github.com/opus-domini/sentinel/commit/1b4e7019245c29d205f5333bf9f71e328d1ea687))

### Bug Fixes

- harden tmux terminal frontend ([8f4600e](https://github.com/opus-domini/sentinel/commit/8f4600e71e4dfc0eeaddb9930d1ecc138010c635))
- harden tmux terminal rendering ([b304601](https://github.com/opus-domini/sentinel/commit/b3046016b38cf99c2349289e185e390db7637bc4))

### Refactors

- polish frontend accessibility and services ([9074f5c](https://github.com/opus-domini/sentinel/commit/9074f5cccd55b8faa127084d7a8d6168139f6e1f))
- split tmux handlers and improve coverage ([6927cc8](https://github.com/opus-domini/sentinel/commit/6927cc8a815ba5042c8b63168f0bd96474d00ec3))

## [0.5.29](https://github.com/opus-domini/sentinel/compare/v0.5.28...v0.5.29) (2026-04-23)

### Features

- **ui:** add session launchers and compact sidebar header actions ([4b216bd](https://github.com/opus-domini/sentinel/commit/4b216bd67e0aa7eb199dcafaa137447e024527d5))

### Bug Fixes

- satisfy watchtower lint checks ([136ca23](https://github.com/opus-domini/sentinel/commit/136ca2385aaafc20c2db5368784888cd3f0c6bc9))

### Documentation

- refresh tmux launcher guides ([01749fe](https://github.com/opus-domini/sentinel/commit/01749fe870671aaad3fa63f051bf8bec01d8864b))

## [0.5.28](https://github.com/opus-domini/sentinel/compare/v0.5.27...v0.5.28) (2026-04-20)

### Features

- make PWA Update App button actively probe and activate a new worker ([356e00d](https://github.com/opus-domini/sentinel/commit/356e00d2afb55e05ee8b8d8bb2c28d188c70be7c))

### Bug Fixes

- align xterm column widths with tmux via unicode-graphemes addon ([03461ff](https://github.com/opus-domini/sentinel/commit/03461ff8e50b318227d84db50afef58e35e5cb0f))
- clear WebGL atlas on growth to avoid black glyphs in emoji-heavy sessions ([dfc8467](https://github.com/opus-domini/sentinel/commit/dfc8467c23530a0884bcf3bade0959ce11fe971d))
- dispose WebGL addon on context loss to prevent black glyphs ([6d2d7ee](https://github.com/opus-domini/sentinel/commit/6d2d7eeef9d4180391bad5f2542b827dd10911a7))
- enable allowProposedApi so the unicode-graphemes addon can activate ([90b9ddf](https://github.com/opus-domini/sentinel/commit/90b9ddf1466828dba7efcc41c2d7842600281983))
- fall back to client/public for the PWA manifest so go tests work without a build ([c756be6](https://github.com/opus-domini/sentinel/commit/c756be6307181fa4ce7253a61d09e9c14ae41f89))
- restore mobile sidebar scroll by passing flex chain through FocusScope ([5a2b422](https://github.com/opus-domini/sentinel/commit/5a2b422efba193dbb776a11d94cbfb82fb0bf140))

## [0.5.27](https://github.com/opus-domini/sentinel/compare/v0.5.26...v0.5.27) (2026-04-12)

### Features

- add uninstall.sh script ([15b424a](https://github.com/opus-domini/sentinel/commit/15b424af587233d59e7a96dab5117a692528254f))

### Bug Fixes

- prevent needrestart from bouncing sentinel during unattended upgrades ([aac1a99](https://github.com/opus-domini/sentinel/commit/aac1a9989e4d9363b76dc423cac9ad9c8dbec654))

## [0.5.26](https://github.com/opus-domini/sentinel/compare/v0.5.25...v0.5.26) (2026-04-11)

### Bug Fixes

- disable HTTP keep-alive to avoid cross-port socket pool contention ([6d26e7d](https://github.com/opus-domini/sentinel/commit/6d26e7d5bbc5b97e6d4b4aefc7728edadbf5049d))
- harden tmux websocket recovery ([6784112](https://github.com/opus-domini/sentinel/commit/67841123c237e46cd5d20d9ddf5817707e3f335a))

## [0.5.25](https://github.com/opus-domini/sentinel/compare/v0.5.24...v0.5.25) (2026-04-10)

### Bug Fixes

- eliminate WebSocket socket pool starvation in Chrome ([c901b9a](https://github.com/opus-domini/sentinel/commit/c901b9a5aa34f1205cd19c7936b96a650a72f41d))
- fall back to default tmux server in WS attach handler ([23be911](https://github.com/opus-domini/sentinel/commit/23be911eda24ca808fb9c7e8276db83b86408895))
- stabilize refreshSessions callback passed to useInspector ([daa15bc](https://github.com/opus-domini/sentinel/commit/daa15bce2098360a7eaf3b8b25dcc584372dc7e2))
- UI polish and lazy terminal connection improvements ([1c579b2](https://github.com/opus-domini/sentinel/commit/1c579b28f4bd0928fd5ace7da24f8996bbd4ac08))

## [0.5.24](https://github.com/opus-domini/sentinel/compare/v0.5.23...v0.5.24) (2026-04-10)

### Bug Fixes

- build frontend before go test -race in ci-full ([bb75c3d](https://github.com/opus-domini/sentinel/commit/bb75c3d96814863e3c2e5eb9766baea1184c0249))
- decouple WebSocket reconnect from rendering triggers ([f5184db](https://github.com/opus-domini/sentinel/commit/f5184db92e2f34c6d81f69953926db3a354b506b))

## [0.5.23](https://github.com/opus-domini/sentinel/compare/v0.5.22...v0.5.23) (2026-04-09)

### Features

- brand the app with the host name ([adf997f](https://github.com/opus-domini/sentinel/commit/adf997fdfc65a3c4c3169cadd9c2f3e66320848e))

### Bug Fixes

- align manifest tests with lint rules ([b0ec963](https://github.com/opus-domini/sentinel/commit/b0ec963ecad2776b036a438434431364ef4f99c8))
- avoid websocket handshake self-aborts ([c78a710](https://github.com/opus-domini/sentinel/commit/c78a71009865cf104aaac28c81dcd69de40a5ff9))
- correlate tmux create operations with socket acks ([52c0222](https://github.com/opus-domini/sentinel/commit/52c0222b449b7ce1cbecda92e3fa445c86884dd7))
- harden xterm texture atlas refreshes ([ea8a978](https://github.com/opus-domini/sentinel/commit/ea8a9786152cb7f6ff62b2dcbaa57b33747335a9))
- isolate concurrent tmux web clients ([0e793b8](https://github.com/opus-domini/sentinel/commit/0e793b85e114bb7403ce04a85c85c130be8e3739))
- restore pinned sessions with configured user ([3322342](https://github.com/opus-domini/sentinel/commit/33223427a0f9e680fc0fdb65dd157d9911de38d3))
- satisfy httpui manifest lint ([b2efb7d](https://github.com/opus-domini/sentinel/commit/b2efb7dd2cbf116faf21845ce5ff3f946c276b59))
- stabilize tmux terminal attach UX ([5c5e2f2](https://github.com/opus-domini/sentinel/commit/5c5e2f28e1880cbff3300bd45d902dd15d24e81a))

## [0.5.22](https://github.com/opus-domini/sentinel/compare/v0.5.21...v0.5.22) (2026-04-07)

### Bug Fixes

- clear WebGL texture atlas periodically to prevent glyph corruption ([a1ceb1e](https://github.com/opus-domini/sentinel/commit/a1ceb1ecd9375082476b8cc4ca5d5aa295e59422))
- refresh sidebar on new sessions and persist auth cookie ([e5a08c3](https://github.com/opus-domini/sentinel/commit/e5a08c3efb8198644d32f632765060aa711cc39a))
- remove session preset on kill to prevent ghost sidebar entry ([29f43e9](https://github.com/opus-domini/sentinel/commit/29f43e955cb86a5267f75e8e9735d9efb2500253))
- resolve staticcheck SA5011 nil-pointer warnings in schedule tests ([36e96ae](https://github.com/opus-domini/sentinel/commit/36e96ae8333aa30f1505a5449fe57cd37652a09b))

## [0.5.21](https://github.com/opus-domini/sentinel/compare/v0.5.20...v0.5.21) (2026-04-03)

### Bug Fixes

- add StartTmuxAttachAsUser to darwin PTY layer ([3045cb5](https://github.com/opus-domini/sentinel/commit/3045cb5e85ce5b9e153e3c17622926783b106997))

### Documentation

- comprehensive documentation update for multi-user sessions ([ef6d79b](https://github.com/opus-domini/sentinel/commit/ef6d79b500fab2cea0c467d1e8b167ef32d7bb03))

## [0.5.20](https://github.com/opus-domini/sentinel/compare/v0.5.19...v0.5.20) (2026-04-02)

### Features

- auto-suffix session names on collision + fix Select empty value ([52e23ff](https://github.com/opus-domini/sentinel/commit/52e23ff3ef90ce13712d761f283b615fbf2e244a))
- load system users at startup as source of truth ([40fcd2c](https://github.com/opus-domini/sentinel/commit/40fcd2cf158800a01ced88d48ba33deb891febe3))
- move unread indicator from window badge to session icon ([b05d03b](https://github.com/opus-domini/sentinel/commit/b05d03b74bfe6c537deaef3af4f8c1af0596fac0))
- multi-user tmux session support ([a685f05](https://github.com/opus-domini/sentinel/commit/a685f05dbe1ab0f76ab577de070f239b7ac85f29))
- replace free-text user input with Select dropdown ([eb67399](https://github.com/opus-domini/sentinel/commit/eb6739973fa2763d05917441e356e2bf92987893))
- replace free-text user input with Select dropdown ([8dd088f](https://github.com/opus-domini/sentinel/commit/8dd088f62e20680a1e413cfddbecbc234dc93cb3))
- rich tooltip on session icon with details ([b75aa63](https://github.com/opus-domini/sentinel/commit/b75aa6312db8cf1f77a6bf662008f2ddd3b902ea))

### Bug Fixes

- include userMode/userValue in launcher normalization ([4ec4b12](https://github.com/opus-domini/sentinel/commit/4ec4b12fa33791b51d6dca0aceb606bbbbe08c57))
- suppress gosec G702 for validated sudo user argument ([628e7e7](https://github.com/opus-domini/sentinel/commit/628e7e70c86d7bef8a1c372e2a7dd01cc705e454))
- validate sudo user against /etc/passwd before execution ([cf645ee](https://github.com/opus-domini/sentinel/commit/cf645ee3d38589008f949e21afddac3d43625ae1))

## [0.5.19](https://github.com/opus-domini/sentinel/compare/v0.5.18...v0.5.19) (2026-04-02)

### Features

- 3-tier density for services sidebar cards ([c06baa8](https://github.com/opus-domini/sentinel/commit/c06baa8ab86029f6a102eaf21a789d71e6f90547))
- 3-tier session card density based on sidebar width ([0869b88](https://github.com/opus-domini/sentinel/commit/0869b88f09bde37a4104d44911e91afb634edec0))

### Bug Fixes

- adjust sidebar density thresholds ([7824116](https://github.com/opus-domini/sentinel/commit/782411683ac3080e7bd0d3ca9c741284bab0a64e))
- soften minimal density session card typography ([4c04c95](https://github.com/opus-domini/sentinel/commit/4c04c95d9a2248ad34c4a0e8c0633ba00454fafe))
- spread full density service card across cleaner lines ([06115df](https://github.com/opus-domini/sentinel/commit/06115df9fbed3e9d13ded155f41decd1867d7f2d))

## [0.5.18](https://github.com/opus-domini/sentinel/compare/v0.5.17...v0.5.18) (2026-04-02)

### Features

- accessibility foundation (WCAG 2.1 AA compliance) ([1aaa64f](https://github.com/opus-domini/sentinel/commit/1aaa64f912d595828c49c21b1f0e9b24df69e8ef))
- force WebSocket reconnection on resync and tab resume ([29184d1](https://github.com/opus-domini/sentinel/commit/29184d19195d6f3ef3c70c49b99740de8383e98d))
- interaction refinement — confirmations, submit guards, persist width ([7488ebb](https://github.com/opus-domini/sentinel/commit/7488ebbc673e6cfd44d380e254eecc615f5a221f))
- terminal and navigation polish ([49939c0](https://github.com/opus-domini/sentinel/commit/49939c0711e5a6e7195397ffd1db3bcacccb7b0f))

### Bug Fixes

- critical UX fixes for toasts, sidebar, viewport, and accessibility ([3a4ea65](https://github.com/opus-domini/sentinel/commit/3a4ea6579bee9845d1576426835fbc4f6cf44e2d))
- fix launchers dialog scroll on mobile and hide strip scrollbars ([bab523e](https://github.com/opus-domini/sentinel/commit/bab523ed65350b83673af720f567902882676b71))
- fix sidebar scroll on desktop and unify section headers ([5bd28af](https://github.com/opus-domini/sentinel/commit/5bd28af0a20f1f17463b741f42ea2028cc11f268))
- key managed tmux windows by runtime id ([1a8bd47](https://github.com/opus-domini/sentinel/commit/1a8bd4733d7e2edc62b1730fe5938188a0eb38b2))
- mobile polish — safe areas, touch, overscroll, tap targets ([29f4f75](https://github.com/opus-domini/sentinel/commit/29f4f75e9fc11c666ae6849992b06d2b1c395474))
- polish services sidebar labels and pinned service navigation ([1d54b6a](https://github.com/opus-domini/sentinel/commit/1d54b6a0c473ead31e874b36098f1dd6ca798e9b))
- prevent browse search reset when navigating to pinned service ([241b01a](https://github.com/opus-domini/sentinel/commit/241b01a3c555fa1444d4c749d56dac56f969aa1d))
- prevent stale debounce from resetting pinned service search ([160d540](https://github.com/opus-domini/sentinel/commit/160d540828b26567d457311f3a83f031abe8345a))
- remove redundant help and launchers buttons from terminal header ([7b635d6](https://github.com/opus-domini/sentinel/commit/7b635d6191384d28ad6acfd1e597a79e9db03fe3))

### Refactors

- visual consistency — semantic tokens, unified patterns ([8598340](https://github.com/opus-domini/sentinel/commit/85983403451feedf3c5d8f044ce222ece185a7d2))

## [0.5.17](https://github.com/opus-domini/sentinel/compare/v0.5.16...v0.5.17) (2026-04-01)

### Bug Fixes

- polish tmux window and session chips ([ff896c6](https://github.com/opus-domini/sentinel/commit/ff896c6a4792f9356d59c1b42691e55adf6d037d))

## [0.5.16](https://github.com/opus-domini/sentinel/compare/v0.5.15...v0.5.16) (2026-04-01)

### Features

- compact session cards before sidebar minimum width ([cb96888](https://github.com/opus-domini/sentinel/commit/cb96888f6cbd19fcdf37262b09fc316e2ec7a82f))
- streamline tmux header and strip interactions ([09a8d52](https://github.com/opus-domini/sentinel/commit/09a8d52ee239b069d914bfb259520df063f51254))

### Bug Fixes

- polish tmux terminal viewport chrome ([6e5262b](https://github.com/opus-domini/sentinel/commit/6e5262b42882515761a623553f78bd0665486b3b))
- track vitest wrapper script ([6ef5483](https://github.com/opus-domini/sentinel/commit/6ef5483939ac7ece04ff05684f8bad8d689a426d))

## [0.5.15](https://github.com/opus-domini/sentinel/compare/v0.5.14...v0.5.15) (2026-04-01)

### Refactors

- rename shared tmux icon catalog ([7f12911](https://github.com/opus-domini/sentinel/commit/7f12911c5099409a8f8b8ab0060d78d780d2f79a))

## [0.5.14](https://github.com/opus-domini/sentinel/compare/v0.5.13...v0.5.14) (2026-04-01)

### Features

- manage launcher windows and reorder tmux windows ([d2ba1b7](https://github.com/opus-domini/sentinel/commit/d2ba1b73174a02da34d1d1e33ddae477f6883bbc))

### Bug Fixes

- harden tmux strip payload parsing ([96f0beb](https://github.com/opus-domini/sentinel/commit/96f0beb4331f27e8bade1ef243d8e81b0ae42f5f))
- preserve tmux strip state across transient refresh failures ([ae9b657](https://github.com/opus-domini/sentinel/commit/ae9b65744bc78978dbf53f3b1654d117d69497c4))
- reorder page title hostname ([c9012ca](https://github.com/opus-domini/sentinel/commit/c9012ca432e2aa11159d82ea10d289f59d654d05))
- stabilize tmux window strip state ([ca53725](https://github.com/opus-domini/sentinel/commit/ca537255e41a95e3a8d3a1479af11da359450b81))
- tidy go sum after sqlite bump ([963f2fc](https://github.com/opus-domini/sentinel/commit/963f2fc5831ec2e0fd506ea7f8233ffb7318731d))

## [0.5.13](https://github.com/opus-domini/sentinel/compare/v0.5.12...v0.5.13) (2026-03-31)

### Bug Fixes

- use dropdown menu for launcher icon picker ([608733b](https://github.com/opus-domini/sentinel/commit/608733bb1cf49450be2043e7f791562d8fe8af22))

## [0.5.12](https://github.com/opus-domini/sentinel/compare/v0.5.11...v0.5.12) (2026-03-31)

### Features

- add tmux launchers ([144e42b](https://github.com/opus-domini/sentinel/commit/144e42b1eee73d86fcc47a3b741e35aaeed58b26))

## [0.5.11](https://github.com/opus-domini/sentinel/compare/v0.5.10...v0.5.11) (2026-03-30)

### Features

- add pinned tmux sessions and sidebar ordering ([33743db](https://github.com/opus-domini/sentinel/commit/33743db3a147536c837780cca16bd9dfc41172e4))

### Build

- **deps:** update client dependencies ([cbb86c8](https://github.com/opus-domini/sentinel/commit/cbb86c8a2c0bcb47493f5fcf96f00568843d7d2c))

## [0.5.10](https://github.com/opus-domini/sentinel/compare/v0.5.9...v0.5.10) (2026-03-25)

### Bug Fixes

- support shift in mobile tmux helper ([78fb660](https://github.com/opus-domini/sentinel/commit/78fb660a504f0b7438f1e2621a3f43591068c066))

## [0.5.9](https://github.com/opus-domini/sentinel/compare/v0.5.8...v0.5.9) (2026-03-23)

### Bug Fixes

- **ux:** resolve tmux window sync desync and add clickable resync badge ([0e816b9](https://github.com/opus-domini/sentinel/commit/0e816b938ce5692db7ade42ed80502d4329d0fcf))

## [0.5.8](https://github.com/opus-domini/sentinel/compare/v0.5.7...v0.5.8) (2026-03-22)

### Bug Fixes

- **ux:** increase PTY read buffer to reduce terminal flickering ([4536151](https://github.com/opus-domini/sentinel/commit/45361512935da00af1229e8d34728956bb9fb9ab))

## [0.5.7](https://github.com/opus-domini/sentinel/compare/v0.5.6...v0.5.7) (2026-03-13)

### Bug Fixes

- **security:** honor X-Forwarded-Proto in CORS origin check ([4263e28](https://github.com/opus-domini/sentinel/commit/4263e280c47c52314dc5721ef7fa13aec147bee4))

## [0.5.6](https://github.com/opus-domini/sentinel/compare/v0.5.5...v0.5.6) (2026-03-13)

### Features

- **ux:** improve mobile UX and add hostname to page title ([32fd35a](https://github.com/opus-domini/sentinel/commit/32fd35ac3b02bf5754c3a8ebafd0e73e024b4495))

## [0.5.5](https://github.com/opus-domini/sentinel/compare/v0.5.4...v0.5.5) (2026-03-12)

### Features

- **ux:** improve tmux consistency and service browsing ([7cd9e75](https://github.com/opus-domini/sentinel/commit/7cd9e75740f9570da4cbddb480a9f27b8ecc9c3c))

### Bug Fixes

- **tmux:** harden inspector consistency ([1ee400e](https://github.com/opus-domini/sentinel/commit/1ee400e0e6986959467469485ff21690ff903495))

## [0.5.4](https://github.com/opus-domini/sentinel/compare/v0.5.3...v0.5.4) (2026-03-11)

### Bug Fixes

- **ci:** satisfy staticcheck nil guards in tests ([b87524e](https://github.com/opus-domini/sentinel/commit/b87524e75de883a8ddb6c060d105210e82efdac2))
- **ci:** tidy go.sum ([3243a52](https://github.com/opus-domini/sentinel/commit/3243a522bf3f111b6db41701c99dbac0cc32f5ba))

### CI

- opt workflows into node 24 for javascript actions ([0d88ff9](https://github.com/opus-domini/sentinel/commit/0d88ff9a9959170a571b2919d8b23d829d2589db))

## [0.5.3](https://github.com/opus-domini/sentinel/compare/v0.5.2...v0.5.3) (2026-03-11)

### Bug Fixes

- **deps:** update dependencies and remove unused client tooling ([0244e8d](https://github.com/opus-domini/sentinel/commit/0244e8d5c83cdb86f6a4b0f2a5ee1d10363c49bf))

## [0.5.2](https://github.com/opus-domini/sentinel/compare/v0.5.1...v0.5.2) (2026-03-10)

### Features

- add enable/disable service actions and make pinning user-driven ([c1748ab](https://github.com/opus-domini/sentinel/commit/c1748abd4cc07b652ff4ab1fc73403d8af34bbb5))

## [0.5.1](https://github.com/opus-domini/sentinel/compare/v0.5.0...v0.5.1) (2026-03-09)

### Documentation

- update documentation for recent features and API changes ([92bffb3](https://github.com/opus-domini/sentinel/commit/92bffb320920db45ca6c1490486c5a8f386d6ebb))

## [0.5.0](https://github.com/opus-domini/sentinel/compare/v0.4.14...v0.5.0) (2026-03-09)

### ⚠ BREAKING CHANGES

- refactor runbook step types — run, script, approval (#steps)
- remove tmux session recovery feature

### Features

- interactive service metric cards with scope-aware counts ([c50ec20](https://github.com/opus-domini/sentinel/commit/c50ec20220a8f1a3e16ac176b7f9c125dfdd80b3))
- limit concurrent manual runbook runs with semaphore ([#1](https://github.com/opus-domini/sentinel/issues/1).4) ([4da038c](https://github.com/opus-domini/sentinel/commit/4da038cb585cc87721d0da95ab66d37118a61260))
- refactor runbook step types — run, script, approval (#steps) ([83dcadc](https://github.com/opus-domini/sentinel/commit/83dcadc98cd49767ce58812696246586155a0c59))
- remove tmux session recovery feature ([7f30112](https://github.com/opus-domini/sentinel/commit/7f3011210a692c1de679ba3a57b351d6aa7bbef4))
- runbook parameters UI — editor, run dialog, job history ([#5](https://github.com/opus-domini/sentinel/issues/5).3) ([6b7a8b4](https://github.com/opus-domini/sentinel/commit/6b7a8b415c0896888ad43d88c716071aa317bc59))
- runbook parameters with variable substitution ([#5](https://github.com/opus-domini/sentinel/issues/5).1, [#5](https://github.com/opus-domini/sentinel/issues/5).2) ([7b5b410](https://github.com/opus-domini/sentinel/commit/7b5b4107c90fd19cadc23db9e76422aaf3669f36))
- scheduled health reports via webhook ([#6](https://github.com/opus-domini/sentinel/issues/6).3) ([d9d93e9](https://github.com/opus-domini/sentinel/commit/d9d93e9c7fc46591bc595cb108d58fdffa0bf612))
- track and surface frequent directories in session creation ([2203999](https://github.com/opus-domini/sentinel/commit/2203999a15820e60d69dbec3436b8fa76f9f6c41))
- webhook settings UI in SettingsDialog ([#4](https://github.com/opus-domini/sentinel/issues/4).2) ([ee6630f](https://github.com/opus-domini/sentinel/commit/ee6630f67789ff09f3960cd6f85a1402fe8f2084))

### Bug Fixes

- add wt_presence index, ARIA dialog tabs, memoize inline sorts ([7395488](https://github.com/opus-domini/sentinel/commit/73954887e98ba690ebf5ad53a94601e05853d73b))
- gofmt formatting and remove unused time helpers ([b3ce05e](https://github.com/opus-domini/sentinel/commit/b3ce05ec3fa2df18f98adbc2ad8f9f53aba0428e))
- resolve all remaining lint warnings (noctx + gosec) ([6a7357e](https://github.com/opus-domini/sentinel/commit/6a7357e3b40706309794537200ef9652868c3b8a))
- resolve golangci-lint warnings from step refactor ([67c2cf5](https://github.com/opus-domini/sentinel/commit/67c2cf54834ccbbfab48af7f9bb527adb2584d21))

### Refactors

- extract runbooks page sub-components and hook ([654f485](https://github.com/opus-domini/sentinel/commit/654f4852b696481bd073cc0d02713d81a13a1465))
- extract services page sub-components ([7d192fa](https://github.com/opus-domini/sentinel/commit/7d192fac1002710bd817c0b22857da9467403e6d))
- migrate config to BurntSushi/toml parser ([#1](https://github.com/opus-domini/sentinel/issues/1).3) ([89b9110](https://github.com/opus-domini/sentinel/commit/89b911085a79bc48355db9edbbd5b79ef39fb5da))
- split store/watchtower.go into domain-specific files ([585daaa](https://github.com/opus-domini/sentinel/commit/585daaac8c9cbef20bbd98eab2556e71e620b189))

### Documentation

- document combobox pattern decision in CreateSessionDialog ([#2](https://github.com/opus-domini/sentinel/issues/2).6) ([7d0cf07](https://github.com/opus-domini/sentinel/commit/7d0cf072f56132c0613d537000c22d439cd76ea7))
- update roadmap with completed Marco 1 and 2 items ([30c3fd3](https://github.com/opus-domini/sentinel/commit/30c3fd3ed6958e7e09c9cde75dc4c1ebaab3a7cf))

## [0.4.14](https://github.com/opus-domini/sentinel/compare/v0.4.13...v0.4.14) (2026-03-06)

### Refactors

- replace ESLint + Prettier with oxlint + oxfmt ([#69](https://github.com/opus-domini/sentinel/issues/69)) ([3c466a2](https://github.com/opus-domini/sentinel/commit/3c466a2a325335f3cc7fec793845f3b7cf9901ec))

### Build

- **deps:** bump @tanstack/react-router and devtools to ^1.166.2 ([#67](https://github.com/opus-domini/sentinel/issues/67)) ([bc33e72](https://github.com/opus-domini/sentinel/commit/bc33e7285270d1e55b8d3296d1cf3ee28af82177))

## [0.4.13](https://github.com/opus-domini/sentinel/compare/v0.4.12...v0.4.13) (2026-03-05)

### Bug Fixes

- hide tmux status bar when attaching from web UI ([2d4de2a](https://github.com/opus-domini/sentinel/commit/2d4de2a12838acaac0216c71161ea0ed8f3a550b))

### Build

- **deps-dev:** bump @tanstack/router-plugin from 1.163.2 to 1.164.0 in /client ([7ba5abd](https://github.com/opus-domini/sentinel/commit/7ba5abdf690b953036cc37f089bbc628557026e4))
- **deps-dev:** bump @tanstack/router-plugin in /client ([ba6b7ca](https://github.com/opus-domini/sentinel/commit/ba6b7ca955978f893fb1063e824fcc64ce6426a1))
- **deps-dev:** bump @types/node from 25.3.0 to 25.3.3 in /client ([84b95f8](https://github.com/opus-domini/sentinel/commit/84b95f834ad0740d1fee6c7b4907a50cef082667))
- **deps-dev:** bump @types/node from 25.3.0 to 25.3.3 in /client ([4e72efa](https://github.com/opus-domini/sentinel/commit/4e72efac8563b4f21a72e87be3e3afdfbd4750c4))
- **deps:** bump actions/download-artifact from 7.0.0 to 8.0.0 ([37e67da](https://github.com/opus-domini/sentinel/commit/37e67dae1e1592db6359657cc02c739c4be62241))
- **deps:** bump actions/download-artifact from 7.0.0 to 8.0.0 ([b6c87ae](https://github.com/opus-domini/sentinel/commit/b6c87aed337314c4581dcc09b70872cc642cfe7b))
- **deps:** bump actions/setup-go from 6.2.0 to 6.3.0 ([f44d531](https://github.com/opus-domini/sentinel/commit/f44d53101d61807f9a2df1a4227330afd713b5e4))
- **deps:** bump actions/setup-go from 6.2.0 to 6.3.0 ([6d24e68](https://github.com/opus-domini/sentinel/commit/6d24e683757676fce4a392619d6d701042d25847))
- **deps:** bump actions/setup-node from 6.2.0 to 6.3.0 ([f13767f](https://github.com/opus-domini/sentinel/commit/f13767fec56bc091165a30f2e58206b10238190f))
- **deps:** bump actions/setup-node from 6.2.0 to 6.3.0 ([6f17374](https://github.com/opus-domini/sentinel/commit/6f17374fc4406b5c3d414fa100297e45ff34a619))
- **deps:** bump actions/upload-artifact from 6.0.0 to 7.0.0 ([9e4cf20](https://github.com/opus-domini/sentinel/commit/9e4cf2085647d8460f2744b149113470ac52a278))
- **deps:** bump actions/upload-artifact from 6.0.0 to 7.0.0 ([33cf2a3](https://github.com/opus-domini/sentinel/commit/33cf2a3c3a0db1d3db633ff6031e1776c5444348))
- **deps:** bump github.com/opus-domini/fast-shot from 1.3.0 to 1.3.1 ([023b761](https://github.com/opus-domini/sentinel/commit/023b7611be9c43cfb1b425d346cc53e1b6bd9eea))
- **deps:** bump github.com/opus-domini/fast-shot from 1.3.0 to 1.3.1 ([56d4655](https://github.com/opus-domini/sentinel/commit/56d465569f8c04bfdfc3beb76e926f9b2524dc11))
- **deps:** bump lucide-react from 0.575.0 to 0.576.0 in /client ([2f168b0](https://github.com/opus-domini/sentinel/commit/2f168b0d52a5c2e645c7833544152a218f92336a))
- **deps:** bump lucide-react from 0.575.0 to 0.576.0 in /client ([f7f046d](https://github.com/opus-domini/sentinel/commit/f7f046d4c33b79ddb0ce9ff7ff4764b8ef547905))

## [0.4.12](https://github.com/opus-domini/sentinel/compare/v0.4.11...v0.4.12) (2026-03-03)

### Bug Fixes

- forward touch coordinates and override wheelDeltaY in touchWheelBridge ([fef46d4](https://github.com/opus-domini/sentinel/commit/fef46d4f9d24dbdb3d5f5b45b7f4098837492b55))

## [0.4.11](https://github.com/opus-domini/sentinel/compare/v0.4.10...v0.4.11) (2026-03-03)

### Bug Fixes

- defer selectInFlightRef reset to server confirmation ([0f86d39](https://github.com/opus-domini/sentinel/commit/0f86d3907d488243be2def83ac4305e58d879452))
- emit inspector event from watchtower on active window change ([ba29da8](https://github.com/opus-domini/sentinel/commit/ba29da85899c03d7490effc8cd99f21c55ce60c0))
- invalidate stale refreshInspector on window/pane selection ([6be1880](https://github.com/opus-domini/sentinel/commit/6be1880153e05243675636af50e4f2b00dabe728))
- prevent window tab flicker on UI click with grace period ([268a660](https://github.com/opus-domini/sentinel/commit/268a6602806889a7c9e9a97ee721f614272cc9e4))
- resolve tab switching regression and prevent stuck loading state ([16d9599](https://github.com/opus-domini/sentinel/commit/16d959966c081d0329c56010a9973e340ad01d79))
- sync window tabs on terminal switch, hide session tab scrollbar, fix mobile scroll ([0164564](https://github.com/opus-domini/sentinel/commit/0164564c8e27f1e439fda4c5625fb5d7b26a642f))

## [0.4.10](https://github.com/opus-domini/sentinel/compare/v0.4.9...v0.4.10) (2026-02-25)

### Build

- **deps:** bump actions/setup-node from 4.4.0 to 6.2.0 ([c12dc97](https://github.com/opus-domini/sentinel/commit/c12dc97e0b1cf494219f225b18ae8a13b69e5adb))
- **deps:** bump github.com/opus-domini/fast-shot from 1.2.0 to 1.3.0 ([44d1429](https://github.com/opus-domini/sentinel/commit/44d14296a0adee854473a6839bd794641db097b5))

## [0.4.9](https://github.com/opus-domini/sentinel/compare/v0.4.8...v0.4.9) (2026-02-25)

### Bug Fixes

- correct paths in .gitignore for consistency ([1d1e75b](https://github.com/opus-domini/sentinel/commit/1d1e75b26a80169f49ef0f1d77c41d2b339fd3d8))

## [0.4.8](https://github.com/opus-domini/sentinel/compare/v0.4.7...v0.4.8) (2026-02-25)

### Features

- auto-reconnect tmux WebSocket on unexpected disconnect ([de8c6c5](https://github.com/opus-domini/sentinel/commit/de8c6c518744d8c1ddc4f4fdd2f230e0d82035ba))

## [0.4.7](https://github.com/opus-domini/sentinel/compare/v0.4.6...v0.4.7) (2026-02-24)

### Features

- add FUNDING.yml to support funding model platforms ([d211b68](https://github.com/opus-domini/sentinel/commit/d211b683ac126954179639858111c4f7c0120c8b))
- recovery stale-job cleanup, icon preservation, and toast fix ([7d906dd](https://github.com/opus-domini/sentinel/commit/7d906ddddcacdf0bc5c078de087cbbb6dca16117))

### Bug Fixes

- complete icon restore before marking recovery job succeeded ([1c08da5](https://github.com/opus-domini/sentinel/commit/1c08da56b7d08ac6d93972a1f1625b2cf9d21fe3))
- extract recoveryStaleReason constant to satisfy goconst ([02788f2](https://github.com/opus-domini/sentinel/commit/02788f2c70f69f214a5c5c1a82d29d65ae4e2047))

## [0.4.6](https://github.com/opus-domini/sentinel/compare/v0.4.5...v0.4.6) (2026-02-23)

### Features

- add internal database migrator with versioned SQL migrations ([1d2b44a](https://github.com/opus-domini/sentinel/commit/1d2b44ac195b95ddb602cf794c463b1ae29d0a7b))
- add timezone and locale settings with unified date formatting ([6ef1737](https://github.com/opus-domini/sentinel/commit/6ef1737bde47ab542fced054fc1418ccd57c8bfe))
- deterministic layout restore, scan helpers, and auto-restore on boot ([13076c8](https://github.com/opus-domini/sentinel/commit/13076c862832ec0dafde55c1d7a113ab4fc35d0a))

### Bug Fixes

- connection badge poisoned when switching sessions after create ([73f3bdc](https://github.com/opus-domini/sentinel/commit/73f3bdc8d9cf52025c05ab7b575c87b0195764f7))
- connection badge stuck on "Connecting" after session creation ([21ae665](https://github.com/opus-domini/sentinel/commit/21ae665e73cf6c1481c1f5ce5b3a38cd126c8a5c))

### Refactors

- replace google/uuid with stdlib crypto/rand for ID generation ([40fabc2](https://github.com/opus-domini/sentinel/commit/40fabc2f28e2deb8e6e34b2cc35893c03c698c8f))

### Documentation

- comprehensive audit and fix of all documentation ([2965459](https://github.com/opus-domini/sentinel/commit/29654599d6ffd5df3536ce2e1044a3dd12c87c47))

## [0.4.5](https://github.com/opus-domini/sentinel/compare/v0.4.4...v0.4.5) (2026-02-20)

### Features

- add webhook support to runbooks with step-level payload details ([5a559d5](https://github.com/opus-domini/sentinel/commit/5a559d5910a0eb67f09909f2ad00f017783a3603))

### Bug Fixes

- prevent daemon tests from killing host systemd services ([b44d733](https://github.com/opus-domini/sentinel/commit/b44d733c5b5f2176c50c7d1bc9f584c6a25f0f25))
- resolve lint failures from golangci-lint v2.10 upgrade ([5a15626](https://github.com/opus-domini/sentinel/commit/5a15626ea75e322fbc712554d5fe0706d113514a))

### CI

- bump golangci-lint to v2.10.1 for Go 1.26 compatibility ([4de0240](https://github.com/opus-domini/sentinel/commit/4de02402bf4dd5b0fee7684168e26b9ef74cc991))

### Documentation

- document runbook webhook feature and payload structure ([21b16ff](https://github.com/opus-domini/sentinel/commit/21b16ff4619068f177fc022539989d0cf3769b85))

## [0.4.4](https://github.com/opus-domini/sentinel/compare/v0.4.3...v0.4.4) (2026-02-20)

### Bug Fixes

- new tmux windows inherit session's working directory ([e16b50f](https://github.com/opus-domini/sentinel/commit/e16b50f567d58ef2435fa8a46d983c9c96f68961))
- open create session dialog directly from empty state button ([a0d29a8](https://github.com/opus-domini/sentinel/commit/a0d29a811299e44558074923c4f89549b2150d74))
- skip systemd user scope when running as root ([2f5481e](https://github.com/opus-domini/sentinel/commit/2f5481ebc310ddd1e12e0de3c8bc2b9e844e5ac2))

## [0.4.3](https://github.com/opus-domini/sentinel/compare/v0.4.2...v0.4.3) (2026-02-20)

### Features

- add server offline detection with retry banner ([cbdd7a9](https://github.com/opus-domini/sentinel/commit/cbdd7a9a9f72a3c8979a30f5b79fafe1846e8f19))

### Bug Fixes

- **httpui:** make embedded dist fs init race-safe ([6ce1ae1](https://github.com/opus-domini/sentinel/commit/6ce1ae1b12363778268608fec28a37a8b2024d50))
- make server offline detection robust across build contexts ([6584005](https://github.com/opus-domini/sentinel/commit/6584005df159a87f1284818a2740aa38872aa4b7))
- move offline detection to QueryCache callbacks ([480cfbf](https://github.com/opus-domini/sentinel/commit/480cfbfed7913f50962bb36f88abd2956b50ecc6))
- remove destructive daemon tests that kill running sentinel service ([bc4a660](https://github.com/opus-domini/sentinel/commit/bc4a660a7cf3b9ea93441d7a3b813a6702acdfad))
- use committed placeholder file in TestServeDistPath ([2a004e2](https://github.com/opus-domini/sentinel/commit/2a004e2d322a332a78fb52cbaa8225d2b31cc954))
- write step result to DB before execution, not after ([40722f6](https://github.com/opus-domini/sentinel/commit/40722f612e5bfe361928c5e590a1968ff6e1dfcb))

### CI

- add Codecov coverage upload to CI ([97e57f0](https://github.com/opus-domini/sentinel/commit/97e57f0bfc0c8d4815fd28765be02f42e5aac245))

## [0.4.2](https://github.com/opus-domini/sentinel/compare/v0.4.1...v0.4.2) (2026-02-20)

### Bug Fixes

- extract orphan error constant to satisfy goconst lint ([46be998](https://github.com/opus-domini/sentinel/commit/46be998903a2e771d3b9ddffcfc395779a3fca8a))
- reconcile orphaned runbook runs stuck in running/queued on startup ([7326125](https://github.com/opus-domini/sentinel/commit/7326125a0808cda0eff69648ad149f52e9f02cea))

## [0.4.1](https://github.com/opus-domini/sentinel/compare/v0.4.0...v0.4.1) (2026-02-20)

### Bug Fixes

- reconcile orphaned runbook runs stuck in running/queued on startup ([80f81b9](https://github.com/opus-domini/sentinel/commit/80f81b9a34f356f1f282398fc7953232a8ad52ba))

## [0.4.0](https://github.com/opus-domini/sentinel/compare/v0.3.12...v0.4.0) (2026-02-20)

### Bug Fixes

- add pointer cursor to dialog close button ([b6b63e1](https://github.com/opus-domini/sentinel/commit/b6b63e10d3d1f05de030d763f7a82acd9cebdf24))
- add pointer cursor to tmux header action buttons ([24f0df0](https://github.com/opus-domini/sentinel/commit/24f0df0a440a180bb4b1462894c25d15c29d3c73))
- position tmux dialogs at top of viewport instead of centered ([e244da4](https://github.com/opus-domini/sentinel/commit/e244da4554469eb9a218d636b9fb412228112695))

## [0.3.12](https://github.com/opus-domini/sentinel/compare/v0.3.11...v0.3.12) (2026-02-20)

### Features

- add cookie_secure policy for auth cookie hardening ([85547e7](https://github.com/opus-domini/sentinel/commit/85547e7213ddc637510f230e9accad40e0be4b73))

### Bug Fixes

- add loop observability, HealthChecker guards, and ticker doneCh ([63349cf](https://github.com/opus-domini/sentinel/commit/63349cf39a1d7258f64b39bf0351f436f06a188e))
- **auth:** migrate token flow to HttpOnly cookie and lock unauthenticated UI ([ee54480](https://github.com/opus-domini/sentinel/commit/ee54480f31e29985c02402bea302d853908cc8ea))
- enforce cookie_secure policy and block insecure remote startup ([8b996b2](https://github.com/opus-domini/sentinel/commit/8b996b231d1ebf2308ca37b1aa005d814f111651))
- ensure deterministic terminal state for async jobs on cancellation ([ed62a67](https://github.com/opus-domini/sentinel/commit/ed62a67d83725c48ef3f1eba09533e8423f524f3))
- harden domain invariants and close safety gaps ([53a2f1d](https://github.com/opus-domini/sentinel/commit/53a2f1dd31325c71e4886af6a29c761aa47c306c))
- implement http.Hijacker on statusRecorder for WebSocket upgrade ([c2d95c0](https://github.com/opus-domini/sentinel/commit/c2d95c0da5053c0aa983ec71bdb3841431fb359e))
- improve mobile responsiveness and add empty state placeholders ([6b3ab31](https://github.com/opus-domini/sentinel/commit/6b3ab3191c3fe1ca22edb775ccbcf76e5cff249e))
- initialize recovery runCtx in constructor to prevent CLI panic ([0643411](https://github.com/opus-domini/sentinel/commit/064341135619e80cd2bed2a43a0cf7b73f3471c4))
- limit scheduler goroutine burst and validate unit action inputs ([3b97c9d](https://github.com/opus-domini/sentinel/commit/3b97c9de89c3ace2ab847758faaa5b298694ed64))
- preserve once schedule disabled state and bound catch-up burst ([573959a](https://github.com/opus-domini/sentinel/commit/573959a3a8ea487832df76730b25212fea79e23c))
- refocus terminal after tmux window tab switch ([ccd3cce](https://github.com/opus-domini/sentinel/commit/ccd3cce890e6802a005d472e3aead6abc639cd11))
- resolve ESLint duplicate import and type-only import errors ([30797aa](https://github.com/opus-domini/sentinel/commit/30797aab11ee54d631d2356a9868e45d746fecd1))
- resolve ESLint duplicate import and type-only import errors ([62d796e](https://github.com/opus-domini/sentinel/commit/62d796e4da1099e515cfdb42434299595600b11a))
- resolve ESLint duplicate import and type-only import errors ([9f5e524](https://github.com/opus-domini/sentinel/commit/9f5e524d7330b1901101b2e60951bbf50c477544))
- resolve exhaustive switch and goconst lint issues ([e349620](https://github.com/opus-domini/sentinel/commit/e34962066d8780562b5b6d70512c2fe47610a4ba))
- scheduler recurrence, manual trigger completion, and CLI restore contract ([7a2018f](https://github.com/opus-domini/sentinel/commit/7a2018feba855985f3fbff12792e71a28a4c5c8f))
- wrap SideRail e2e test with LayoutContext provider ([8ffde77](https://github.com/opus-domini/sentinel/commit/8ffde77a5148ee71109d6df2a3bdd3fba853a1c7))

### Refactors

- add lifecycle context propagation and scheduler concurrency control ([19e315e](https://github.com/opus-domini/sentinel/commit/19e315e1c5ac4035a7e1083614ebf6f140da52bc))
- consolidate WS auth checks and propagate request context to PTY ([03dc652](https://github.com/opus-domini/sentinel/commit/03dc6521258a9059b629c83df3482e2d2836b5e4))
- create single opsManager in composition root ([59019bc](https://github.com/opus-domini/sentinel/commit/59019bcd5758e5fff6efb9f442a6040fcb0da52d))
- decompose store/ops.go into domain-specific files ([e9318f4](https://github.com/opus-domini/sentinel/commit/e9318f4715778007dee15fffbd8a8834aadde511))
- decouple recovery and watchtower from \*store.Store ([680074a](https://github.com/opus-domini/sentinel/commit/680074aef5c93e12bb1ec6754963f2e37f91e29b))
- extract orchestrations from handlers into opsOrchestrator ([1ffaf3a](https://github.com/opus-domini/sentinel/commit/1ffaf3a6bc7afb48fca975d25c78d8b2deebc36d))
- improve request logging and remove dead security code ([d9ee213](https://github.com/opus-domini/sentinel/commit/d9ee21351886cfc88fa20d5625a24a53625b13f8))
- remove \*store.Store from handler structs in api and httpui ([7f674ee](https://github.com/opus-domini/sentinel/commit/7f674eec53219ab28676dab7e95ffcbdc5eb5bc1))
- rename internal/service to internal/daemon ([16ea46a](https://github.com/opus-domini/sentinel/commit/16ea46a337c6ad296445c15c8c57c92ff0a3193b))

## [0.3.11](https://github.com/opus-domini/sentinel/compare/v0.3.10...v0.3.11) (2026-02-19)

### Bug Fixes

- simplify remote security validation and improve startup logs ([d5f9886](https://github.com/opus-domini/sentinel/commit/d5f9886e0dd7ed7f889efbee895ccb93b613ae93))
- simplify remote security validation and improve startup logs ([df73c21](https://github.com/opus-domini/sentinel/commit/df73c21f73581c1cbccb50104a521ab1dbb6351a))

## [0.3.10](https://github.com/opus-domini/sentinel/compare/v0.3.9...v0.3.10) (2026-02-18)

### Bug Fixes

- bump Go to 1.25.7 to resolve stdlib vulnerabilities ([aa9224b](https://github.com/opus-domini/sentinel/commit/aa9224b14447db88f6ad9d5d7aef620fff8b85b6))

### Refactors

- gold standard audit — security, code quality, CI, frontend, tests, and API consistency ([28a3128](https://github.com/opus-domini/sentinel/commit/28a3128c3d688f5b5921352cf8dede3b90bc2c8f))

### CI

- bump GitHub Actions to latest major versions ([7b5259d](https://github.com/opus-domini/sentinel/commit/7b5259d130138c6e3d13c0641d3bc876775101a4))

## [0.3.9](https://github.com/opus-domini/sentinel/compare/v0.3.8...v0.3.9) (2026-02-18)

### Features

- add built-in Apply Update runbook ([ab4aa39](https://github.com/opus-domini/sentinel/commit/ab4aa394aa94c6c152ea75849dbf427c550dc1a3))

### Bug Fixes

- add return after t.Fatal in nil guards to satisfy staticcheck SA5011 ([fc21299](https://github.com/opus-domini/sentinel/commit/fc212990b7ff0018b8d60ec6eeb4ebb0edd45466))
- clipboard crash and silent failure on plain HTTP ([02c94e2](https://github.com/opus-domini/sentinel/commit/02c94e2ce89818a7d9cc7f90243231aec0893c24))
- replace crypto.randomUUID with fallback for non-HTTPS contexts ([a8b6b93](https://github.com/opus-domini/sentinel/commit/a8b6b939de090024da40e9e6c7947f7c15f05ced))

## [0.3.8](https://github.com/opus-domini/sentinel/compare/v0.3.7...v0.3.8) (2026-02-18)

### Features

- add rich log viewer with streaming for services page ([37cecdb](https://github.com/opus-domini/sentinel/commit/37cecdb0dacd92aa001478f23de469cbb2289977))
- add sparklines with hover tooltips to metrics page ([268f213](https://github.com/opus-domini/sentinel/commit/268f213d301ff90885c86e554632cbb3c59fd120))

## [0.3.7](https://github.com/opus-domini/sentinel/compare/v0.3.6...v0.3.7) (2026-02-18)

### Bug Fixes

- enable clipboard copy for mouse-selected text in web terminal ([6b0ab83](https://github.com/opus-domini/sentinel/commit/6b0ab83c40f2f348d77faed700581987c8531c2c))
- enable clipboard copy for mouse-selected text in web terminal ([8a18c74](https://github.com/opus-domini/sentinel/commit/8a18c74d85a810a06941e035b5e52b22a2ddcc77))

## [0.3.6](https://github.com/opus-domini/sentinel/compare/v0.3.5...v0.3.6) (2026-02-18)

### Features

- push metrics over WebSocket and fix settings dialog on mobile ([411a078](https://github.com/opus-domini/sentinel/commit/411a07841c1541b164a479296ba547ae7b2e8bb4))

### Bug Fixes

- align live indicators with backend 5s push tickers ([a4b8427](https://github.com/opus-domini/sentinel/commit/a4b84278a4c3cacef64f2986c5b5c7565ab93d10))
- **client:** replace timestamps with static refresh rate indicator ([c1a597f](https://github.com/opus-domini/sentinel/commit/c1a597fde8bb6869e4cba5b3d09d12318b1892e4))
- **client:** show relative time for metrics timestamps ([78b34c5](https://github.com/opus-domini/sentinel/commit/78b34c53ee440cc2ade2e5777717b4851f1db029))

### Documentation

- add screenshots and captions across all feature docs and README ([bfc374e](https://github.com/opus-domini/sentinel/commit/bfc374e5830e62b743e15be53516ea210fd1c301))
- prefix ops control plane captions with feature names ([7311a21](https://github.com/opus-domini/sentinel/commit/7311a21a56bb769076eec74906aac6de582c08a4))
- rewrite README facecard and remove legacy /ops references ([8ddda33](https://github.com/opus-domini/sentinel/commit/8ddda33d899249977f35ccdc8f550a1df1860ee3))

## [0.3.5](https://github.com/opus-domini/sentinel/compare/v0.3.4...v0.3.5) (2026-02-18)

### Features

- **client:** improve mobile responsiveness and runbook consistency ([9b2f984](https://github.com/opus-domini/sentinel/commit/9b2f984cb6e1d23339cd7dd931e3dfe3718fc76d))

## [0.3.4](https://github.com/opus-domini/sentinel/compare/v0.3.3...v0.3.4) (2026-02-18)

### Bug Fixes

- resolve lint issues in new test files ([763e99a](https://github.com/opus-domini/sentinel/commit/763e99a58c9a6c5e36d253a30714bd55cdf3d9be))

## [0.3.3](https://github.com/opus-domini/sentinel/compare/v0.3.2...v0.3.3) (2026-02-18)

### Features

- **client:** redesign SideRail with lucide icons and active indicator ([83b5c3c](https://github.com/opus-domini/sentinel/commit/83b5c3c69ca848b69d67ef70864884cbef625e6e))

## [0.3.2](https://github.com/opus-domini/sentinel/compare/v0.3.1...v0.3.2) (2026-02-18)

### Features

- **client:** add help dialog to tmux, services, and runbooks sidebars ([27a87e5](https://github.com/opus-domini/sentinel/commit/27a87e54c2f212c6c75669343b8a45ab08f52bf8))
- **ops:** persist runbook step output and add dedicated pages ([aaf1a88](https://github.com/opus-domini/sentinel/commit/aaf1a88de404e02dfa92981a860fd117965fcdba))

### Documentation

- update documentation for ops page breakup and new features ([83979fd](https://github.com/opus-domini/sentinel/commit/83979fd46140931698348746282b542dc6f1f217))

## [0.3.1](https://github.com/opus-domini/sentinel/compare/v0.3.0...v0.3.1) (2026-02-15)

### Features

- **client:** refine ops and session sidebar UX ([0941f21](https://github.com/opus-domini/sentinel/commit/0941f21f140957c0674c6f9b53e17e3e74841de8))
- **ops:** add browsed services and unit-level controls ([b8e7e23](https://github.com/opus-domini/sentinel/commit/b8e7e23f1168c6b57ca00bd903e999fcd6e81383))

## [0.3.0](https://github.com/opus-domini/sentinel/compare/v0.2.13...v0.3.0) (2026-02-15)

### ⚠ BREAKING CHANGES

- removed /api/terminals\* and /ws/terminals endpoints, plus /terminal and /terminals routes.

### Features

- **client:** cache tmux state with react-query ([c98d6a0](https://github.com/opus-domini/sentinel/commit/c98d6a02a8e43f848be11001e85c4d0c017752dc))
- **client:** expand react-query caching across ops and settings ([bc7fde4](https://github.com/opus-domini/sentinel/commit/bc7fde44028ca38a6104a993591cda4de341f1e6))
- **client:** persist tmux tabs and add react-query dependency ([1d5de25](https://github.com/opus-domini/sentinel/commit/1d5de25f7217bf80d8bcc3550570c3d1494a04d5))
- **metrics:** add host metrics collection for CPU, memory, disk, and load averages ([ef81bf4](https://github.com/opus-domini/sentinel/commit/ef81bf4d3085eb982d76d68d52463ecb204e9629))
- **ops:** add service status inspect and resilient updater actions ([b06df3b](https://github.com/opus-domini/sentinel/commit/b06df3bfef006914105815e3ff3a444eb36c20ae))
- **ops:** discover and register host services from sidebar ([5e41c96](https://github.com/opus-domini/sentinel/commit/5e41c9686085ca9398fa16e08f35197495106afe))
- replace standalone terminals with ops control plane ([8a291bd](https://github.com/opus-domini/sentinel/commit/8a291bd5191d96b2bfc7bad9bdc6f77a705a0d4f))
- **runbook:** implement runbook executor for sequential step execution ([ef81bf4](https://github.com/opus-domini/sentinel/commit/ef81bf4d3085eb982d76d68d52463ecb204e9629))
- **store:** add CRUD operations for custom services and runbooks ([ef81bf4](https://github.com/opus-domini/sentinel/commit/ef81bf4d3085eb982d76d68d52463ecb204e9629))

### Bug Fixes

- **client:** polish ops panel affordances and header layout ([87a289e](https://github.com/opus-domini/sentinel/commit/87a289ea09afa25a8761dde7d043111bf01b9c5b))
- **client:** prevent tmux inspector loading race on route return ([507baf6](https://github.com/opus-domini/sentinel/commit/507baf6c247a6031aecbccf7294b1e0c6ba05ec1))

## [0.2.13](https://github.com/opus-domini/sentinel/compare/v0.2.12...v0.2.13) (2026-02-15)

### Refactors

- reduce backend cyclomatic complexity ([18862e0](https://github.com/opus-domini/sentinel/commit/18862e027e4a0e18b0d59731c8b0b6ab35df77ee))

## [0.2.12](https://github.com/opus-domini/sentinel/compare/v0.2.11...v0.2.12) (2026-02-15)

### Documentation

- restructure docsify guide and expand product documentation ([62f2b5d](https://github.com/opus-domini/sentinel/commit/62f2b5db716924198ab3124d8379f054422ab566))
- simplify root readme and point to detailed docs ([6139803](https://github.com/opus-domini/sentinel/commit/61398034c51ee5d6b611bab1e1fecd5da9d1f102))

## [0.2.11](https://github.com/opus-domini/sentinel/compare/v0.2.10...v0.2.11) (2026-02-15)

### Bug Fixes

- **service:** restart active unit on install ([9cbc6d0](https://github.com/opus-domini/sentinel/commit/9cbc6d0903b6285db6c7ceea19b7d09344fa459d))

## [0.2.10](https://github.com/opus-domini/sentinel/compare/v0.2.9...v0.2.10) (2026-02-15)

### Bug Fixes

- **service:** preserve home/config resolution for system installs ([5501e5f](https://github.com/opus-domini/sentinel/commit/5501e5f907ff94b7088237c8538039641adb14c2))

## [0.2.9](https://github.com/opus-domini/sentinel/compare/v0.2.8...v0.2.9) (2026-02-15)

### Bug Fixes

- **install:** unify service install behavior across scopes ([8d99587](https://github.com/opus-domini/sentinel/commit/8d99587cc012664171eff187317e9ca5cf11255a))
- **install:** use sentinel service name for root installs ([eb76405](https://github.com/opus-domini/sentinel/commit/eb76405224e1a4b76102ac8b303469de5db8944c))

## [0.2.8](https://github.com/opus-domini/sentinel/compare/v0.2.7...v0.2.8) (2026-02-15)

### Refactors

- **cli:** unify cross-platform service semantics and scope flags ([91a40c2](https://github.com/opus-domini/sentinel/commit/91a40c2e4a0950eb52849b05db2345a7e8214528))

## [0.2.7](https://github.com/opus-domini/sentinel/compare/v0.2.6...v0.2.7) (2026-02-15)

### Features

- **ui:** show runtime version in settings and harden status parsing ([178ef2f](https://github.com/opus-domini/sentinel/commit/178ef2fc9d8be570f39f8c19220e3ec0c15eb489))

## [0.2.6](https://github.com/opus-domini/sentinel/compare/v0.2.5...v0.2.6) (2026-02-15)

### Features

- **service:** add seamless auto scope across launchd and updater restart ([945046f](https://github.com/opus-domini/sentinel/commit/945046f8ed29c6015e3c4a7ee25bc6ab396ec320))

### Refactors

- **cli:** standardize long option style in help and docs ([51c3d7f](https://github.com/opus-domini/sentinel/commit/51c3d7f368ee3f3af23a7cbeb6d50ad7b4201943))

## [0.2.5](https://github.com/opus-domini/sentinel/compare/v0.2.4...v0.2.5) (2026-02-15)

### Features

- add launchd service and autoupdate support for macos ([5d08b34](https://github.com/opus-domini/sentinel/commit/5d08b34b7a4eb108b2580f120ec78be11822e0bd))
- add secure autoupdate workflow for service installs ([25b7774](https://github.com/opus-domini/sentinel/commit/25b77740c984980013eb53acf786347f5d810e27))
- improve cli output readability with pretty status formatting ([07f87b1](https://github.com/opus-domini/sentinel/commit/07f87b141fa2c7720711e41ae26f61d11b1a5ee5))

### CI

- publish checksums and embed release version in binaries ([a1d508d](https://github.com/opus-domini/sentinel/commit/a1d508dae06c05ed08971040aae3546234907e04))

### Documentation

- document autoupdate commands and install options ([f968733](https://github.com/opus-domini/sentinel/commit/f968733d0b72f9676ac8c78afcb27ffb4f1e4e56))

## [0.2.4](https://github.com/opus-domini/sentinel/compare/v0.2.3...v0.2.4) (2026-02-14)

### Features

- **client:** add mobile touch-to-scroll bridge for tmux ([df7db0d](https://github.com/opus-domini/sentinel/commit/df7db0d0fb31d63b80a0d7f579927f673c31e8ff))
- **client:** make session rename optimistic and stable ([a03f7a3](https://github.com/opus-domini/sentinel/commit/a03f7a3705b41cda694ac606c54421447b7eabdf))
- **client:** standardize optimistic tmux mutations ([836cd9f](https://github.com/opus-domini/sentinel/commit/836cd9f0b4c29d9a02f34ff43382fac2ea07c358))
- **events:** enrich tmux updates with inspector patches and event IDs ([226e62e](https://github.com/opus-domini/sentinel/commit/226e62eb82f53726999fb1fd58694bd50e2b5225))
- **pwa:** add install/update flow with local app shell caching ([fafe9d1](https://github.com/opus-domini/sentinel/commit/fafe9d18abdde46057f6a118a6d8433588d4087d))
- **tmux:** auto-name new windows and panes ([41a066b](https://github.com/opus-domini/sentinel/commit/41a066bbebdb95f988bb90a42a7cba51a1866437))
- **tmux:** keep window naming monotonic and create on right ([30f6ec3](https://github.com/opus-domini/sentinel/commit/30f6ec3111dfafc7491d17a8005d4311e8e1185b))

### Bug Fixes

- **api:** name new windows by session size ([6717427](https://github.com/opus-domini/sentinel/commit/6717427e7e9c717d5ae6e0eff8bcebfd72b4703b))
- **client:** harden optimistic tmux create/kill flows ([b49854c](https://github.com/opus-domini/sentinel/commit/b49854c2e493cd2f39ecc31f98cb076c9c70f777))
- **client:** make split panes optimistic and right-aligned ([a56ac4c](https://github.com/opus-domini/sentinel/commit/a56ac4c70a75d1d4f40c3e1ceedc00709e263fe5))
- **client:** prevent ghost windows after optimistic close/create ([6431ae8](https://github.com/opus-domini/sentinel/commit/6431ae8834d73d768cf94f812847a63814cebb34))
- **client:** stabilize mobile touch scroll with keyboard open ([7f9edef](https://github.com/opus-domini/sentinel/commit/7f9edef0573bbc23173da26489fcae504b61eec9))
- **tmux:** keep optimistic session creation attached until backend confirms ([7982c83](https://github.com/opus-domini/sentinel/commit/7982c835816126cd67cd18e6df3b67e6f9482df8))
- **tmux:** make session kill optimistic with rollback ([75cdedd](https://github.com/opus-domini/sentinel/commit/75cdedd7c2a924a4aca432dca0b29fa1f61c306a))

### Performance

- **client:** reduce activity-driven session refresh fallback ([11d6e79](https://github.com/opus-domini/sentinel/commit/11d6e79de38ce7da275449dc55d822c4b0723bb3))

### Documentation

- **readme:** document frontend realtime metrics diagnostics ([2a688f1](https://github.com/opus-domini/sentinel/commit/2a688f1232af684c56343d2b9f007597d0052f71))

## [0.2.3](https://github.com/opus-domini/sentinel/compare/v0.2.2...v0.2.3) (2026-02-14)

### Features

- implement watchtower activity projections and unread UX ([cb16068](https://github.com/opus-domini/sentinel/commit/cb160680258e45a73080b9864611bdfa7cd175f4))
- support seen acknowledgements over events websocket ([8aabb72](https://github.com/opus-domini/sentinel/commit/8aabb7251a7f84ab361488c809bdff3ac65d45b9))

### Bug Fixes

- keep idle session metadata visible in sidebar ([21d02bd](https://github.com/opus-domini/sentinel/commit/21d02bda04e68b7001843cfd8f8b8d030f907174))
- prevent session list cards from causing horizontal overflow ([efb6349](https://github.com/opus-domini/sentinel/commit/efb6349def2a6e25c80977bf90b13bdbc4b71273))
- refresh window unread state without pane refetch storms ([8b580c0](https://github.com/opus-domini/sentinel/commit/8b580c01fcadd7c0e1b2db2e249bfb98258fe0c3))
- simplify unread indicators in tmux ui ([05ab75a](https://github.com/opus-domini/sentinel/commit/05ab75a21d6701575bce90b8953e9de19c2f7bcf))
- stabilize attached session grouping during attach transitions ([ed2906b](https://github.com/opus-domini/sentinel/commit/ed2906b6cfadd15fff80c17829b09a3f54789ccb))

### Performance

- apply watchtower session deltas over events websocket ([10c5ead](https://github.com/opus-domini/sentinel/commit/10c5ead7a9db9d24928e4e35b56a9426d468f115))
- eliminate inspector refresh leaks across sessions ([e5a17d6](https://github.com/opus-domini/sentinel/commit/e5a17d617a297ba72f63de10fc025996fd949106))
- move tmux presence heartbeat to websocket ([e824c39](https://github.com/opus-domini/sentinel/commit/e824c3903b7146b737163ee20c6a54cad598e398))
- reduce sessions polling with seen session patches ([05a192d](https://github.com/opus-domini/sentinel/commit/05a192d1ea290bba43b8be8d29d7f4e98f6f30b6))
- skip redundant tmux select and keep optimistic selection ([1a54be6](https://github.com/opus-domini/sentinel/commit/1a54be655d2fcaf62fdfd027a311af5581afd07c))
- stop inspector polling loop on stable unread output ([478f772](https://github.com/opus-domini/sentinel/commit/478f772b52fc6960e4b0db29c0b85e686d34f22c))
- stop inspector refetch on recovery activity ([59795b7](https://github.com/opus-domini/sentinel/commit/59795b75e4d366d249c62970114fb2a764928b9e))

### Refactors

- build recovery snapshots from watchtower projections ([e4eb050](https://github.com/opus-domini/sentinel/commit/e4eb050a1f4e6c567fc9628736e9b7e65ad119f9))

### CI

- add release-please token fallback and permissions docs ([f0c5ec9](https://github.com/opus-domini/sentinel/commit/f0c5ec9cefeda54cdfd53e6551448b4d0df09376))
- **release:** automate versioning and release notes ([fa5e337](https://github.com/opus-domini/sentinel/commit/fa5e337d747b2f67b6e0cabf9b0068cbe7100c41))
- use github token only for release-please ([2a79860](https://github.com/opus-domini/sentinel/commit/2a79860100ef7a72279302ea634d0a3090581981))
