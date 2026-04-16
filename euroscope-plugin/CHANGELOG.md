# Changelog

## [0.11.0](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.10.0...plugin/v0.11.0) (2026-04-16)


### Features

* open app button in ES ([76c9332](https://github.com/flightstrips/FlightStrips/commit/76c933217b5de374106e1d0c8c609e6505c729ae))

## [0.10.0](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.9.0...plugin/v0.10.0) (2026-04-12)


### Features

* CLR / PDC tag items and funnctions ([13a5f6e](https://github.com/flightstrips/FlightStrips/commit/13a5f6ebe590091812c9552ecfd054cdbfeafe5c))
* pdc remarks ([8179aea](https://github.com/flightstrips/FlightStrips/commit/8179aea9b93d8540d8041a12052566710a017b14))

## [0.9.0](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.8.6...plugin/v0.9.0) (2026-04-11)


### Features

* Add connection selector ([c3b0ebf](https://github.com/flightstrips/FlightStrips/commit/c3b0ebfc2b2c1ff8d845e31e449aab477be98682))
* delay tag drop to after aircraft has landed ([be217b2](https://github.com/flightstrips/FlightStrips/commit/be217b2d0277c039e1518f8c6fb185015fd7db06))
* Implement CDM backend and EuroScope flow ([51d6a77](https://github.com/flightstrips/FlightStrips/commit/51d6a77676da6e211e5df9a274d067db1c02c529))


### Bug Fixes

* CDM options tag function ([f10be8f](https://github.com/flightstrips/FlightStrips/commit/f10be8ff18b2765847c4f27fcd849353b900b90a))
* remove IsReceived check ([8ccd66e](https://github.com/flightstrips/FlightStrips/commit/8ccd66e516ec891e3cce71030cf13860070431f0))

## [0.8.6](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.8.5...plugin/v0.8.6) (2026-04-07)


### Bug Fixes

* faster reconnects ([f09d3ee](https://github.com/flightstrips/FlightStrips/commit/f09d3ee0dceaf974bc8aba4da165c117a8010595))

## [0.8.5](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.8.4...plugin/v0.8.5) (2026-04-06)


### Bug Fixes

* Drop invalid UTF-8 chars ([fec2329](https://github.com/flightstrips/FlightStrips/commit/fec2329c1a10b530473ba40b24820b2ad2c6b6a3))
* enable ES plugin ([da48c67](https://github.com/flightstrips/FlightStrips/commit/da48c674d0af2713e0835bc6231141606779d469))

## [0.8.4](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.8.3...plugin/v0.8.4) (2026-03-29)


### Bug Fixes

* potential crashes ([e31bed9](https://github.com/flightstrips/FlightStrips/commit/e31bed9285d2b92d265cd943348b440f90a5a665))

## [0.8.3](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.8.2...plugin/v0.8.3) (2026-03-24)


### Bug Fixes

* disable ES plugin ([d2ac362](https://github.com/flightstrips/FlightStrips/commit/d2ac362dee0ac0c59a3477faca118bb4bf7bae5e))

## [0.8.2](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.8.1...plugin/v0.8.2) (2026-03-24)


### Bug Fixes

* ES build ([#115](https://github.com/flightstrips/FlightStrips/issues/115)) ([c8211fa](https://github.com/flightstrips/FlightStrips/commit/c8211fae1ae4978b12796c27f99d82f5f931de5c))

## [0.8.1](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.8.0...plugin/v0.8.1) (2026-03-24)


### Bug Fixes

* only set tobt on ready message ([0667dfe](https://github.com/flightstrips/FlightStrips/commit/0667dfe29baaa46155781fc271955d1ee0d28541))

## [0.8.0](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.7.1...plugin/v0.8.0) (2026-03-23)


### Features

* enable ES plugin by default ([2aaab99](https://github.com/flightstrips/FlightStrips/commit/2aaab9982f0c77e2f7a8c31ca7fe4502ca76e218))
* Fast CDM ready ([6e3f3ae](https://github.com/flightstrips/FlightStrips/commit/6e3f3ae6f80e913f0dfcf566c22f7ae1ee616b02))
* Sync ES CDM data to backend ([5a4111c](https://github.com/flightstrips/FlightStrips/commit/5a4111c10eee4f86b4dc7284d0989a76b94db7bf))


### Bug Fixes

* change fast path to write to strip annotation ([3b72dc8](https://github.com/flightstrips/FlightStrips/commit/3b72dc89dc8f550a043bf5828345b75e67676877))

## [0.7.1](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.7.0...plugin/v0.7.1) (2026-03-22)


### Bug Fixes

* Revert ES tests ([72f63a4](https://github.com/flightstrips/FlightStrips/commit/72f63a49a6cc715f6d3d3c95ef576f64ccc4a581))

## [0.7.0](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.6.3...plugin/v0.7.0) (2026-03-20)


### Features

* Add tests for ES plugin ([9c84e0a](https://github.com/flightstrips/FlightStrips/commit/9c84e0a28d339cc0f6472e07b8d98baf3e68fd5d))
* Correct PDC ([8930e27](https://github.com/flightstrips/FlightStrips/commit/8930e27512f251c756ce38395989be0f2a5666c5))
* Create IFR / VFR flightplan ([07a158b](https://github.com/flightstrips/FlightStrips/commit/07a158b4fc96059fcf77e3002f1ea517f914c443))
* Send all aircrafts even if they have no FP ([756a1c8](https://github.com/flightstrips/FlightStrips/commit/756a1c861307e698718f7e41d703128aeb12a0de))
* **sids:** source available SIDs from EuroScope sync event ([43a1f1f](https://github.com/flightstrips/FlightStrips/commit/43a1f1f6eaa82bbb854a4967f7a5bf8e5705e8bd))


### Bug Fixes

* disingenuous between FP and no FP ([aa87d4f](https://github.com/flightstrips/FlightStrips/commit/aa87d4f39661a2286cee90e09d647af10b7a5cd1))
* euroscope does not send runway events ([7e1b776](https://github.com/flightstrips/FlightStrips/commit/7e1b77665baa6ff719fc9945ec15964f64c4d61c))
* possible hangs ([b0e8b52](https://github.com/flightstrips/FlightStrips/commit/b0e8b52e010fb3dc5852bb07d8b3e80d6967de70))
* reduce sync time when already online ([6d0f447](https://github.com/flightstrips/FlightStrips/commit/6d0f4471fb83bfc4c1e956720eb6de994a61a781))

## [0.6.3](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.6.2...plugin/v0.6.3) (2026-03-15)


### Bug Fixes

* Vatsim auth ([95993da](https://github.com/flightstrips/FlightStrips/commit/95993da6755b263eb18c3b75fec1ccb3c6120302))

## [0.6.2](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.6.1...plugin/v0.6.2) (2026-03-14)


### Bug Fixes

* loaclhost port ([7eea0de](https://github.com/flightstrips/FlightStrips/commit/7eea0de1c167f6453983556ca693b567468935ce))

## [0.6.1](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.6.0...plugin/v0.6.1) (2026-03-12)


### Bug Fixes

* potential crashes ([d5aa56a](https://github.com/flightstrips/FlightStrips/commit/d5aa56a5768e62db84cdd187d07fcead3e124263))
* Runway sync ([6f6fd0b](https://github.com/flightstrips/FlightStrips/commit/6f6fd0b6a0f399bc883ea2c682bdcbe7aa7a5275))

## [0.6.0](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.5.0...plugin/v0.6.0) (2026-03-12)


### Features

* include VFR/no-FP aircraft within 30NM in EuroScope sync ([11374f1](https://github.com/flightstrips/FlightStrips/commit/11374f13d9d3c556cb1dcde7d3aa4745380c5e9c))
* receive and apply backend sync event in EuroScope plugin ([c4d216e](https://github.com/flightstrips/FlightStrips/commit/c4d216ea278f178e26d0fe3e40797d1e28c891fb))
* Wait with connection and better UI ([6233d06](https://github.com/flightstrips/FlightStrips/commit/6233d06ed57bdef4ed9aacc1a75bbc84c4dcc587))


### Bug Fixes

* address post-032-044 review feedback ([cbf0714](https://github.com/flightstrips/FlightStrips/commit/cbf0714324b0263f76052d38d84e2984b6ebfdbf))
* correctly unload plugin ([2257f8b](https://github.com/flightstrips/FlightStrips/commit/2257f8bc726c7969835afd2f2761b0d42e03a859))
* move airport coordinates from hardcoded plugin map to config file ([91ec78d](https://github.com/flightstrips/FlightStrips/commit/91ec78d3183fb1f0f861e1d9decca3029d8e8d86))
* show update dialog before renaming plugin files; restore on failure ([28112a4](https://github.com/flightstrips/FlightStrips/commit/28112a4a96bd4a4fca56f39b3c1ca197b6e246d8))

## [0.5.0](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.4.0...plugin/v0.5.0) (2026-03-08)


### Features

* Support assuming and transferring tags ([2cf5d1b](https://github.com/flightstrips/FlightStrips/commit/2cf5d1b2f9bcda87b16f933ae6e91988074e4574))


### Bug Fixes

* Set correct stand path ([7605f5c](https://github.com/flightstrips/FlightStrips/commit/7605f5c21b871f61df76ec779a8917dace34b271))

## [0.4.0](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.3.0...plugin/v0.4.0) (2026-03-07)


### Features

* Allow config of stand file location ([#76](https://github.com/flightstrips/FlightStrips/issues/76)) ([bd74f4b](https://github.com/flightstrips/FlightStrips/commit/bd74f4b6c212510ab5afea27bc1770d359ef7061))
* Reduce the number of position updates being sent ([#74](https://github.com/flightstrips/FlightStrips/issues/74)) ([3227296](https://github.com/flightstrips/FlightStrips/commit/3227296079e3343fe954d7e89c4fb33345908929))


### Bug Fixes

* Fix altitudes not being sent correctly ([#77](https://github.com/flightstrips/FlightStrips/issues/77)) ([dff7374](https://github.com/flightstrips/FlightStrips/commit/dff73742fe6a7af0b37ea4e6a28bbf1c52e07068))

## [0.3.0](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.2.1...plugin/v0.3.0) (2026-01-17)


### Features

* Add auto update of ES plugin ([#71](https://github.com/flightstrips/FlightStrips/issues/71)) ([c37e958](https://github.com/flightstrips/FlightStrips/commit/c37e9581f9a83221126f57d737ad6b025faece74))
* Introduce spdlog ([26d19e8](https://github.com/flightstrips/FlightStrips/commit/26d19e8e40ddffb98d5054525b25c45cdaea204c))


### Bug Fixes

* Catch exceptions to avoid crashes ([a17db86](https://github.com/flightstrips/FlightStrips/commit/a17db861472f45d604851006c42b7115f0e2a0f5))

## [0.2.1](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.2.0...plugin/v0.2.1) (2025-12-27)


### Bug Fixes

* **plugin:** Only display UI when API is enabled ([0ad21ac](https://github.com/flightstrips/FlightStrips/commit/0ad21ac4c19a18670e63d3bcd1fede809e2805b6))

## [0.2.0](https://github.com/flightstrips/FlightStrips/compare/plugin/v0.1.0...plugin/v0.2.0) (2025-12-26)


### Features

* global release ([33b3d8e](https://github.com/flightstrips/FlightStrips/commit/33b3d8e73cc66f18b2aaba2e47756186625feeab))
