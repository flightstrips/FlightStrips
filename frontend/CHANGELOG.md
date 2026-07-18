# Changelog

## [0.46.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.46.0...frontend/v0.46.1) (2026-07-18)


### Bug Fixes

* **stand:** preserve assignments during stand conflicts ([14d4f42](https://github.com/flightstrips/FlightStrips/commit/14d4f425d831b4cbf041e5827e0176664fc5fc18))

## [0.46.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.45.0...frontend/v0.46.0) (2026-07-18)


### Features

* add authenticated EFB behind feature flag ([#348](https://github.com/flightstrips/FlightStrips/issues/348)) ([b9fb84e](https://github.com/flightstrips/FlightStrips/commit/b9fb84ea9522a26cc9090468713568bef5be5c30))
* refresh EFB interface and briefing assets ([#357](https://github.com/flightstrips/FlightStrips/issues/357)) ([f6233bd](https://github.com/flightstrips/FlightStrips/commit/f6233bd16a672b274fea4f772e771ebe7e742437))
* **sat:** add local test console ([#361](https://github.com/flightstrips/FlightStrips/issues/361)) ([782afae](https://github.com/flightstrips/FlightStrips/commit/782afae462081ddce1c2a32d5da07ee6e8c66c1e))
* **stand:** add system status diagnostics ([#358](https://github.com/flightstrips/FlightStrips/issues/358)) ([8f926b3](https://github.com/flightstrips/FlightStrips/commit/8f926b35f762968a5906d8dd9abaa46943698f28))
* **strip:** show arrival STAR in EFB ([#353](https://github.com/flightstrips/FlightStrips/issues/353)) ([d2431cb](https://github.com/flightstrips/FlightStrips/commit/d2431cb37783691d35fc9d632586bf2f82361583))
* **strip:** store arrival STAR ([#347](https://github.com/flightstrips/FlightStrips/issues/347)) ([02747f2](https://github.com/flightstrips/FlightStrips/commit/02747f28c25f9fd5e83655392164fdcbc49c5388))


### Bug Fixes

* **cdm:** align EST startup states ([#354](https://github.com/flightstrips/FlightStrips/issues/354)) ([3567888](https://github.com/flightstrips/FlightStrips/commit/3567888699c5134c393a8b53ea82395f2b10ab59))
* **cdm:** synchronize startup request timing ([#355](https://github.com/flightstrips/FlightStrips/issues/355)) ([53db077](https://github.com/flightstrips/FlightStrips/commit/53db07714180cd5afae83e21dc77f55e39f36674))
* **stand:** prefer current stand occupants in SEQ ([#352](https://github.com/flightstrips/FlightStrips/issues/352)) ([32f71e4](https://github.com/flightstrips/FlightStrips/commit/32f71e49a369d9ec2d88631be106ae6881439705))
* **stand:** synchronize lifecycle removals and adjacency ([#340](https://github.com/flightstrips/FlightStrips/issues/340)) ([eed49fb](https://github.com/flightstrips/FlightStrips/commit/eed49fb998d0543c3e2a7c7b272043bf3a4d8e26))
* Test console provide correct snapshot time ([#362](https://github.com/flightstrips/FlightStrips/issues/362)) ([6e253b9](https://github.com/flightstrips/FlightStrips/commit/6e253b9d25e522a9ec98dd07a685587732152d44))

## [0.45.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.44.0...frontend/v0.45.0) (2026-07-12)


### Features

* **stand:** add strip assignment workflow ([#289](https://github.com/flightstrips/FlightStrips/issues/289)) ([376ee54](https://github.com/flightstrips/FlightStrips/commit/376ee545f30aa92e5c548a579bcbb2717db10cd6))
* **stand:** complete SAT controller integration ([#287](https://github.com/flightstrips/FlightStrips/issues/287)) ([f35bcca](https://github.com/flightstrips/FlightStrips/commit/f35bcca32dc4bcfa4b78193d47bd42b5b7d4e777))
* **stand:** integrate EST board with SAT backend assignment and block metadata ([#286](https://github.com/flightstrips/FlightStrips/issues/286)) ([d37e8a9](https://github.com/flightstrips/FlightStrips/commit/d37e8a9445c8ad09b98aca5c2e411f92e5830147))


### Bug Fixes

* **stand:** keep VATSIM-only departures hidden ([#295](https://github.com/flightstrips/FlightStrips/issues/295)) ([66f4dc7](https://github.com/flightstrips/FlightStrips/commit/66f4dc7f2181f287d30eed621069da1980801d15))

## [0.44.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.43.0...frontend/v0.44.0) (2026-07-07)


### Features

* **callsigns:** source spoken callsigns from the Euroscope plugin ([#234](https://github.com/flightstrips/FlightStrips/issues/234)) ([5354294](https://github.com/flightstrips/FlightStrips/commit/5354294c0afa48fb53174120a457bd49783680b5))
* **CDM:** use CDM slot label for CTOT display ([#233](https://github.com/flightstrips/FlightStrips/issues/233)) ([8070a97](https://github.com/flightstrips/FlightStrips/commit/8070a97f08851a6e233dbb0428463e9289e7d968))
* Confirm voice clearance on cleared PDC moves ([#240](https://github.com/flightstrips/FlightStrips/issues/240)) ([6fdd6e8](https://github.com/flightstrips/FlightStrips/commit/6fdd6e8ebdd49c3223c75094e73c75b73f9731ad))
* **frontend:** add ECFMP restriction highlights in FlightPlanDialog ([2d57cc1](https://github.com/flightstrips/FlightStrips/commit/2d57cc1d04e1f21c6f6286def6e774501420c772)), closes [#221](https://github.com/flightstrips/FlightStrips/issues/221)
* **frontend:** add ECFMP restriction highlights to strip components ([ad1f8d8](https://github.com/flightstrips/FlightStrips/commit/ad1f8d8b1a810455205be47273ab2d515050c7fe)), closes [#221](https://github.com/flightstrips/FlightStrips/issues/221)
* **frontend:** add ECFMP restriction models, store updates, and helper functions ([e97cb68](https://github.com/flightstrips/FlightStrips/commit/e97cb680b0ed4f19bfba32f3cc8b3df940f58e28)), closes [#221](https://github.com/flightstrips/FlightStrips/issues/221)
* **frontend:** add MandatoryRouteDialog for ECFMP mandatory routes ([27ae0a0](https://github.com/flightstrips/FlightStrips/commit/27ae0a0b0fc889937aa7f6d2f2e1d56a5594ecd6)), closes [#221](https://github.com/flightstrips/FlightStrips/issues/221)
* **pdc:** implement mandatory route clearance flow behind feature flag ([a829e39](https://github.com/flightstrips/FlightStrips/commit/a829e39cc356eaa12d5bba09fd6634389667cb9b))
* **pilot:** Expand pilot flight details ([cf4c5c8](https://github.com/flightstrips/FlightStrips/commit/cf4c5c8e2b56ea7c5fa2b31dce7db8431d6ebd88))
* **strip:** rename IATA TYPE to SPOKEN C/S and add text-shrinking for spoken callsign ([#262](https://github.com/flightstrips/FlightStrips/issues/262)) ([5a3cb97](https://github.com/flightstrips/FlightStrips/commit/5a3cb97bda98cd813fdb77e1cbc8f87c90e794a6))


### Bug Fixes

* **ecfmp:** clear mandatory route restriction after strip is cleared ([#256](https://github.com/flightstrips/FlightStrips/issues/256)) ([0b0ae65](https://github.com/flightstrips/FlightStrips/commit/0b0ae65c2d62673c44d66a643c6ae9301638f339))
* **ecfmp:** resolve critical bugs in ECFMP restriction implementation ([9fd6a3e](https://github.com/flightstrips/FlightStrips/commit/9fd6a3e69c84913ab8d9bbf08fe978d58d6dc3fd)), closes [#221](https://github.com/flightstrips/FlightStrips/issues/221)

## [0.43.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.42.0...frontend/v0.43.0) (2026-05-25)


### Features

* **frontend,backend:** highlight manual TOBT times ([04cabb1](https://github.com/flightstrips/FlightStrips/commit/04cabb1cd0b3de4d3a8002261e22b71684b17ff9)), closes [#201](https://github.com/flightstrips/FlightStrips/issues/201)


### Bug Fixes

* **cdm:** batch websocket updates and cap backend memory ([76239a5](https://github.com/flightstrips/FlightStrips/commit/76239a536c8044f039bc2b1e9d388751a4bd3deb))

## [0.42.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.41.0...frontend/v0.42.0) (2026-05-17)


### Features

* **controlzone:** add CONTROLZONE strip workflow ([49ac51c](https://github.com/flightstrips/FlightStrips/commit/49ac51cb24a0b8e2ab0c78dc68a6f38478067780))


### Bug Fixes

* missing colors ([8931c30](https://github.com/flightstrips/FlightStrips/commit/8931c30e6f025f256c94167da06af8e6a6ae7011))

## [0.41.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.40.0...frontend/v0.41.0) (2026-05-16)


### Features

* **backend,frontend,plugin:** track client versions ([7c6d509](https://github.com/flightstrips/FlightStrips/commit/7c6d509aac88cb49980bd3fb54fa61ec5e559c5a))

## [0.40.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.39.0...frontend/v0.40.0) (2026-05-15)


### Features

* **vacs:** use controller LAN IP for associated frontends ([d1a32b0](https://github.com/flightstrips/FlightStrips/commit/d1a32b0223f4e589774b63982e502e418034d03a))

## [0.39.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.38.4...frontend/v0.39.0) (2026-05-15)


### Features

* **frontend,docs:** optional VACS remote host in settings ([9a1802d](https://github.com/flightstrips/FlightStrips/commit/9a1802dfea2ad1ec9ca623599619f112d62e20a8))
* **frontend,docs:** outgoing VACS ringing UI and user documentation ([8d786c4](https://github.com/flightstrips/FlightStrips/commit/8d786c43c962c4a4d42827f36a738993edb3718a))

## [0.38.4](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.38.3...frontend/v0.38.4) (2026-05-15)


### Bug Fixes

* **frontend:** buiild error resolved ([d0bf3ad](https://github.com/flightstrips/FlightStrips/commit/d0bf3adbf6b0cf3f6413b4edb3b27bfbffbd7c42))

## [0.38.3](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.38.2...frontend/v0.38.3) (2026-05-15)


### Bug Fixes

* **frontend:** send CallSource struct for VACS signaling_start_call ([54a9416](https://github.com/flightstrips/FlightStrips/commit/54a94160cc58af33fb2f48dbb082071bfa95132a))

## [0.38.2](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.38.1...frontend/v0.38.2) (2026-05-15)


### Bug Fixes

* **frontend:** use lowercase VACS call target enum tags ([2266e36](https://github.com/flightstrips/FlightStrips/commit/2266e369ed7f5e8bb1a494654129fd6417fce128))

## [0.38.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.38.0...frontend/v0.38.1) (2026-05-15)


### Bug Fixes

* **frontend:** fix VACS dial errors and simplify end-call button ([a00510d](https://github.com/flightstrips/FlightStrips/commit/a00510d8e1f44177e64a929548107bbfb88bd227))

## [0.38.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.37.0...frontend/v0.38.0) (2026-05-15)


### Features

* **frontend:** add VACS voice integration to command bar ([fe6ef3d](https://github.com/flightstrips/FlightStrips/commit/fe6ef3dee4b6981118fb0458d0f80a144c913366))

## [0.37.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.36.2...frontend/v0.37.0) (2026-05-13)


### Features

* **branding:** added new logo as favicon ([761de56](https://github.com/flightstrips/FlightStrips/commit/761de5634d14691cfc31e004eb5ab891bd933464))
* **frontend:** Added logo to loading pages ([fcac0b6](https://github.com/flightstrips/FlightStrips/commit/fcac0b67d2406a3ae29c0db093512f87be0cce85))


### Bug Fixes

* **public:** broken login button ([1eccdc8](https://github.com/flightstrips/FlightStrips/commit/1eccdc8d03f1d6025f7a0d2a176e41c174dfe630))

## [0.36.2](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.36.1...frontend/v0.36.2) (2026-05-12)


### Bug Fixes

* keep arrivals out of departure bays ([4b26c7b](https://github.com/flightstrips/FlightStrips/commit/4b26c7b59a40f54859409c8a074e0c4f9576a70a))

## [0.36.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.36.0...frontend/v0.36.1) (2026-05-12)


### Bug Fixes

* half strips ([31890d7](https://github.com/flightstrips/FlightStrips/commit/31890d7af4b58fba2c90407c0e41ddc29f35a89a))

## [0.36.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.35.2...frontend/v0.36.0) (2026-05-11)


### Features

* **Public Site:** Refactor design ([a853dad](https://github.com/flightstrips/FlightStrips/commit/a853dad45465dfbca14b363da0415f8e0945439b))

## [0.35.2](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.35.1...frontend/v0.35.2) (2026-05-10)


### Bug Fixes

* allow force assume during transfers ([82943e3](https://github.com/flightstrips/FlightStrips/commit/82943e3536da6b30052b29bb70a9ce57252921b0))

## [0.35.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.35.0...frontend/v0.35.1) (2026-05-10)


### Bug Fixes

* allow startup cdm ready from TSAT and CTOT ([0958f9b](https://github.com/flightstrips/FlightStrips/commit/0958f9bcd4371a6757206bcc2dc78ccdf664edc6))
* show next freq under strip callsign ([a6ccc51](https://github.com/flightstrips/FlightStrips/commit/a6ccc5177e3086a3f1cb2a64b5164c7074717b59))

## [0.35.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.34.2...frontend/v0.35.0) (2026-05-10)


### Features

* **ACDM:** expand departure CDM engine ([80fe451](https://github.com/flightstrips/FlightStrips/commit/80fe451dfdcc7dfdc3556f03350bed117b91bd3a))
* **ACDM:** flag invalid phase TOBT ([3e89c41](https://github.com/flightstrips/FlightStrips/commit/3e89c411643f14f4b124ece483370a8d380947cf))
* **ACDM:** prioritize CTOT and sync LVP ([dec7f8e](https://github.com/flightstrips/FlightStrips/commit/dec7f8e9df829210033db044ad0b3334d66c7dd4))
* **cdm:** add standalone sequence page ([4ee51da](https://github.com/flightstrips/FlightStrips/commit/4ee51da5fa1b9a46074c9b0f3e3a74f0d97a54f1))
* **SEQ:** startup ready ([72b66d6](https://github.com/flightstrips/FlightStrips/commit/72b66d62d5e3371d6720b793773c0cc0a59e9d27)), closes [#202](https://github.com/flightstrips/FlightStrips/issues/202)
* support sector-aware next display ([a7915f7](https://github.com/flightstrips/FlightStrips/commit/a7915f7b7c5b14464ac260a5e33b5a449a297c2c))


### Bug Fixes

* **ACDM:** enforce cdm_ready flow ([232d19f](https://github.com/flightstrips/FlightStrips/commit/232d19f1a705fc8ee2119da43025137c4dfd4576))
* add missing stand ([a00b3fb](https://github.com/flightstrips/FlightStrips/commit/a00b3fb07ecfb13ab6b2c1ab34a95bca03700c45))
* **est:** always show CTOT ready background in seq display ([7121864](https://github.com/flightstrips/FlightStrips/commit/7121864bd9cf86ed09665c3216be71c13635f071))
* **est:** improve CTOT contrast on EST board ([2cb5f6e](https://github.com/flightstrips/FlightStrips/commit/2cb5f6e8ae9aa7b73009662fdb5d222fbb3eea3d))
* **est:** keep inbound callsigns visible on seq display ([e6da32f](https://github.com/flightstrips/FlightStrips/commit/e6da32f3d07224ee27d1b09beb85159160c259be))
* **est:** match unoccupied stand color ([39a0e6b](https://github.com/flightstrips/FlightStrips/commit/39a0e6b89e3eaa22ce549be3eccf78862abcf959)), closes [#198](https://github.com/flightstrips/FlightStrips/issues/198)
* **est:** move MRK highlight off CTOT row ([#199](https://github.com/flightstrips/FlightStrips/issues/199) [#200](https://github.com/flightstrips/FlightStrips/issues/200)) ([4b3ba2a](https://github.com/flightstrips/FlightStrips/commit/4b3ba2a04e707da7b09988cd7c3ec74886c2769b))
* require two clicks to show menu ([95a4f75](https://github.com/flightstrips/FlightStrips/commit/95a4f750fbc85bcbecc16d60f433553ace8af5ed))
* scope CTWR frequency to EKCH controllers ([129c34d](https://github.com/flightstrips/FlightStrips/commit/129c34d5497c3074d071c6833b7661d0026c9c10))

## [0.34.2](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.34.1...frontend/v0.34.2) (2026-05-04)


### Features

* Added G117 thru G119 ([8c342b3](https://github.com/flightstrips/FlightStrips/commit/8c342b317e530adb9668d724d3bf7d0e211f86e1))


### Bug Fixes

* F stands are not CARGO ([25861d5](https://github.com/flightstrips/FlightStrips/commit/25861d5f041490b6e4db1e3b3cf32549de2378df))
* **frontend:** prevent reload loop after deployments ([b22a332](https://github.com/flightstrips/FlightStrips/commit/b22a3328c9c9159aaf37c5318c958e1ad758d02d))
* Ghost stands on cargo view removed ([058d8cf](https://github.com/flightstrips/FlightStrips/commit/058d8cf0c5637061fb10dea10e1ab8e4fdd8936f))
* ormalizes owned_sectors to [], resolves broken SEQ PLN view ([07576aa](https://github.com/flightstrips/FlightStrips/commit/07576aa3f7a99955aaab6fd517997e39e4d4b085))
* **stand:** default cargo view only for cargo stands ([03d6cf8](https://github.com/flightstrips/FlightStrips/commit/03d6cf8bf97b311328b2629d066d6b0df5198ab9))
* **startup:** align startup bay ordering with pushback ([91ecf41](https://github.com/flightstrips/FlightStrips/commit/91ecf410e61ca05125ab045582fc4180f8a1f021))
* **strips:** preserve top drop space in filled bays ([c01bf47](https://github.com/flightstrips/FlightStrips/commit/c01bf47d0c4d031e577887f817686e6d87d603f1))

## [0.34.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.34.0...frontend/v0.34.1) (2026-05-03)


### Bug Fixes

* **cdm:** display CDM times with leading zeroes ([43c3e41](https://github.com/flightstrips/FlightStrips/commit/43c3e418d3cda45beb060d3c6b50565148085ea0))
* **strips:** display aircraft registration in flight plan dialog REG field ([d1d86b6](https://github.com/flightstrips/FlightStrips/commit/d1d86b64aaf42e7fb6f0ec8de1c3f094ffd8187f))

## [0.34.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.33.0...frontend/v0.34.0) (2026-05-01)


### Features

* **strips:** gate frontend actions on active validation status ([be251cd](https://github.com/flightstrips/FlightStrips/commit/be251cd8c8c3e206df74d43a17e7c2e5a9a04a71))
* **strips:** send ready message when clicking TOBT on CLR and CLROK strips ([9fc009b](https://github.com/flightstrips/FlightStrips/commit/9fc009bbbbeaf022d47729cfe15301592b58e86a))


### Bug Fixes

* correct transfer cancellation initiator rendering and owned-strip ES arrival handover ([6a0429d](https://github.com/flightstrips/FlightStrips/commit/6a0429d95258a93e8f4a8eb8bab95b60589174ad))
* **frontend:** refresh clients after deployments ([79714af](https://github.com/flightstrips/FlightStrips/commit/79714af107995a1ab9914176790f60c96728bda4))
* **strips:** keep validation callsigns clickable ([e5bbb7d](https://github.com/flightstrips/FlightStrips/commit/e5bbb7d8c394272ca5f7dde4251a27960f624c2b))
* **strips:** show EOBT/CTOT divider based on CTOT presence ([7034d16](https://github.com/flightstrips/FlightStrips/commit/7034d16983cca38ed35f440bb5376120f9f8fb8e))

## [0.33.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.32.0...frontend/v0.33.0) (2026-04-30)


### Features

* **strips:** add CLX dialogue validation ([6e9dac0](https://github.com/flightstrips/FlightStrips/commit/6e9dac0c8ed141cb33ec41246d00c2fcca812c5b))
* **strips:** add editable RNAV capability updates ([b38270e](https://github.com/flightstrips/FlightStrips/commit/b38270e523912442c60df23c8c61166fde1a3099))


### Bug Fixes

* **strips:** align CTOT divider placement ([69e7795](https://github.com/flightstrips/FlightStrips/commit/69e779586123feca19e1b0796a856de589fbcf69))
* **strips:** correct pushback chart controls ([9c2f227](https://github.com/flightstrips/FlightStrips/commit/9c2f2278669e05cde220fa68c6586b3a6971463c)), closes [#189](https://github.com/flightstrips/FlightStrips/issues/189)
* **strips:** improve CLX validation updates ([9362b27](https://github.com/flightstrips/FlightStrips/commit/9362b279d9a0871d9c54047d14005f012cd2e73b))
* **strips:** resize validation status highlights ([8ad7a71](https://github.com/flightstrips/FlightStrips/commit/8ad7a71c54ccaa65fda076fbb90549516d72468f)), closes [#186](https://github.com/flightstrips/FlightStrips/issues/186)
* **strips:** restore pushback direction button width ([c150b33](https://github.com/flightstrips/FlightStrips/commit/c150b33ace70daf3adf8665ca9b30f2b60543e1b)), closes [#189](https://github.com/flightstrips/FlightStrips/issues/189)
* **strips:** send SEQ startup transfers to AD owner ([b2f4232](https://github.com/flightstrips/FlightStrips/commit/b2f423298759bf4c720879cbc65012c55651f5af))
* **strips:** show divider above CTOT rows ([22c865a](https://github.com/flightstrips/FlightStrips/commit/22c865a098f3141cc9b1f1c185c40129f5c9bd98))

## [0.32.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.31.1...frontend/v0.32.0) (2026-04-29)


### Features

* **strips:** add TWR+GND layout ([85b28a5](https://github.com/flightstrips/FlightStrips/commit/85b28a5e1d646e7e886dac6fa01fac78a5db6d72))


### Bug Fixes

* **strips:** restore touch strip dragging ([ba34674](https://github.com/flightstrips/FlightStrips/commit/ba346748cd178128fb52feeeb63170b481ebb127))

## [0.31.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.31.0...frontend/v0.31.1) (2026-04-28)


### Bug Fixes

* correct lable on AD fro SAS traffic ([786570b](https://github.com/flightstrips/FlightStrips/commit/786570b4598e8c8b93839c3354e2873fdff5473f))
* inconsistency between SEQ PLN & EST. ([5fc7c85](https://github.com/flightstrips/FlightStrips/commit/5fc7c852b1cd2a0512bde10be83fc9fdfc6a6c2e))
* order of GW/GE on select screen ([be56652](https://github.com/flightstrips/FlightStrips/commit/be56652a56034e240de8ff70df64b922edd57ec5))
* **strips:** stabilize bay scrolling ([d8cad0d](https://github.com/flightstrips/FlightStrips/commit/d8cad0da24f5bf836513c9e24c3b3405f0a6e78b))


### Performance Improvements

* **strips:** batch EuroScope sync processing ([f1ee21f](https://github.com/flightstrips/FlightStrips/commit/f1ee21fbd64c1e76a820452238d72385d8638e5e))

## [0.31.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.30.0...frontend/v0.31.0) (2026-04-26)


### Features

* **deploy:** prompt users to reload after deploys ([c49ed9f](https://github.com/flightstrips/FlightStrips/commit/c49ed9f6a228fb08f15858e56f72410691079a2b))


### Bug Fixes

* **dialogs:** scale remaining controller dialogs ([d38b9b3](https://github.com/flightstrips/FlightStrips/commit/d38b9b367e1612e82a6fdaf0a7ba3d5d608828e9))
* **strips:** limit runway-arr highlight to runway cell ([b8e07a9](https://github.com/flightstrips/FlightStrips/commit/b8e07a91e521623827fcf650dcb9ae87fdf454fb))
* **strips:** match ARR popup rows to final bays ([6348d45](https://github.com/flightstrips/FlightStrips/commit/6348d45b8e399a5e7b72a54d37df81f39f3234f9))
* **strips:** scale CLX dialogs with viewport ([ade3087](https://github.com/flightstrips/FlightStrips/commit/ade3087d131eede67ce60ddef408bc9c5d8636b2))
* **strips:** use responsive twy tactical widths ([7ae479e](https://github.com/flightstrips/FlightStrips/commit/7ae479e4d356e93d8f0b78a8c514ea6e2efe2429))

## [0.30.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.29.2...frontend/v0.30.0) (2026-04-26)


### Features

* **strip:** route pushback strip to TaxiLwr when tower selects taxi route ([eefeeef](https://github.com/flightstrips/FlightStrips/commit/eefeeefefa06c0032a1fd912a92950324240de4e))


### Bug Fixes

* **layout:** use dvh for viewport-based heights ([4c5838d](https://github.com/flightstrips/FlightStrips/commit/4c5838da6120b111efb7084d7c446b9f781bd5f1))
* **strips:** block validation-locked interactions ([df03a55](https://github.com/flightstrips/FlightStrips/commit/df03a5512f4ba3cc196687fbf3fc70ad6fc23778))
* **strips:** clear arrival runway state on backward move ([b5b7f6c](https://github.com/flightstrips/FlightStrips/commit/b5b7f6cca3f7998e9627abb11e46a5c7e12e4fb5))
* **strips:** keep runway-cleared strips at top of runway bays ([d58dd3a](https://github.com/flightstrips/FlightStrips/commit/d58dd3a3eece1dc5d11b96e426be71be3e83c224))
* **strips:** show dropped SI state on stand ([d397fa9](https://github.com/flightstrips/FlightStrips/commit/d397fa9402709591c5ceccb13250b18797785f86))

## [0.29.2](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.29.1...frontend/v0.29.2) (2026-04-25)


### Bug Fixes

* text color on dark mode ([cb6e97a](https://github.com/flightstrips/FlightStrips/commit/cb6e97a220c886d0ae26841ddce77be928b04edd))

## [0.29.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.29.0...frontend/v0.29.1) (2026-04-25)


### Bug Fixes

* **strips:** allow force assume during validation ([8f4ff48](https://github.com/flightstrips/FlightStrips/commit/8f4ff485509501a2994f6256fd3cded33c5dcf20))
* **strips:** restore req armed flow ([07bd2fc](https://github.com/flightstrips/FlightStrips/commit/07bd2fcca260a8557ededff4a32b3d59acbb4145))

## [0.29.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.28.0...frontend/v0.29.0) (2026-04-25)


### Features

* **strip:** open tower taxi map from pushback strip when release point is set ([6f929f5](https://github.com/flightstrips/FlightStrips/commit/6f929f55d4847df35a8eba9b60c325edc2248c1b))
* **transfer:** filter transfer window to controllers with flight strip sectors ([4f2780b](https://github.com/flightstrips/FlightStrips/commit/4f2780badc7dcdddf8648afde8b5e4b124d6814a))


### Bug Fixes

* **runway:** scope auto-confirm to same runway only ([1546ea8](https://github.com/flightstrips/FlightStrips/commit/1546ea85e370bb441d788f84d94783565c926f18))

## [0.28.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.27.1...frontend/v0.28.0) (2026-04-25)


### Features

* duplicate squawk validation ([a556ebc](https://github.com/flightstrips/FlightStrips/commit/a556ebc6c71ba6456c8d184c122d7dd522987944))
* **frontend:** match validation status designs ([2d531de](https://github.com/flightstrips/FlightStrips/commit/2d531de0d38eb8682e23fefe32b251810dbb2aeb))
* **observer:** add read-only observer mode ([f919dc4](https://github.com/flightstrips/FlightStrips/commit/f919dc49a259610def9366037f4a8e8075d3c555))
* **plugin-sync:** alert runway config mismatches ([e0363ad](https://github.com/flightstrips/FlightStrips/commit/e0363ad35be80a47a2d96d90212916cfd02f274e))
* **validation:** add ctot validation ([e8380c9](https://github.com/flightstrips/FlightStrips/commit/e8380c9419062edab2453cdcd22beacd4020ecd7))
* **validation:** add custom pdc validation ([ea84795](https://github.com/flightstrips/FlightStrips/commit/ea847950d6e3ff992af005a2e0833557519c9cfc))
* **validation:** add invalid pdc validation ([607b354](https://github.com/flightstrips/FlightStrips/commit/607b35444f6a443cdba85200df0c27855a4ec59c))
* **validation:** add landing clearance validation ([61429e5](https://github.com/flightstrips/FlightStrips/commit/61429e5120d434a6727b02fb4f25172bc9ca9169))
* **validation:** add no-stand validation ([0d20fe4](https://github.com/flightstrips/FlightStrips/commit/0d20fe4bb62b13bd44d95af3d5e491b263e9f4cc))
* wrong squawk validation ([026a3cc](https://github.com/flightstrips/FlightStrips/commit/026a3cc1803b925367317c25ae712df3d3d484e4))


### Bug Fixes

* **auth:** prompt login after invalid refresh token ([ab9b7f2](https://github.com/flightstrips/FlightStrips/commit/ab9b7f2c74be4551d4c7207374dc02e99695b017))
* block departure strips in arrival bays ([535ef92](https://github.com/flightstrips/FlightStrips/commit/535ef92d0b90fff078fe16e3976591ee953e03ec))
* **coordination:** remove tag request confirm button ([b5444fe](https://github.com/flightstrips/FlightStrips/commit/b5444fee378a6a0fa1c9ea2284f8060790697eac))
* **frontend:** add map close buttons ([cf2d3a5](https://github.com/flightstrips/FlightStrips/commit/cf2d3a57f8b1337bf5649f2fe3f93bf2fac1dc46))
* **frontend:** confirm off-map requests ([f60274a](https://github.com/flightstrips/FlightStrips/commit/f60274ac7f5da2a7ea6dace0b9f7706bc683cd53))
* **frontend:** restore taxi arrival erase ([163e815](https://github.com/flightstrips/FlightStrips/commit/163e8154244ad74947ef52b3b7506a144dbd6638))
* **strips:** preserve startup pickup flow ([806df8c](https://github.com/flightstrips/FlightStrips/commit/806df8c2bc7adc30bf235b926a5095b84600f548))
* **strips:** restore validation styling cues ([4e9d844](https://github.com/flightstrips/FlightStrips/commit/4e9d8446997566845d70fe97cf288169daab53f3))
* **strips:** stabilize validation blinking ([8609789](https://github.com/flightstrips/FlightStrips/commit/8609789ce41729965fcf3eed1ac6b3c8602dae8a))

## [0.27.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.27.0...frontend/v0.27.1) (2026-04-23)


### Bug Fixes

* **frontend:** prevent websocket token spam ([29cbadc](https://github.com/flightstrips/FlightStrips/commit/29cbadc3e201eacd797af9ee6baa96ee5335d4f1))

## [0.27.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.26.0...frontend/v0.27.0) (2026-04-21)


### Features

* new metrics ([69aa8ef](https://github.com/flightstrips/FlightStrips/commit/69aa8ef1d7fef2e874b042e6f0ef5bb4bcb00274))

## [0.26.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.25.0...frontend/v0.26.0) (2026-04-18)


### Features

* add validation status framework (task-146-00) ([222d7bb](https://github.com/flightstrips/FlightStrips/commit/222d7bbe605c2395c1f02f8dc7836e3fafc7b25d))

## [0.25.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.24.1...frontend/v0.25.0) (2026-04-17)


### Features

* new froundend public site design ([555dae9](https://github.com/flightstrips/FlightStrips/commit/555dae9e3a1f64e1a209ac9de76ca58a3fb19c07))


### Bug Fixes

* build ([b7849d1](https://github.com/flightstrips/FlightStrips/commit/b7849d1c4246ea4735a30190b72e72de41ab49a1))

## [0.24.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.24.0...frontend/v0.24.1) (2026-04-17)


### Bug Fixes

* add .gitattributes to enforce LF for Linux-executed files ([ae4a5d7](https://github.com/flightstrips/FlightStrips/commit/ae4a5d772b5a91f8d77535b9332332b9cda3c82a))

## [0.24.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.23.0...frontend/v0.24.0) (2026-04-17)


### Features

* add web PDC flow ([237f75a](https://github.com/flightstrips/FlightStrips/commit/237f75a4fb7e50bf19580e54528b288a3f50531f))

## [0.23.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.22.0...frontend/v0.23.0) (2026-04-16)


### Features

* open app button in ES ([76c9332](https://github.com/flightstrips/FlightStrips/commit/76c933217b5de374106e1d0c8c609e6505c729ae))
* top down coverage ([48ddb6e](https://github.com/flightstrips/FlightStrips/commit/48ddb6ed52e1a1848093437aeae86417b0d89bfb))

## [0.22.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.21.1...frontend/v0.22.0) (2026-04-14)


### Features

* remove right-click menu ([75709d6](https://github.com/flightstrips/FlightStrips/commit/75709d6de46c8438738471c255a9c76adac4bdc5))

## [0.21.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.21.0...frontend/v0.21.1) (2026-04-14)


### Bug Fixes

* missing scroll ([d91e8ef](https://github.com/flightstrips/FlightStrips/commit/d91e8ef11b634ae2033b84700964b14d3bab723c))

## [0.21.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.20.0...frontend/v0.21.0) (2026-04-12)


### Features

* sync go around sound ([b4f6482](https://github.com/flightstrips/FlightStrips/commit/b4f64827dc7dd6dbbf42216c595bc64372030e6c))

## [0.20.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.19.1...frontend/v0.20.0) (2026-04-11)


### Features

* Detect landing and move strip to TWY ARR when vacated ([a12892d](https://github.com/flightstrips/FlightStrips/commit/a12892d29032ff3b30a10df2a125cf27ae2ed1b6))
* erase heading, cleared altitude ([37c2360](https://github.com/flightstrips/FlightStrips/commit/37c236046edf6545ed5e35ca1f625561cfce7f73))


### Bug Fixes

* MRK Btn size ([254d8f8](https://github.com/flightstrips/FlightStrips/commit/254d8f89cb7ef5565d3b5bddca633d834650865a))
* remove commandbar buttom border ([5390b17](https://github.com/flightstrips/FlightStrips/commit/5390b179effde170b082802335868ee9ecd434fd))

## [0.19.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.19.0...frontend/v0.19.1) (2026-04-07)


### Bug Fixes

* able to move when not owned ([605a05f](https://github.com/flightstrips/FlightStrips/commit/605a05f0691ade43842eef82acbc8745de2d1284))
* wrong callsign prefix for norwegian bay ([a727f1e](https://github.com/flightstrips/FlightStrips/commit/a727f1e68c75077b782a24b1848775ad757fd884))

## [0.19.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.18.0...frontend/v0.19.0) (2026-03-31)


### Features

* cargo stands ([373a9c3](https://github.com/flightstrips/FlightStrips/commit/373a9c315a9ec5020faaeefb5a9dd35ea9d995ce))
* enable movement of tactical strips ([1528c8d](https://github.com/flightstrips/FlightStrips/commit/1528c8df2d968f15048ae1de46bd8566088acacf))


### Bug Fixes

* coordination of points ([89e40a2](https://github.com/flightstrips/FlightStrips/commit/89e40a2828d287f0fce150ebadf0d369ec649df8))
* CTOT display and TOBT color ([429f6be](https://github.com/flightstrips/FlightStrips/commit/429f6be1bef2b250ae65b9b53a079f57634a2fe5))
* only display PDC backend in certain bays ([42b2bbc](https://github.com/flightstrips/FlightStrips/commit/42b2bbc32c00b9f5e9efd14d2c65c545bd11c313))
* remove shadows from bay headers [#145](https://github.com/flightstrips/FlightStrips/issues/145) ([aedacfb](https://github.com/flightstrips/FlightStrips/commit/aedacfb90f1962b27f4c353dc7e82559e5fc5f53))
* style memaid popup ([e1f615d](https://github.com/flightstrips/FlightStrips/commit/e1f615d186f0f5d58f76690de86d77018e6a79cb))
* tactical strip length ([a8cb157](https://github.com/flightstrips/FlightStrips/commit/a8cb15709b2a5ab3beba3c875da58bf217f4cad3))
* taxi maps ([abe59b6](https://github.com/flightstrips/FlightStrips/commit/abe59b6670bc46337b93b7f548369c88673ea820))

## [0.18.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.17.3...frontend/v0.18.0) (2026-03-29)


### Features

* add category type aicraft type ([d858a4f](https://github.com/flightstrips/FlightStrips/commit/d858a4f404a1d87d8bb2ff66b902cd8fd5d76d13))
* added web-pdc ([6fdc590](https://github.com/flightstrips/FlightStrips/commit/6fdc5904a91e51dd1d595983b0137d9adadb8379))
* Adjust EST view ([c2b607c](https://github.com/flightstrips/FlightStrips/commit/c2b607cc19c2e4348280ef6330ee1458187f5305))
* auto set altitude ([e0332a5](https://github.com/flightstrips/FlightStrips/commit/e0332a5abb15d001e36361a55b40e381cdb9bc5a))
* confirm delete strip ([cb4837c](https://github.com/flightstrips/FlightStrips/commit/cb4837c0fd1178204b5caf5291c2954bd6874120))


### Bug Fixes

* correct labels for TETW, GEGW ([6bce70a](https://github.com/flightstrips/FlightStrips/commit/6bce70a02a4eff81b5e4747bf1375694e445053f))
* missing ctot and missing pdc status color ([b035326](https://github.com/flightstrips/FlightStrips/commit/b03532604f74491617f8b4d26aff6e26180e321b))
* pdc text color ([d749ed7](https://github.com/flightstrips/FlightStrips/commit/d749ed791101b43631b59afbf8ede19c7fc4446f))
* reloading when moving strips from arr or startup ([15e4ea8](https://github.com/flightstrips/FlightStrips/commit/15e4ea875c50ded49acc08005129ae79bf9d5f43))
* scroll main pages and disable select for most page ([8a16ceb](https://github.com/flightstrips/FlightStrips/commit/8a16ceb68dddea5e31e6288423ae64604d469add))
* switch everything to view based sizes ([22b3eda](https://github.com/flightstrips/FlightStrips/commit/22b3eda9ef3f79054addb11d0bff1051dcca2f5b))
* use correct strip on GEGW view [#132](https://github.com/flightstrips/FlightStrips/issues/132) ([d11d7b3](https://github.com/flightstrips/FlightStrips/commit/d11d7b3c8ac0e3788b889f8d34b3c4ffc19dec1b))

## [0.17.3](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.17.2...frontend/v0.17.3) (2026-03-28)


### Bug Fixes

* a third hotfix ([c952f43](https://github.com/flightstrips/FlightStrips/commit/c952f43641a34f9e3220fe9ec7ee4545d0c9a008))

## [0.17.2](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.17.1...frontend/v0.17.2) (2026-03-28)


### Bug Fixes

* another hotfix ([273eaed](https://github.com/flightstrips/FlightStrips/commit/273eaedd48dc1be62a2eee873558fb7838549b80))

## [0.17.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.17.0...frontend/v0.17.1) (2026-03-28)


### Bug Fixes

* touch hot-fix ([a87e4aa](https://github.com/flightstrips/FlightStrips/commit/a87e4aaa3dae15525384afe8b07dddf4cbac2eb6))

## [0.17.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.16.1...frontend/v0.17.0) (2026-03-28)


### Features

* borders and shadows ([00bcc8c](https://github.com/flightstrips/FlightStrips/commit/00bcc8cacaffeb5344dc1085eafd64ced0cf0cbc))
* gaurd metar decoding behind S3 rating or higher ([e7bcf94](https://github.com/flightstrips/FlightStrips/commit/e7bcf94e4d57f9221001fd47d1466f20fbae7deb))
* missed approach ([c12e3b5](https://github.com/flightstrips/FlightStrips/commit/c12e3b503b46a04749245f967bf5296e6c4097f3))
* open FPL on all strips ([2c7dfec](https://github.com/flightstrips/FlightStrips/commit/2c7dfecb383e421674fe89fdb2f803860f29e8d8))
* Runway status ([3514fa6](https://github.com/flightstrips/FlightStrips/commit/3514fa6b7feae51b6db9044c7560c7d6318a841e))


### Bug Fixes

* bay colors ([6926e02](https://github.com/flightstrips/FlightStrips/commit/6926e023ef9b85e93894a1de4e85d7246eea8915))
* borders on bays ([e062850](https://github.com/flightstrips/FlightStrips/commit/e062850eef69400461a236661bddea520c931439))
* click point for callsign ([bdb6893](https://github.com/flightstrips/FlightStrips/commit/bdb6893f4ab47988b583c6786f9a2e1bbb184359))
* de-ice bay heights ([4b469a3](https://github.com/flightstrips/FlightStrips/commit/4b469a3ad29ed2a5d04fdcd4b2ee3277c4822437))
* header borders and shadows ([eee0486](https://github.com/flightstrips/FlightStrips/commit/eee048687f0dbb4da7e8af73eacd71306afcfb9f))
* SI unconcerned color ([3f44eff](https://github.com/flightstrips/FlightStrips/commit/3f44effbd4c4cd00968f5fe89ee2e2a58f0535fc))

## [0.16.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.16.0...frontend/v0.16.1) (2026-03-27)


### Bug Fixes

* bay ordering ([d81cd5c](https://github.com/flightstrips/FlightStrips/commit/d81cd5c5bbffcfd4f40e5622b0674966b30edba9))
* bay ordering ([9c0c640](https://github.com/flightstrips/FlightStrips/commit/9c0c640eb55aecf306e249dea1b87469d1c82c2a))
* confirmed runway strips no longer turn red when new strip arrives ([bf87d57](https://github.com/flightstrips/FlightStrips/commit/bf87d57bfd6b6e373c1c237fa51bbd5af37473e2))
* Disable Callsign Selection in CLR DEL ([82c172f](https://github.com/flightstrips/FlightStrips/commit/82c172f5a42449f3518636348b28011e0e1ddc74))
* make command bar smaller ([2cec0f9](https://github.com/flightstrips/FlightStrips/commit/2cec0f94ec216f181ec4c94334943ffc380dff56))
* remove selection from non-cleared strips ([e351d36](https://github.com/flightstrips/FlightStrips/commit/e351d36eca060e753bff69e1191d09ed722d55e9))
* strip sizes ([26d2454](https://github.com/flightstrips/FlightStrips/commit/26d2454761a15538d4bf78356bbfb786862b501e))

## [0.16.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.15.2...frontend/v0.16.0) (2026-03-24)


### Features

* Add PWA ([1239c3a](https://github.com/flightstrips/FlightStrips/commit/1239c3a1cf2f9e1fbeb43cdfd8d5f22244ea2778))

## [0.15.2](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.15.1...frontend/v0.15.2) (2026-03-24)


### Bug Fixes

* only change layout first time ([9a95ac9](https://github.com/flightstrips/FlightStrips/commit/9a95ac93c8ca1459cc727ebfc4bde3595497fad8))

## [0.15.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.15.0...frontend/v0.15.1) (2026-03-24)


### Bug Fixes

* use SI box component for all strips ([7e99857](https://github.com/flightstrips/FlightStrips/commit/7e998578e3989c5fa3374607355ffc1d54dd5cbc))

## [0.15.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.14.1...frontend/v0.15.0) (2026-03-23)


### Features

* AD + AA views ([cca9e87](https://github.com/flightstrips/FlightStrips/commit/cca9e871ee6d11626e5c2afd87da2742d0ed16bc))
* highlight wrong squawks ([de7a780](https://github.com/flightstrips/FlightStrips/commit/de7a780cb6756637a23cf68fd0e51f1f85006677))
* Unconcerned strips ([c8ec0e0](https://github.com/flightstrips/FlightStrips/commit/c8ec0e09b8a13c57bd6eee7f72fe9c0b73876045))

## [0.14.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.14.0...frontend/v0.14.1) (2026-03-22)


### Bug Fixes

* build problems ([063ce9e](https://github.com/flightstrips/FlightStrips/commit/063ce9eaeddc6d017da3f196fdce7a7cc137a4c9))

## [0.14.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.13.1...frontend/v0.14.0) (2026-03-22)


### Features

* GEGW, commandbar + small fixes ([bf4d091](https://github.com/flightstrips/FlightStrips/commit/bf4d0910fc39baf35538b0b1da5d931f15963fc6))


### Bug Fixes

* atis ([63e0b23](https://github.com/flightstrips/FlightStrips/commit/63e0b230ba629365c2d63e9f68bcab866b8f5e97))
* display TWY on click point ([0037c9f](https://github.com/flightstrips/FlightStrips/commit/0037c9f37e6e46367c66da67b1ef8a076175357e))
* small fixes on GEGW view ([4516ad3](https://github.com/flightstrips/FlightStrips/commit/4516ad35c7e551c25281b05921c8e2f4509be41a))

## [0.13.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.13.0...frontend/v0.13.1) (2026-03-22)


### Bug Fixes

* AAAD bay widths ([08e48dc](https://github.com/flightstrips/FlightStrips/commit/08e48dc923bb5262e52ff8cc52390ee9b090d757))
* AAAD headers view ([10609c7](https://github.com/flightstrips/FlightStrips/commit/10609c7b255965277783e5f5e8464aeab9ca91fc))
* AAAD TWY DEP strip design ([cf6899f](https://github.com/flightstrips/FlightStrips/commit/cf6899fc9af07ff0dcd42360615b2cdf4efeb8cf))
* minor ui improvements and twr taxi map ([6440a85](https://github.com/flightstrips/FlightStrips/commit/6440a8512ff73e73973d68e41b11e9efa99b826e))
* only render simple part of aircraft type ([df2635e](https://github.com/flightstrips/FlightStrips/commit/df2635ec670c42a8dcf3dceb12c718b7c32fc879))

## [0.13.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.12.0...frontend/v0.13.0) (2026-03-20)


### Features

* add force assume strip command ([f3c68a9](https://github.com/flightstrips/FlightStrips/commit/f3c68a920aed45662a749032eee66a33abdcce87))
* add non-dismissable layout chooser screen ([29beb55](https://github.com/flightstrips/FlightStrips/commit/29beb55a0d4071ed738b810df58dd2be8ccdabe3))
* Adde AHDG dropdown and updated store ([2890d2e](https://github.com/flightstrips/FlightStrips/commit/2890d2e947f84209cc8c10cc81dcbfb9c09f99fb))
* Added ALT dropdown & updated store ([0ab16ba](https://github.com/flightstrips/FlightStrips/commit/0ab16ba363beb92beacd353f4b911836c8da506f))
* Correct PDC ([8930e27](https://github.com/flightstrips/FlightStrips/commit/8930e27512f251c756ce38395989be0f2a5666c5))
* Create IFR / VFR flightplan ([07a158b](https://github.com/flightstrips/FlightStrips/commit/07a158b4fc96059fcf77e3002f1ea517f914c443))
* enforce strip ownership when moving strips ([2fb702d](https://github.com/flightstrips/FlightStrips/commit/2fb702df44cbc011655a7b88dca167743c2918d1))
* gate frontend connections behind active euroscope client ([4f06f3d](https://github.com/flightstrips/FlightStrips/commit/4f06f3dae917c312bfff3f5be95453014994e07c))
* Pull ATIS if available ([92fa0b2](https://github.com/flightstrips/FlightStrips/commit/92fa0b22e75f3f8ace6501785d23c645a6dba76b))
* Request strips ([a9d1a46](https://github.com/flightstrips/FlightStrips/commit/a9d1a46407e75708e0b3f35776672bbb7b8e4771))
* right-click ([e23e8a2](https://github.com/flightstrips/FlightStrips/commit/e23e8a2afdef6bbc9b0732390a511be250385221))
* **sids:** source available SIDs from EuroScope sync event ([43a1f1f](https://github.com/flightstrips/FlightStrips/commit/43a1f1f6eaa82bbb854a4967f7a5bf8e5705e8bd))
* Simulate CDM ([8999c0e](https://github.com/flightstrips/FlightStrips/commit/8999c0e6f743f238080354af96daa7d7e793de92))
* veiw flightplan ([30102dc](https://github.com/flightstrips/FlightStrips/commit/30102dc97d51b4b3bc0b0dceb512518fdde0a945))


### Bug Fixes

* able to move non-owned strip ([6f18fba](https://github.com/flightstrips/FlightStrips/commit/6f18fba7ec71ddfdbfc7634e1562240e6ff2aa80))
* allow frontend to wait for ES connection ([c4641ea](https://github.com/flightstrips/FlightStrips/commit/c4641eaeee43ca08619cbc401a611e707b3afd1b))
* border colors ([1626539](https://github.com/flightstrips/FlightStrips/commit/162653969e50940a1d0ecca767723fbb0309050a))
* broadcast bulk bay event on strip sequence recalculation ([2e1c0ca](https://github.com/flightstrips/FlightStrips/commit/2e1c0ca091ffedb092c214bb68ae638c966ce92c))
* CDM colors ([7c26519](https://github.com/flightstrips/FlightStrips/commit/7c26519457a9143167ee899016d85ab61b58e3ea))
* correct bay names ([fc1f085](https://github.com/flightstrips/FlightStrips/commit/fc1f085ea3318359d7c83feff67c0c144d53900c))
* disingenuous between FP and no FP ([aa87d4f](https://github.com/flightstrips/FlightStrips/commit/aa87d4f39661a2286cee90e09d647af10b7a5cd1))
* force assume ([de96249](https://github.com/flightstrips/FlightStrips/commit/de9624995a38ecbf11a0301d3325943368570798))
* handle errors on backend and frontend ([a8cda2a](https://github.com/flightstrips/FlightStrips/commit/a8cda2a610f2980efa5e42a56f6d4f24eda77649))
* lint issues ([5d27cd2](https://github.com/flightstrips/FlightStrips/commit/5d27cd2fce95ecba3981f290111b6ae50ddfb1bd))
* tactical strip size ([e3ad819](https://github.com/flightstrips/FlightStrips/commit/e3ad819b4419fb26acbb51ab072991f47b7d10e6))
* unknown layout screen ([c1d227d](https://github.com/flightstrips/FlightStrips/commit/c1d227d05b9a19d8dd47e9d47276d996f5efbc9f))
* wrong strip color for stand bay ([049bcf4](https://github.com/flightstrips/FlightStrips/commit/049bcf4e65afd1f30768bbb96cd8bae08940d6c5))

## [0.12.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.11.1...frontend/v0.12.0) (2026-03-15)


### Features

* Added sid selection mockup in flightplan view ([374574d](https://github.com/flightstrips/FlightStrips/commit/374574dbbb9566ea7d2dfccdad34d42189dc777c))
* Added UX feedback for new SSR codes ([f22f4c3](https://github.com/flightstrips/FlightStrips/commit/f22f4c35279ef8c10c4372eb1e3d5e4018bb348a))
* flightplan dialog, stand change is now possible ([bc73d65](https://github.com/flightstrips/FlightStrips/commit/bc73d657eaa3d863fa8812e4105d8db2d88d72f9))
* refactored ATIS ([c2a2fa2](https://github.com/flightstrips/FlightStrips/commit/c2a2fa2a8fd42580ce4a15730f401b330acac93f))


### Bug Fixes

* refactor sid selector ([10bbfe5](https://github.com/flightstrips/FlightStrips/commit/10bbfe5a2a11c13e03a49f8bfe970201c451a7c8))
* Runway dialog box fonts ([4a4cc30](https://github.com/flightstrips/FlightStrips/commit/4a4cc3026652393bcd69dd0b0af7f349274c22c8))

## [0.11.1](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.11.0...frontend/v0.11.1) (2026-03-15)


### Bug Fixes

* build again ([ab47d3a](https://github.com/flightstrips/FlightStrips/commit/ab47d3abc83d71080460e6e441ab2a9e7f46ba41))
* build and lint problems ([15fb5e4](https://github.com/flightstrips/FlightStrips/commit/15fb5e4fa0766d463d8f85decf1092c143efb1cc))
* Vatsim auth ([95993da](https://github.com/flightstrips/FlightStrips/commit/95993da6755b263eb18c3b75fec1ccb3c6120302))

## [0.11.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.10.0...frontend/v0.11.0) (2026-03-14)


### Features

* Added favicon ([116a163](https://github.com/flightstrips/FlightStrips/commit/116a163d03ee22f0e9ebf4766afb518959da11a1))

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
* EST View ([e715a81](https://github.com/flightstrips/FlightStrips/commit/e715a8115fc99a0fd5d245f1d3e7fc6f867167f3))

## [0.2.0](https://github.com/flightstrips/FlightStrips/compare/frontend/v0.1.0...frontend/v0.2.0) (2026-03-07)


### Features

* Add CDM colors ([096b2ac](https://github.com/flightstrips/FlightStrips/commit/096b2acabe1caee1b6c8f1176754d5552499d4bc))

## 0.1.0 (2026-03-07)


### Features

* Add basic CDM implentation ([146de8c](https://github.com/flightstrips/FlightStrips/commit/146de8c4d38f9aaa5958372264d3d86fc57c63b8))
* global release ([33b3d8e](https://github.com/flightstrips/FlightStrips/commit/33b3d8e73cc66f18b2aaba2e47756186625feeab))
* Implment basic PDC ([#72](https://github.com/flightstrips/FlightStrips/issues/72)) ([55010f5](https://github.com/flightstrips/FlightStrips/commit/55010f540b97bf3e84cdd5c0f25339d07f8f9184))
* Support release points ([41d964e](https://github.com/flightstrips/FlightStrips/commit/41d964eae2c18d6386b292dccfd251961b31bf4f))
