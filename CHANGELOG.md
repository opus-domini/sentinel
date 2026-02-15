# Changelog

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
