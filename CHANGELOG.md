# Changelog

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
