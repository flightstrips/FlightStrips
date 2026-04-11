# Changelog

## [0.16.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.15.1...backend/v0.16.0) (2026-04-11)


### Features

* Add connection selector ([c3b0ebf](https://github.com/flightstrips/FlightStrips/commit/c3b0ebfc2b2c1ff8d845e31e449aab477be98682))
* delay tag drop to after aircraft has landed ([be217b2](https://github.com/flightstrips/FlightStrips/commit/be217b2d0277c039e1518f8c6fb185015fd7db06))
* Detect landing and move strip to TWY ARR when vacated ([a12892d](https://github.com/flightstrips/FlightStrips/commit/a12892d29032ff3b30a10df2a125cf27ae2ed1b6))
* erase heading, cleared altitude ([37c2360](https://github.com/flightstrips/FlightStrips/commit/37c236046edf6545ed5e35ca1f625561cfce7f73))
* Implement CDM backend and EuroScope flow ([51d6a77](https://github.com/flightstrips/FlightStrips/commit/51d6a77676da6e211e5df9a274d067db1c02c529))


### Bug Fixes

* correctly handle assume after missed ([991c324](https://github.com/flightstrips/FlightStrips/commit/991c324fb153cd4510a5be11a53be074bbd81c3f))
* ensure only twy-lwr bay is used when only tower is online ([d598054](https://github.com/flightstrips/FlightStrips/commit/d59805413de5b5c6dc20fe3f3bf3e10dd2bd20a3))
* remove duplicate log ([43f25dd](https://github.com/flightstrips/FlightStrips/commit/43f25dd8b8b597c74ccb557ff237afb9cb36c988))

## [0.15.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.15.0...backend/v0.15.1) (2026-04-07)


### Bug Fixes

* breaking when switching master with a lot of controllers online ([2b2cf90](https://github.com/flightstrips/FlightStrips/commit/2b2cf90c1dc56df6c8a0c567355205e0587adb55))
* support login event ([704a211](https://github.com/flightstrips/FlightStrips/commit/704a211c5af9a21f2ce7d1b7c6e225e5f17f7e58))

## [0.15.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.14.0...backend/v0.15.0) (2026-04-06)


### Features

* Set aircraft state to PARKED when in STAND bay ([a0ef3ea](https://github.com/flightstrips/FlightStrips/commit/a0ef3eae8a6a194e7d642c181d6e1a04d3466233))
* Support 30/22R runway configuration ([07739c6](https://github.com/flightstrips/FlightStrips/commit/07739c66fca2f60f474d07fcfb07147e2a3a989b))

## [0.14.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.13.0...backend/v0.14.0) (2026-03-31)


### Features

* enable movement of tactical strips ([1528c8d](https://github.com/flightstrips/FlightStrips/commit/1528c8df2d968f15048ae1de46bd8566088acacf))


### Bug Fixes

* adjust log levels ([d78163a](https://github.com/flightstrips/FlightStrips/commit/d78163a4739124d49703831215c2ec470e897e8c))
* arrival route calculation ([18ebc3d](https://github.com/flightstrips/FlightStrips/commit/18ebc3d4e2e6f9bbc1d24ce385ba2028d29151f0))
* coordination of points ([89e40a2](https://github.com/flightstrips/FlightStrips/commit/89e40a2828d287f0fce150ebadf0d369ec649df8))
* do not process pdc if strip is already cleared ([e004f51](https://github.com/flightstrips/FlightStrips/commit/e004f51a019656d17e7b4e02b868d3615d57d68b))
* send default altitude back to ES if set ([77301b0](https://github.com/flightstrips/FlightStrips/commit/77301b0e5274063db07104cd25c6b400c1ace7af))

## [0.13.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.12.0...backend/v0.13.0) (2026-03-29)


### Features

* added web-pdc ([6fdc590](https://github.com/flightstrips/FlightStrips/commit/6fdc5904a91e51dd1d595983b0137d9adadb8379))
* auto set altitude ([e0332a5](https://github.com/flightstrips/FlightStrips/commit/e0332a5abb15d001e36361a55b40e381cdb9bc5a))

## [0.12.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.11.0...backend/v0.12.0) (2026-03-28)


### Features

* gaurd metar decoding behind S3 rating or higher ([e7bcf94](https://github.com/flightstrips/FlightStrips/commit/e7bcf94e4d57f9221001fd47d1466f20fbae7deb))
* missed approach ([c12e3b5](https://github.com/flightstrips/FlightStrips/commit/c12e3b503b46a04749245f967bf5296e6c4097f3))
* Runway status ([3514fa6](https://github.com/flightstrips/FlightStrips/commit/3514fa6b7feae51b6db9044c7560c7d6318a841e))

## [0.11.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.10.4...backend/v0.11.0) (2026-03-27)


### Features

* upper TWY DEP strip moves to lower bay when assumed by tower ([299cd45](https://github.com/flightstrips/FlightStrips/commit/299cd45509ca0486dd253602fecfe9008cce0d91))


### Bug Fixes

* apron single controller always gets AAAD layout ([e6f4a13](https://github.com/flightstrips/FlightStrips/commit/e6f4a13a914c5dd7a46b5f2d5c3975370fa0082e))
* confirmed runway strips no longer turn red when new strip arrives ([bf87d57](https://github.com/flightstrips/FlightStrips/commit/bf87d57bfd6b6e373c1c237fa51bbd5af37473e2))
* dual login same position receives correct layout on connect ([44b547a](https://github.com/flightstrips/FlightStrips/commit/44b547aedc05cdf5b6927b7d0ca48bf260189a4f))
* move strip when transfering to tower from hidden tower bay ([94e6e52](https://github.com/flightstrips/FlightStrips/commit/94e6e526343fda371ded247b93eb82483a990302))
* taxi bay no longer reverts when ground state is TAXI ([9ef5173](https://github.com/flightstrips/FlightStrips/commit/9ef517355eb19e7328f8830e74b4974f6e3a6e94))
* update layout on freqency change ([280dea2](https://github.com/flightstrips/FlightStrips/commit/280dea29200fa3eee5c8d25234387b1e17217a95))

## [0.10.4](https://github.com/flightstrips/FlightStrips/compare/backend/v0.10.3...backend/v0.10.4) (2026-03-24)


### Bug Fixes

* arrivals not going in the correct bay ([41234a9](https://github.com/flightstrips/FlightStrips/commit/41234a95d35fcf8c12f7786f3ba7208132ceea2b))

## [0.10.3](https://github.com/flightstrips/FlightStrips/compare/backend/v0.10.2...backend/v0.10.3) (2026-03-24)


### Bug Fixes

* change runway? ([a25ed18](https://github.com/flightstrips/FlightStrips/commit/a25ed180774d8f00d467a80db748dc13afb138d8))
* delivery next freq ([6f90531](https://github.com/flightstrips/FlightStrips/commit/6f905313261ce24f176d1c6884c0895da9d3dd0f))

## [0.10.2](https://github.com/flightstrips/FlightStrips/compare/backend/v0.10.1...backend/v0.10.2) (2026-03-24)


### Bug Fixes

* arrival routes ([bf6d33c](https://github.com/flightstrips/FlightStrips/commit/bf6d33c11068a254510c84eba3f627828c6d0492))
* arrivals should always come into arr_hidden ([3556122](https://github.com/flightstrips/FlightStrips/commit/355612229d065553e3ce1893620a187160c6fa21))
* prev_owner added for delivery after they give clearance ([ea51f4d](https://github.com/flightstrips/FlightStrips/commit/ea51f4d9df616134f134394fd738cd4b5d14e4eb))
* push strips to noncleared bay ([59b0f83](https://github.com/flightstrips/FlightStrips/commit/59b0f833bd8597932a2ed518b6a7538913458f04))
* sending invalid pdcs ([ccf3e6b](https://github.com/flightstrips/FlightStrips/commit/ccf3e6b1751c6c14fe067f9b782d47c5bc106183))

## [0.10.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.10.0...backend/v0.10.1) (2026-03-24)


### Bug Fixes

* arrivals not showing up ([899c2b4](https://github.com/flightstrips/FlightStrips/commit/899c2b4ea0682628f01f7b746ec9ffdaf569da5f))
* missing airborne controller ([4b812a5](https://github.com/flightstrips/FlightStrips/commit/4b812a57847115886fd7bdad2b8aa25d247ec42b))
* pdc required to specfic aircraft type ([e58ba94](https://github.com/flightstrips/FlightStrips/commit/e58ba9496297cc4ca4de367f58136dd37c8eba09))

## [0.10.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.9.2...backend/v0.10.0) (2026-03-23)


### Features

* Fast CDM ready ([6e3f3ae](https://github.com/flightstrips/FlightStrips/commit/6e3f3ae6f80e913f0dfcf566c22f7ae1ee616b02))
* Sync ES CDM data to backend ([5a4111c](https://github.com/flightstrips/FlightStrips/commit/5a4111c10eee4f86b4dc7284d0989a76b94db7bf))


### Bug Fixes

* able to disable fast CDM path ([ca186d6](https://github.com/flightstrips/FlightStrips/commit/ca186d6578bf91cfc00c327995e1c150b74c7925))
* auto-handover no longer requires airborne controller to be connected to FS ([deb2b23](https://github.com/flightstrips/FlightStrips/commit/deb2b23aa6af7ff87c04a28314f6f76071cd11e3))

## [0.9.2](https://github.com/flightstrips/FlightStrips/compare/backend/v0.9.1...backend/v0.9.2) (2026-03-22)


### Bug Fixes

* atis ([63e0b23](https://github.com/flightstrips/FlightStrips/commit/63e0b230ba629365c2d63e9f68bcab866b8f5e97))

## [0.9.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.9.0...backend/v0.9.1) (2026-03-22)


### Bug Fixes

* broadcast ALB event to sender as well ([b2a494d](https://github.com/flightstrips/FlightStrips/commit/b2a494dee0eafe1143ba375ef27ee846f821377d))

## [0.9.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.8.3...backend/v0.9.0) (2026-03-20)


### Features

* add debug logging in airborne controller resolution ([cfe9974](https://github.com/flightstrips/FlightStrips/commit/cfe9974f15ba1cfd3ad192020aac06c91853eda6))
* add force assume strip command ([f3c68a9](https://github.com/flightstrips/FlightStrips/commit/f3c68a920aed45662a749032eee66a33abdcce87))
* Correct PDC ([8930e27](https://github.com/flightstrips/FlightStrips/commit/8930e27512f251c756ce38395989be0f2a5666c5))
* Create IFR / VFR flightplan ([07a158b](https://github.com/flightstrips/FlightStrips/commit/07a158b4fc96059fcf77e3002f1ea517f914c443))
* enforce strip ownership when moving strips ([2fb702d](https://github.com/flightstrips/FlightStrips/commit/2fb702df44cbc011655a7b88dca167743c2918d1))
* gate frontend connections behind active euroscope client ([4f06f3d](https://github.com/flightstrips/FlightStrips/commit/4f06f3dae917c312bfff3f5be95453014994e07c))
* Pull ATIS if available ([92fa0b2](https://github.com/flightstrips/FlightStrips/commit/92fa0b22e75f3f8ace6501785d23c645a6dba76b))
* Request strips ([a9d1a46](https://github.com/flightstrips/FlightStrips/commit/a9d1a46407e75708e0b3f35776672bbb7b8e4771))
* **sids:** source available SIDs from EuroScope sync event ([43a1f1f](https://github.com/flightstrips/FlightStrips/commit/43a1f1f6eaa82bbb854a4967f7a5bf8e5705e8bd))
* trigger layout update after active runway change ([2089ad7](https://github.com/flightstrips/FlightStrips/commit/2089ad76b9439aec3152e0920d8d309df1c6f8f3))


### Bug Fixes

* align de-ice bay constant and validate tactical strip bay ([cd16578](https://github.com/flightstrips/FlightStrips/commit/cd16578a38aa2b15778f952bd02eb8eb22861449))
* allow frontend to wait for ES connection ([c4641ea](https://github.com/flightstrips/FlightStrips/commit/c4641eaeee43ca08619cbc401a611e707b3afd1b))
* backend tests ([0a27a70](https://github.com/flightstrips/FlightStrips/commit/0a27a70d7bcc450bad6f0b5f6bcd415c7b461284))
* broadcast bulk bay event on strip sequence recalculation ([2e1c0ca](https://github.com/flightstrips/FlightStrips/commit/2e1c0ca091ffedb092c214bb68ae638c966ce92c))
* correct bay names ([fc1f085](https://github.com/flightstrips/FlightStrips/commit/fc1f085ea3318359d7c83feff67c0c144d53900c))
* disingenuous between FP and no FP ([aa87d4f](https://github.com/flightstrips/FlightStrips/commit/aa87d4f39661a2286cee90e09d647af10b7a5cd1))
* force assume ([de96249](https://github.com/flightstrips/FlightStrips/commit/de9624995a38ecbf11a0301d3325943368570798))
* ground states + force assume ([ee57f72](https://github.com/flightstrips/FlightStrips/commit/ee57f7267fa2fb319c31e4da59eb5d8ae780a130))
* handle errors on backend and frontend ([a8cda2a](https://github.com/flightstrips/FlightStrips/commit/a8cda2a610f2980efa5e42a56f6d4f24eda77649))
* missing layout ([297af2f](https://github.com/flightstrips/FlightStrips/commit/297af2f82ab069d1c50d04548f05e54f9bcd0a4b))
* service test ([66c63c5](https://github.com/flightstrips/FlightStrips/commit/66c63c5744f86bf37eda06233d3ed7aeeb53a591))
* transfer to airborne did not work for manual transfer ([815a176](https://github.com/flightstrips/FlightStrips/commit/815a176b7aeabca9c9641f1910e6b7a11c4f097a))

## [0.8.3](https://github.com/flightstrips/FlightStrips/compare/backend/v0.8.2...backend/v0.8.3) (2026-03-15)


### Bug Fixes

* departure getting wrong bay ([a7a9e01](https://github.com/flightstrips/FlightStrips/commit/a7a9e013eb6b74b9dad0c77e9f9134412f6a2e4c))
* Vatsim auth ([95993da](https://github.com/flightstrips/FlightStrips/commit/95993da6755b263eb18c3b75fec1ccb3c6120302))

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
