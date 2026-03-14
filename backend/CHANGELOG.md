# Changelog

## [0.8.2](https://github.com/flightstrips/FlightStrips/compare/backend/v0.8.1...backend/v0.8.2) (2026-03-14)


### Bug Fixes

* server address ([981dae5](https://github.com/flightstrips/FlightStrips/commit/981dae56e7b1b38eff6838dec3bfbc5c994ce16f))

## [0.8.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.8.0...backend/v0.8.1) (2026-03-14)


### Bug Fixes

* missing .env ([e9c1c00](https://github.com/flightstrips/FlightStrips/commit/e9c1c009a73eb22040ca3c354552cb2c391c747c))

## [0.8.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.7.1...backend/v0.8.0) (2026-03-14)


### Features

* Allow assuming strip with no owner ([eabca16](https://github.com/flightstrips/FlightStrips/commit/eabca16e641e60fc8d75f6b363b2af5c3f721a0c))
* Assign runway ([1c21816](https://github.com/flightstrips/FlightStrips/commit/1c21816cd0ed25407964d70a1bd7991c0be0175c))
* Controller changes ([0052835](https://github.com/flightstrips/FlightStrips/commit/00528350aff4da5d719e4c7f9fa9a1ea74751074))
* Mark non-owner changes as unexpected ([471d2ee](https://github.com/flightstrips/FlightStrips/commit/471d2ee2d8cb9d62097962ad0224a6afcb83cc5a))
* Runway arrival ([a5d4044](https://github.com/flightstrips/FlightStrips/commit/a5d4044f17be50b3f1867d417c4381b164f11676))
* Runway clearence ([138c40a](https://github.com/flightstrips/FlightStrips/commit/138c40a5a1ee36bd5f57df481de39bb173f48965))
* Unexpected changes highlight ([f8afab8](https://github.com/flightstrips/FlightStrips/commit/f8afab868df4637a0ec345f1a8c3311ccfa7a6b5))
* UPR+LWR TWY DEP ([6115e59](https://github.com/flightstrips/FlightStrips/commit/6115e59cc15d5acbaae518f81bc5656a6991e02c))


### Bug Fixes

* auto-assume correctly picks up controllers ([cf61fda](https://github.com/flightstrips/FlightStrips/commit/cf61fda5187483673ddd855d40192ab062038df9))
* better route computation ([8ac2e33](https://github.com/flightstrips/FlightStrips/commit/8ac2e3305d213505a8012fccaa9c563b77009146))
* increase auto hide time ([00fbd70](https://github.com/flightstrips/FlightStrips/commit/00fbd70821800a52cac6b58d2ef026ad293f1249))
* online / offline messages ([e367ea1](https://github.com/flightstrips/FlightStrips/commit/e367ea1a48eb9c8c42f581e02a6d7b457b6e1d12))
* reduce app container image size ([e574a8f](https://github.com/flightstrips/FlightStrips/commit/e574a8f342b6f6823d51b9bc7bab1902ed422ee8))
* unable to assume strip if it had no owner ([35e1707](https://github.com/flightstrips/FlightStrips/commit/35e1707e19be88f8082e33e24513d4f513e5de57))

## [0.7.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.7.0...backend/v0.7.1) (2026-03-12)


### Bug Fixes

* Runway sync ([6f6fd0b](https://github.com/flightstrips/FlightStrips/commit/6f6fd0b6a0f399bc883ea2c682bdcbe7aa7a5275))

## [0.7.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.6.0...backend/v0.7.0) (2026-03-12)


### Features

* auto-hide arrival strip from STAND bay after 15 seconds ([78fc8f2](https://github.com/flightstrips/FlightStrips/commit/78fc8f28a0e617e34ab11c00d485d54620fd1f6a))
* controller online/offline grace period and broadcast notifications ([a901d0d](https://github.com/flightstrips/FlightStrips/commit/a901d0d9d19557365beac24c741722fc3c83895b))
* include VFR/no-FP aircraft within 30NM in EuroScope sync ([11374f1](https://github.com/flightstrips/FlightStrips/commit/11374f13d9d3c556cb1dcde7d3aa4745380c5e9c))
* Initial support for ALB ([862846e](https://github.com/flightstrips/FlightStrips/commit/862846e3544a4199406e6b463217a16b3d4f67d4))
* push METAR from backend via atis_update event ([aa02a09](https://github.com/flightstrips/FlightStrips/commit/aa02a09cdc9c69d325dcd38e99e952a9f49d5dae))
* runway auto-assignment, update on config change, fix route trimming ([d1a0893](https://github.com/flightstrips/FlightStrips/commit/d1a089304d01008ec88cf8e7d163e30d60f54f60))
* send backend sync event to connecting EuroScope clients ([d03dc13](https://github.com/flightstrips/FlightStrips/commit/d03dc132d0ca34506be0eb490ed3a75ca7cc8612))


### Bug Fixes

* address post-032-044 review feedback ([cbf0714](https://github.com/flightstrips/FlightStrips/commit/cbf0714324b0263f76052d38d84e2984b6ebfdbf))
* auto-assume logic for cleared strips and controller online ([facefe4](https://github.com/flightstrips/FlightStrips/commit/facefe436bea8a5c68f155c88f50480e35e56653))
* cache METAR in hub and send atis_update on initial connect ([fc8ace3](https://github.com/flightstrips/FlightStrips/commit/fc8ace3c2b404a6390ef7560c2b6fecfef8d5813))
* controller path computation for inbound and cargo stands ([eebe82a](https://github.com/flightstrips/FlightStrips/commit/eebe82a657c296bf53fb19d6addd22df39c1eb86))
* Fix layouts not getting sent to the frontend ([ee41dfc](https://github.com/flightstrips/FlightStrips/commit/ee41dfcff6db639d4e7fdf49c5a03bee0468bccf))
* move airport coordinates from hardcoded plugin map to config file ([91ec78d](https://github.com/flightstrips/FlightStrips/commit/91ec78d3183fb1f0f861e1d9decca3029d8e8d86))
* prevent empty-string bay from being persisted ([22e6d9b](https://github.com/flightstrips/FlightStrips/commit/22e6d9b86ccd25247a94256740df45aec728d5d3))
* route CidOnline through hub channel to prevent data race ([388d452](https://github.com/flightstrips/FlightStrips/commit/388d45282907ade77490a493ec534c592d3b3fb3))
* update bay field when moving strip to bay ([60a8453](https://github.com/flightstrips/FlightStrips/commit/60a845322ccfd7ba20f37301c3ac8f4619e14a7c))

## [0.6.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.5.0...backend/v0.6.0) (2026-03-08)


### Features

* ES transfer to tower trigger FS coordination ([3c3a0fc](https://github.com/flightstrips/FlightStrips/commit/3c3a0fcb27509ef5397080434b082cfbe22f7f40))

## [0.5.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.4.0...backend/v0.5.0) (2026-03-08)


### Features

* Auto handoff ([089c3a6](https://github.com/flightstrips/FlightStrips/commit/089c3a6289363ab31b4f5d3e7b4360f390290142))
* Auto layout and adjust privacy page ([5a424cb](https://github.com/flightstrips/FlightStrips/commit/5a424cb3616637be9017cddb96cc516e331d9766))

## [0.4.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.3.0...backend/v0.4.0) (2026-03-07)


### Features

* Add basic CDM implentation ([146de8c](https://github.com/flightstrips/FlightStrips/commit/146de8c4d38f9aaa5958372264d3d86fc57c63b8))
* Add OTEL for monitoring ([aca637c](https://github.com/flightstrips/FlightStrips/commit/aca637c66afdf863b5c4798442244dd067eff825))
* E2E test harness with record/replay and message validation ([7c4c8a4](https://github.com/flightstrips/FlightStrips/commit/7c4c8a4bb3455b9d3efddc401c06e12bda818dc9))
* Implment basic PDC ([#72](https://github.com/flightstrips/FlightStrips/issues/72)) ([55010f5](https://github.com/flightstrips/FlightStrips/commit/55010f540b97bf3e84cdd5c0f25339d07f8f9184))
* Support release points ([41d964e](https://github.com/flightstrips/FlightStrips/commit/41d964eae2c18d6386b292dccfd251961b31bf4f))


### Bug Fixes

* **config:** correct sector region definitions in EKCH config ([7fea2c4](https://github.com/flightstrips/FlightStrips/commit/7fea2c45d1379bd054ad35d1e7b3aa1e98e69e7a))

## [0.3.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.2.0...backend/v0.3.0) (2025-12-26)


### Features

* global release ([33b3d8e](https://github.com/flightstrips/FlightStrips/commit/33b3d8e73cc66f18b2aaba2e47756186625feeab))

## [0.2.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.1.0...backend/v0.2.0) (2025-12-26)


### Features

* global release ([33b3d8e](https://github.com/flightstrips/FlightStrips/commit/33b3d8e73cc66f18b2aaba2e47756186625feeab))

## [0.2.0](https://github.com/flightstrips/FlightStrips/compare/backend-v0.1.0...backend-v0.2.0) (2025-12-26)


### Features

* global release ([33b3d8e](https://github.com/flightstrips/FlightStrips/commit/33b3d8e73cc66f18b2aaba2e47756186625feeab))
