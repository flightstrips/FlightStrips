# Changelog

## [0.10.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.9.0...frontend/v0.10.0) (2026-03-14)


### Features

* added darkmode for the web page ([182f68e](https://github.com/flightstrips/FlightStrips/commit/182f68e964ad47bfbc2c002d2c192e7babe30333))

## [0.9.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.8.0...frontend/v0.9.0) (2026-03-14)


### Features

* push site ([67d0d3f](https://github.com/flightstrips/FlightStrips/commit/67d0d3fc7d3969d2ac9b822fa40d646ef697a5b2))

## [0.8.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.7.0...frontend/v0.8.0) (2026-03-14)


### Features

* Allow assuming strip with no owner ([eabca16](https://github.com/flightstrips/FlightStrips/commit/eabca16e641e60fc8d75f6b363b2af5c3f721a0c))
* Assign runway ([1c21816](https://github.com/flightstrips/FlightStrips/commit/1c21816cd0ed25407964d70a1bd7991c0be0175c))
* Controller changes ([0052835](https://github.com/flightstrips/FlightStrips/commit/00528350aff4da5d719e4c7f9fa9a1ea74751074))
* proactive token refresh based on JWT expiry ([f956034](https://github.com/flightstrips/FlightStrips/commit/f9560342d7c8021df8ce19288442d06b0a7f0837))
* Runway arrival ([a5d4044](https://github.com/flightstrips/FlightStrips/commit/a5d4044f17be50b3f1867d417c4381b164f11676))
* Runway clearence ([138c40a](https://github.com/flightstrips/FlightStrips/commit/138c40a5a1ee36bd5f57df481de39bb173f48965))
* select stand ([82f0890](https://github.com/flightstrips/FlightStrips/commit/82f0890621b9afc3aa1c105fdb919fc22632cbac))
* Unexpected changes highlight ([f8afab8](https://github.com/flightstrips/FlightStrips/commit/f8afab868df4637a0ec345f1a8c3311ccfa7a6b5))
* UPR+LWR TWY DEP ([6115e59](https://github.com/flightstrips/FlightStrips/commit/6115e59cc15d5acbaae518f81bc5656a6991e02c))


### Bug Fixes

* add missing email contact ([a69844c](https://github.com/flightstrips/FlightStrips/commit/a69844c41613fc9e1cf6d6df22566bd9c2b8f944))
* move strip to correct twy dep when setting taxi point ([5367572](https://github.com/flightstrips/FlightStrips/commit/53675720caed9c13153d13a5fc7207a8f4b8b146))
* next controllor ([7798394](https://github.com/flightstrips/FlightStrips/commit/7798394e08ffbffe5fa8c3ee3e06674236bf8273))
* online / offline messages ([e367ea1](https://github.com/flightstrips/FlightStrips/commit/e367ea1a48eb9c8c42f581e02a6d7b457b6e1d12))

## [0.7.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.6.0...frontend/v0.7.0) (2026-03-12)


### Features

* Runway holding points ([b8dd6f0](https://github.com/flightstrips/FlightStrips/commit/b8dd6f06928a68bf662ad388a0d00678f409be58))

## [0.6.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.5.0...frontend/v0.6.0) (2026-03-12)


### Features

* add HP/TWY column to push strip and fix split display in taxi dep strip ([526dbdb](https://github.com/flightstrips/FlightStrips/commit/526dbdb04aac14eb3a5cdf9470ee0865eeffa767))
* controller online/offline grace period and broadcast notifications ([a901d0d](https://github.com/flightstrips/FlightStrips/commit/a901d0d9d19557365beac24c741722fc3c83895b))
* dynamic active/inactive CLR/DEL panel based on controller online status ([663dc14](https://github.com/flightstrips/FlightStrips/commit/663dc1424b17f74316a2ef13fb8a6e2cdff8a85e))
* handle unknown layout with chooser dialog ([f0c37b1](https://github.com/flightstrips/FlightStrips/commit/f0c37b15e920520923af56d4a55af7aa4b78df45))
* push METAR from backend via atis_update event ([aa02a09](https://github.com/flightstrips/FlightStrips/commit/aa02a09cdc9c69d325dcd38e99e952a9f49d5dae))
* show release point in push strip stand cell ([1826b60](https://github.com/flightstrips/FlightStrips/commit/1826b609beaf32018d819dcd5a447f06757bbe16))
* strip selection and click-to-move bay transfer ([b1d383e](https://github.com/flightstrips/FlightStrips/commit/b1d383e86e6c42aaa1c116ebc55b8960e2815e4d))


### Bug Fixes

* address post-032-044 review feedback ([cbf0714](https://github.com/flightstrips/FlightStrips/commit/cbf0714324b0263f76052d38d84e2984b6ebfdbf))
* Build errors ([502b900](https://github.com/flightstrips/FlightStrips/commit/502b900ddfc99797d5bca50255d89a6e9e0e32d7))
* Correctly set eihter TWY or HP ([27b3c8d](https://github.com/flightstrips/FlightStrips/commit/27b3c8d897e5f224ae3ad800edbbb413b83cef44))
* distinguish clearance limits from holding points in taxi map ([40defaf](https://github.com/flightstrips/FlightStrips/commit/40defaf6de0db7098754d9216f952d06bebcccdb))
* Incorrect detection of CTWR ([6962d5a](https://github.com/flightstrips/FlightStrips/commit/6962d5a7913ec99401bda78e2903464590079f8a))
* Remove 'pr' from holding point ([0bcbfad](https://github.com/flightstrips/FlightStrips/commit/0bcbfada7f41b664038956d0c34abc2044684241))
* use section (DEL/GND/TWR) instead of frequency strings in frontend controller hooks ([2579523](https://github.com/flightstrips/FlightStrips/commit/25795236b198510c6f0dc8b3251843f290c748c4))

## [0.5.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.4.0...frontend/v0.5.0) (2026-03-08)


### Features

* new public site ([41563b6](https://github.com/flightstrips/FlightStrips/commit/41563b66ef6338f8ae43e57100f4d6c45ad1ad1d))

## [0.4.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.3.0...frontend/v0.4.0) (2026-03-08)


### Features

* More strip designs ([4117ceb](https://github.com/flightstrips/FlightStrips/commit/4117cebb175d692f2a87252d5250b04326352833))

## [0.3.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.2.0...frontend/v0.3.0) (2026-03-08)


### Features

* Auto layout and adjust privacy page ([5a424cb](https://github.com/flightstrips/FlightStrips/commit/5a424cb3616637be9017cddb96cc516e331d9766))
* ESET View ([e715a81](https://github.com/flightstrips/FlightStrips/commit/e715a8115fc99a0fd5d245f1d3e7fc6f867167f3))

## [0.2.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.1.0...frontend/v0.2.0) (2026-03-07)


### Features

* Add CDM colors ([096b2ac](https://github.com/flightstrips/FlightStrips/commit/096b2acabe1caee1b6c8f1176754d5552499d4bc))

## 0.1.0 (2026-03-07)


### Features

* Add basic CDM implentation ([146de8c](https://github.com/flightstrips/FlightStrips/commit/146de8c4d38f9aaa5958372264d3d86fc57c63b8))
* global release ([33b3d8e](https://github.com/flightstrips/FlightStrips/commit/33b3d8e73cc66f18b2aaba2e47756186625feeab))
* Implment basic PDC ([#72](https://github.com/flightstrips/FlightStrips/issues/72)) ([55010f5](https://github.com/flightstrips/FlightStrips/commit/55010f540b97bf3e84cdd5c0f25339d07f8f9184))
* Support release points ([41d964e](https://github.com/flightstrips/FlightStrips/commit/41d964eae2c18d6386b292dccfd251961b31bf4f))
