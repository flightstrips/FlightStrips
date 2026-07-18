# Changelog

## [0.37.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.36.0...backend/v0.37.0) (2026-07-18)


### Features

* add authenticated EFB behind feature flag ([#348](https://github.com/flightstrips/FlightStrips/issues/348)) ([b9fb84e](https://github.com/flightstrips/FlightStrips/commit/b9fb84ea9522a26cc9090468713568bef5be5c30))
* **sat:** add local test console ([#361](https://github.com/flightstrips/FlightStrips/issues/361)) ([782afae](https://github.com/flightstrips/FlightStrips/commit/782afae462081ddce1c2a32d5da07ee6e8c66c1e))
* send stand assignment ([#364](https://github.com/flightstrips/FlightStrips/issues/364)) ([ca957f2](https://github.com/flightstrips/FlightStrips/commit/ca957f2288e17ecceeeb45107f3348b255a10a44))
* **stand:** add system status diagnostics ([#358](https://github.com/flightstrips/FlightStrips/issues/358)) ([8f926b3](https://github.com/flightstrips/FlightStrips/commit/8f926b35f762968a5906d8dd9abaa46943698f28))
* **strip:** show arrival STAR in EFB ([#353](https://github.com/flightstrips/FlightStrips/issues/353)) ([d2431cb](https://github.com/flightstrips/FlightStrips/commit/d2431cb37783691d35fc9d632586bf2f82361583))
* **strip:** store arrival STAR ([#347](https://github.com/flightstrips/FlightStrips/issues/347)) ([02747f2](https://github.com/flightstrips/FlightStrips/commit/02747f28c25f9fd5e83655392164fdcbc49c5388))


### Bug Fixes

* **cdm:** align EST startup states ([#354](https://github.com/flightstrips/FlightStrips/issues/354)) ([3567888](https://github.com/flightstrips/FlightStrips/commit/3567888699c5134c393a8b53ea82395f2b10ab59))
* **cdm:** preserve TSAT after startup ([#345](https://github.com/flightstrips/FlightStrips/issues/345)) ([bf95c40](https://github.com/flightstrips/FlightStrips/commit/bf95c40a52939d2ed5a94dd2e7efe08f1e84d87d))
* **cdm:** restore EOBT clamping during strip sync ([#346](https://github.com/flightstrips/FlightStrips/issues/346)) ([547749e](https://github.com/flightstrips/FlightStrips/commit/547749e45526616fbe31b32de1e33c48c5042747))
* **cdm:** synchronize startup request timing ([#355](https://github.com/flightstrips/FlightStrips/issues/355)) ([53db077](https://github.com/flightstrips/FlightStrips/commit/53db07714180cd5afae83e21dc77f55e39f36674))
* **pdc:** allow Web PDC without Hoppie ([#359](https://github.com/flightstrips/FlightStrips/issues/359)) ([886e502](https://github.com/flightstrips/FlightStrips/commit/886e50245c601039a3651395a893ebe3b3e7cb67))
* **pdc:** clear pending state on EuroScope clearance ([#351](https://github.com/flightstrips/FlightStrips/issues/351)) ([28e6e78](https://github.com/flightstrips/FlightStrips/commit/28e6e78a9fbc95c31a0fad771bf3dd60aa0f470e))
* **sat:** complete airline stand assignment rules ([#360](https://github.com/flightstrips/FlightStrips/issues/360)) ([0f61b1c](https://github.com/flightstrips/FlightStrips/commit/0f61b1c06753c313b7a4df42d2062cee94f9c9b6))
* stand allocation blocks and retries ([#363](https://github.com/flightstrips/FlightStrips/issues/363)) ([03afd4c](https://github.com/flightstrips/FlightStrips/commit/03afd4c10adb0c4d20bea4fbdcc77e80cfc89d64))
* **stand:** synchronize lifecycle removals and adjacency ([#340](https://github.com/flightstrips/FlightStrips/issues/340)) ([eed49fb](https://github.com/flightstrips/FlightStrips/commit/eed49fb998d0543c3e2a7c7b272043bf3a4d8e26))
* Test console provide correct snapshot time ([#362](https://github.com/flightstrips/FlightStrips/issues/362)) ([6e253b9](https://github.com/flightstrips/FlightStrips/commit/6e253b9d25e522a9ec98dd07a685587732152d44))
* **websocket:** normalize EuroScope disconnect logs ([#349](https://github.com/flightstrips/FlightStrips/issues/349)) ([ecd1959](https://github.com/flightstrips/FlightStrips/commit/ecd19592abacd980189a4381ac7e44f409c30f4c))

## [0.36.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.35.1...backend/v0.36.0) (2026-07-12)


### Features

* **sat:** add validated airline assignment schema ([#275](https://github.com/flightstrips/FlightStrips/issues/275)) ([be3f7c6](https://github.com/flightstrips/FlightStrips/commit/be3f7c6e3390be342cc46073765d812fa0bc8bf2))
* **sat:** import aircraft reference data ([#270](https://github.com/flightstrips/FlightStrips/issues/270)) ([2e9bdb6](https://github.com/flightstrips/FlightStrips/commit/2e9bdb669d8a9e81f1b25bdc96d303fb64c33d9b))
* **sat:** import stand capability data ([#272](https://github.com/flightstrips/FlightStrips/issues/272)) ([8cc10ea](https://github.com/flightstrips/FlightStrips/commit/8cc10ea93a5ec0ef71889a982dc3fd0cea572971))
* **sat:** persist stand assignments and blocks ([#274](https://github.com/flightstrips/FlightStrips/issues/274)) ([8088d5b](https://github.com/flightstrips/FlightStrips/commit/8088d5bf1d5dff3a17f76a9f4f31c855ef3b1376))
* **stand:** add observability and replay ([#290](https://github.com/flightstrips/FlightStrips/issues/290)) ([22248f7](https://github.com/flightstrips/FlightStrips/commit/22248f70eb32acc8e295c043d7b1102b88b03080))
* **stand:** add strip assignment workflow ([#289](https://github.com/flightstrips/FlightStrips/issues/289)) ([376ee54](https://github.com/flightstrips/FlightStrips/commit/376ee545f30aa92e5c548a579bcbb2717db10cd6))
* **stand:** allocate stands transactionally ([#282](https://github.com/flightstrips/FlightStrips/issues/282)) ([d08eb51](https://github.com/flightstrips/FlightStrips/commit/d08eb516ae522700ab2e740f4f43974339e6c213))
* **stand:** calculate arrival ETA ([#283](https://github.com/flightstrips/FlightStrips/issues/283)) ([b34e6c9](https://github.com/flightstrips/FlightStrips/commit/b34e6c9f26fe8c93a0b54f1cb530de31397338e4))
* **stand:** complete SAT controller integration ([#287](https://github.com/flightstrips/FlightStrips/issues/287)) ([f35bcca](https://github.com/flightstrips/FlightStrips/commit/f35bcca32dc4bcfa4b78193d47bd42b5b7d4e777))
* **stand:** evaluate physical compatibility ([#277](https://github.com/flightstrips/FlightStrips/issues/277)) ([d3a2ed0](https://github.com/flightstrips/FlightStrips/commit/d3a2ed0c4d86839243036ba3d1a1f9ad54bd5696))
* **stand:** handle wrong stand departures ([#288](https://github.com/flightstrips/FlightStrips/issues/288)) ([b9024e7](https://github.com/flightstrips/FlightStrips/commit/b9024e78a33c2e4c440eb26068e5c2c05afe08c3))
* **stand:** implement arrival stage lifecycle ([#285](https://github.com/flightstrips/FlightStrips/issues/285)) ([3313fa1](https://github.com/flightstrips/FlightStrips/commit/3313fa12239160707262c29e07bfd34280c041b1))
* **stand:** implement departure reservation lifecycle ([#284](https://github.com/flightstrips/FlightStrips/issues/284)) ([05ddd6c](https://github.com/flightstrips/FlightStrips/commit/05ddd6cd7d750adf3ffe85667c8aa2ff98951721))
* **stand:** integrate EST board with SAT backend assignment and block metadata ([#286](https://github.com/flightstrips/FlightStrips/issues/286)) ([d37e8a9](https://github.com/flightstrips/FlightStrips/commit/d37e8a9445c8ad09b98aca5c2e411f92e5830147))
* **stand:** resolve flight compatibility facts ([#276](https://github.com/flightstrips/FlightStrips/issues/276)) ([fd85b0c](https://github.com/flightstrips/FlightStrips/commit/fd85b0c123027ac152eb763f359d6041bb06d7b2))
* **stand:** select weighted assignment tiers ([#280](https://github.com/flightstrips/FlightStrips/issues/280)) ([1022577](https://github.com/flightstrips/FlightStrips/commit/10225776f9f622cadb1526c90f2d2ca6db08adde))
* **vatsim:** expand feed cache snapshots ([#278](https://github.com/flightstrips/FlightStrips/issues/278)) ([dda9655](https://github.com/flightstrips/FlightStrips/commit/dda96558f1a711ee64c277db57edf38965f5e3a0))
* **vatsim:** reconcile flights into sessions ([#281](https://github.com/flightstrips/FlightStrips/issues/281)) ([5287dac](https://github.com/flightstrips/FlightStrips/commit/5287dac340c466b5090c2f9a0da4cffb6ff1bcd6))


### Bug Fixes

* **cdm:** initialize new sessions as master ([#291](https://github.com/flightstrips/FlightStrips/issues/291)) ([8eb06e0](https://github.com/flightstrips/FlightStrips/commit/8eb06e02d85e229d8ea5c7eb04f180d7c15c9f6c))
* **cdm:** prioritize confirmed TOBT flights ([#268](https://github.com/flightstrips/FlightStrips/issues/268)) ([d0cba64](https://github.com/flightstrips/FlightStrips/commit/d0cba64e676f3262e74195056bad342e5c239c8b))
* **cdm:** restrict sequencing to EuroScope flights ([#292](https://github.com/flightstrips/FlightStrips/issues/292)) ([fd10ea6](https://github.com/flightstrips/FlightStrips/commit/fd10ea646e7799eead6b2aa9f3ac057ea8af8383))
* **cdm:** stabilize midnight sequence recalculation test ([#279](https://github.com/flightstrips/FlightStrips/issues/279)) ([95cf074](https://github.com/flightstrips/FlightStrips/commit/95cf074c5dae1e0100b9705d1e58f6dd15636ae9))
* **stand:** hide offline departures from CLX ([#294](https://github.com/flightstrips/FlightStrips/issues/294)) ([13b7d83](https://github.com/flightstrips/FlightStrips/commit/13b7d83f1a777402b97a2262aac27b58fce7d970))
* **stand:** keep VATSIM-only departures hidden ([#295](https://github.com/flightstrips/FlightStrips/issues/295)) ([66f4dc7](https://github.com/flightstrips/FlightStrips/commit/66f4dc7f2181f287d30eed621069da1980801d15))
* **stand:** normalize VATSIM aircraft types ([#296](https://github.com/flightstrips/FlightStrips/issues/296)) ([7b57f4b](https://github.com/flightstrips/FlightStrips/commit/7b57f4bf3ff27dfaa2a045f4c0e604b091c83fea))
* **stand:** occupy stands for local aircraft ([#293](https://github.com/flightstrips/FlightStrips/issues/293)) ([b0b633c](https://github.com/flightstrips/FlightStrips/commit/b0b633cf45815adf138aad357a393f0ffaedc3af))

## [0.35.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.35.0...backend/v0.35.1) (2026-07-08)


### Bug Fixes

* **pdc:** sync web clearance state with strips ([#263](https://github.com/flightstrips/FlightStrips/issues/263)) ([020abac](https://github.com/flightstrips/FlightStrips/commit/020abac9370591881892740243ee4d2e5bd0966d))

## [0.35.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.34.0...backend/v0.35.0) (2026-07-07)


### Features

* add PrivateMessageSender and send_private_message event relay ([1ae21c7](https://github.com/flightstrips/FlightStrips/commit/1ae21c75ecc6a4e7f03a7ab70bcbd386e2e48a78)), closes [#221](https://github.com/flightstrips/FlightStrips/issues/221)
* **callsigns:** source spoken callsigns from the Euroscope plugin ([#234](https://github.com/flightstrips/FlightStrips/issues/234)) ([5354294](https://github.com/flightstrips/FlightStrips/commit/5354294c0afa48fb53174120a457bd49783680b5))
* **cdm:** enable CDM master by default for new sessions ([6f04f2a](https://github.com/flightstrips/FlightStrips/commit/6f04f2ab13a4ca0743f83835b804844a3a07e174))
* **CDM:** use CDM slot label for CTOT display ([#233](https://github.com/flightstrips/FlightStrips/issues/233)) ([8070a97](https://github.com/flightstrips/FlightStrips/commit/8070a97f08851a6e233dbb0428463e9289e7d968))
* Confirm voice clearance on cleared PDC moves ([#240](https://github.com/flightstrips/FlightStrips/issues/240)) ([6fdd6e8](https://github.com/flightstrips/FlightStrips/commit/6fdd6e8ebdd49c3223c75094e73c75b73f9731ad))
* **ecfmp:** add ECFMP API client, models, and restriction matcher ([6484810](https://github.com/flightstrips/FlightStrips/commit/6484810c27698f658bbfd07efcfc418aad1cdd41)), closes [#221](https://github.com/flightstrips/FlightStrips/issues/221)
* **ecfmp:** add test endpoints ([47cd871](https://github.com/flightstrips/FlightStrips/commit/47cd8715493272455eb5dd8058078e0c63e5c676))
* **ecfmp:** integrate ECFMP restrictions into CDM data model and events ([3f6ad7e](https://github.com/flightstrips/FlightStrips/commit/3f6ad7e544d60781dea0c7e3f9d3013aa1f644c4)), closes [#221](https://github.com/flightstrips/FlightStrips/issues/221)
* **es-plugin:** add ECFMP restriction fields to CdmState and tag items ([5ffe133](https://github.com/flightstrips/FlightStrips/commit/5ffe133c950c08776b2ccc88d11f31cfd0d2faf8)), closes [#221](https://github.com/flightstrips/FlightStrips/issues/221)
* **pdc:** enable mandatory route clearance flow permanently ([81319f9](https://github.com/flightstrips/FlightStrips/commit/81319f9da0b0681094b308b06f71c9dce9b85b9b))
* **pdc:** implement mandatory route clearance flow behind feature flag ([a829e39](https://github.com/flightstrips/FlightStrips/commit/a829e39cc356eaa12d5bba09fd6634389667cb9b))
* **pdc:** reject clearance with no SID or vectors ([1873260](https://github.com/flightstrips/FlightStrips/commit/18732606713d03c2d590f7c273c72a6e69541528))
* **pilot:** Expand pilot flight details ([cf4c5c8](https://github.com/flightstrips/FlightStrips/commit/cf4c5c8e2b56ea7c5fa2b31dce7db8431d6ebd88))


### Bug Fixes

* **backend:** reduce controller offline grace period from 60s to 15s ([#232](https://github.com/flightstrips/FlightStrips/issues/232)) ([3c0f4ee](https://github.com/flightstrips/FlightStrips/commit/3c0f4ee582deeb287382369f08c9847e679e70c8))
* **ecfmp:** clear mandatory route restriction after strip is cleared ([#256](https://github.com/flightstrips/FlightStrips/issues/256)) ([0b0ae65](https://github.com/flightstrips/FlightStrips/commit/0b0ae65c2d62673c44d66a643c6ae9301638f339))
* **ecfmp:** resolve critical bugs in ECFMP restriction implementation ([9fd6a3e](https://github.com/flightstrips/FlightStrips/commit/9fd6a3e69c84913ab8d9bbf08fe978d58d6dc3fd)), closes [#221](https://github.com/flightstrips/FlightStrips/issues/221)
* **pdc:** use departure atis and omit missing atis letter ([#231](https://github.com/flightstrips/FlightStrips/issues/231)) ([c067286](https://github.com/flightstrips/FlightStrips/commit/c0672867f7aa6b1da2477f4304a06fa4dac4beab))

## [0.34.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.33.0...backend/v0.34.0) (2026-05-25)


### Features

* **frontend,backend:** highlight manual TOBT times ([04cabb1](https://github.com/flightstrips/FlightStrips/commit/04cabb1cd0b3de4d3a8002261e22b71684b17ff9)), closes [#201](https://github.com/flightstrips/FlightStrips/issues/201)


### Bug Fixes

* **backend:** export Go runtime telemetry metrics ([aec5adf](https://github.com/flightstrips/FlightStrips/commit/aec5adf2b7536baf61b54a212601da0db2117e8f))
* **backend:** force online orchestration on ES reconnect ([44d3c55](https://github.com/flightstrips/FlightStrips/commit/44d3c55ecd998c720cd7c116ac0e473db5a7e294))
* **backend:** remove PDC EOBT validation gate ([e27220b](https://github.com/flightstrips/FlightStrips/commit/e27220b06a93d96962eec2519c20eade42f15b7c))
* **backend:** use covered frequencies for PDC departure ([a39a239](https://github.com/flightstrips/FlightStrips/commit/a39a2390da533639f4346fbd6484f86c9d8cc44a))
* **cdm:** batch websocket updates and cap backend memory ([76239a5](https://github.com/flightstrips/FlightStrips/commit/76239a536c8044f039bc2b1e9d388751a4bd3deb))

## [0.33.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.32.0...backend/v0.33.0) (2026-05-17)


### Features

* **acdm:** CDM improvements ([2f857fa](https://github.com/flightstrips/FlightStrips/commit/2f857fac7ef297c155023432c5ec366ed844f496))
* **controlzone:** add CONTROLZONE strip workflow ([49ac51c](https://github.com/flightstrips/FlightStrips/commit/49ac51cb24a0b8e2ab0c78dc68a6f38478067780))

## [0.32.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.31.0...backend/v0.32.0) (2026-05-16)


### Features

* **backend,frontend,plugin:** track client versions ([7c6d509](https://github.com/flightstrips/FlightStrips/commit/7c6d509aac88cb49980bd3fb54fa61ec5e559c5a))


### Bug Fixes

* **backend,plugin:** log websocket close reasons ([7e70d44](https://github.com/flightstrips/FlightStrips/commit/7e70d44435f6dcb4de1bc6e4b49af6b4e5d399d9))
* **backend:** serialize session recalculations ([18a34ad](https://github.com/flightstrips/FlightStrips/commit/18a34adaba4bc52c032d57a6f9f97ef5b8bff24f))

## [0.31.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.30.1...backend/v0.31.0) (2026-05-15)


### Features

* **vacs:** use controller LAN IP for associated frontends ([d1a32b0](https://github.com/flightstrips/FlightStrips/commit/d1a32b0223f4e589774b63982e502e418034d03a))


### Bug Fixes

* **backend:** align EKCH airborne layouts with ground coverage ([12748d2](https://github.com/flightstrips/FlightStrips/commit/12748d204211c91a5bb0bcd7c8b385e300683f69))

## [0.30.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.30.0...backend/v0.30.1) (2026-05-12)


### Bug Fixes

* keep arrivals out of departure bays ([4b26c7b](https://github.com/flightstrips/FlightStrips/commit/4b26c7b59a40f54859409c8a074e0c4f9576a70a))
* recalculate sectors after controller login ([1763692](https://github.com/flightstrips/FlightStrips/commit/176369212b42b1d774ab3adb2a25b71238eb25c1))

## [0.30.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.29.2...backend/v0.30.0) (2026-05-12)


### Features

* **acdm:** respect AOBT ([5b1f5f5](https://github.com/flightstrips/FlightStrips/commit/5b1f5f51ead93ec7908e8bc3feee1b525cdac60d))


### Bug Fixes

* reset strip lifecycle on arrival refiles ([e5dbb46](https://github.com/flightstrips/FlightStrips/commit/e5dbb46c8949cce500e1389a4d92edfb5f094d79))

## [0.29.2](https://github.com/flightstrips/FlightStrips/compare/backend/v0.29.1...backend/v0.29.2) (2026-05-10)


### Bug Fixes

* allow force assume during transfers ([82943e3](https://github.com/flightstrips/FlightStrips/commit/82943e3536da6b30052b29bb70a9ce57252921b0))

## [0.29.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.29.0...backend/v0.29.1) (2026-05-10)


### Bug Fixes

* correct next controller displays ([b8bfabf](https://github.com/flightstrips/FlightStrips/commit/b8bfabfc14feb5671ab1a218489460d369fae535))
* keep hidden departure strips hidden ([51ecad9](https://github.com/flightstrips/FlightStrips/commit/51ecad9c461559ef3255e92a4aed074206e4d14e))
* prefer primary owner for cross-coupled sectors ([3250ecd](https://github.com/flightstrips/FlightStrips/commit/3250ecdfaae46d93417cbdf2030fa3830120c78a))
* require cross-coupled next-sector frequencies ([ac56a2c](https://github.com/flightstrips/FlightStrips/commit/ac56a2c7ab94fff36d50f06deeec100eed205ce0))

## [0.29.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.28.0...backend/v0.29.0) (2026-05-10)


### Features

* **ACDM:** add adverse condition floors ([4c478c8](https://github.com/flightstrips/FlightStrips/commit/4c478c8066e0705df8fd62878242e6a4bdc5e431))
* **ACDM:** add destination spacing ([2ec1dff](https://github.com/flightstrips/FlightStrips/commit/2ec1dffc3c9e292da85d446976544d4aa7c94721))
* **ACDM:** add wake spacing ([d8dd112](https://github.com/flightstrips/FlightStrips/commit/d8dd11289e9c09b3ba15658ab84c3f1a79eca966))
* **ACDM:** broadcast local status updates ([117490c](https://github.com/flightstrips/FlightStrips/commit/117490c1ea09c0a54f28b70caefe31d813222be7))
* **ACDM:** clear local snapshots on slave sync ([2367075](https://github.com/flightstrips/FlightStrips/commit/236707520b5e73ec34be26d868dc5eac966c356a))
* **ACDM:** drop legacy taxi fields ([9fd10c5](https://github.com/flightstrips/FlightStrips/commit/9fd10c50279d2292e5449e449efaf77f123598c2))
* **ACDM:** expand departure CDM engine ([80fe451](https://github.com/flightstrips/FlightStrips/commit/80fe451dfdcc7dfdc3556f03350bed117b91bd3a))
* **ACDM:** harden sync persistence ([1ca74b2](https://github.com/flightstrips/FlightStrips/commit/1ca74b2ba748a74690e4eb0a1622f76c7857f893))
* **ACDM:** prioritize CTOT and sync LVP ([dec7f8e](https://github.com/flightstrips/FlightStrips/commit/dec7f8e9df829210033db044ad0b3334d66c7dd4))
* **ACDM:** recalc master sync changes ([2556bd1](https://github.com/flightstrips/FlightStrips/commit/2556bd1df30190828b738d1ae57402be7b5f1e84))
* **ACDM:** return final CLX TOBT update ([006ff54](https://github.com/flightstrips/FlightStrips/commit/006ff5485ff2bd3e98009521db303a7af64e816e))
* **ACDM:** sync EOBT to EuroScope ([edab28c](https://github.com/flightstrips/FlightStrips/commit/edab28c6e08e0075e0c9ccf2b5504f5bfde2025d))
* **ACDM:** sync EOBT TOBT recalculation ([d7aaf18](https://github.com/flightstrips/FlightStrips/commit/d7aaf1844765315dcf87989dd4bf49db88f5f81b))
* **ACDM:** unify TOBT taxi resolution ([367ab1c](https://github.com/flightstrips/FlightStrips/commit/367ab1cca7be0ba89a4ec774456062be8b15bd70))
* **cdm:** add standalone sequence page ([4ee51da](https://github.com/flightstrips/FlightStrips/commit/4ee51da5fa1b9a46074c9b0f3e3a74f0d97a54f1))
* **SEQ:** startup ready ([72b66d6](https://github.com/flightstrips/FlightStrips/commit/72b66d62d5e3371d6720b793773c0cc0a59e9d27)), closes [#202](https://github.com/flightstrips/FlightStrips/issues/202)
* support sector-aware next display ([a7915f7](https://github.com/flightstrips/FlightStrips/commit/a7915f7b7c5b14464ac260a5e33b5a449a297c2c))


### Bug Fixes

* **ACDM:** enforce cdm_ready flow ([232d19f](https://github.com/flightstrips/FlightStrips/commit/232d19f1a705fc8ee2119da43025137c4dfd4576))
* **ACDM:** restore FlightStrips master recovery ([51908cc](https://github.com/flightstrips/FlightStrips/commit/51908ccf8bd4869f5efa594cef08576cbb2285cf))
* **ACDM:** use ready flow for CdmReady ([91f5b91](https://github.com/flightstrips/FlightStrips/commit/91f5b91e6d16f8d046c7bb9dc20c9b87adbcb350))
* ensure multiple frontend clients get updated ([200acda](https://github.com/flightstrips/FlightStrips/commit/200acda879217a0c9e8440704972490966143b57))

## [0.28.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.27.1...backend/v0.28.0) (2026-05-04)


### Features

* **obs:** track and display master EuroScope client ([7c58fe1](https://github.com/flightstrips/FlightStrips/commit/7c58fe1299c06b039145e331559bab6552eee85e))


### Bug Fixes

* **pdc:** restrict request validations to NOT_CLEARED bay ([b434d5e](https://github.com/flightstrips/FlightStrips/commit/b434d5ee4b914705695deddcc4d5a14d03f78473))
* **pdc:** suppress Hoppie standby for auto-cleared requests ([310f80f](https://github.com/flightstrips/FlightStrips/commit/310f80fc4127361fb32ddcdf9e8052db8bd8bd96))
* **routes:** correct pathing to high alpha stands ([21aa358](https://github.com/flightstrips/FlightStrips/commit/21aa358aa25f5878b4d7313e36f7e45def5c28f7))

## [0.27.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.27.0...backend/v0.27.1) (2026-05-03)


### Bug Fixes

* **cdm:** recalculate on master enable ([17c479b](https://github.com/flightstrips/FlightStrips/commit/17c479b76527106924a61401b3a093c714214a7e))
* **cdm:** sync master TSAT/TTOT exports to vIFF ([4a0fa2a](https://github.com/flightstrips/FlightStrips/commit/4a0fa2a6c28715b71e3f1278f7a1dfc3300e3efd))

## [0.27.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.26.0...backend/v0.27.0) (2026-05-01)


### Features

* **strips:** gate frontend actions on active validation status ([be251cd](https://github.com/flightstrips/FlightStrips/commit/be251cd8c8c3e206df74d43a17e7c2e5a9a04a71))


### Bug Fixes

* correct transfer cancellation initiator rendering and owned-strip ES arrival handover ([6a0429d](https://github.com/flightstrips/FlightStrips/commit/6a0429d95258a93e8f4a8eb8bab95b60589174ad))
* **routes:** add route-scoped owner overrides to resolve GWA transit sector through TE ([1ca1b73](https://github.com/flightstrips/FlightStrips/commit/1ca1b733d3dc0bd735e22f4aaf5cbea2e5a53904))
* **routes:** expand EKCH arrival route coverage for transit sectors and missing stands ([0b71cec](https://github.com/flightstrips/FlightStrips/commit/0b71cec78751b65a374a7222e0c30abe8f7117fb))

## [0.26.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.25.0...backend/v0.26.0) (2026-04-30)


### Features

* **strips:** add CLX dialogue validation ([6e9dac0](https://github.com/flightstrips/FlightStrips/commit/6e9dac0c8ed141cb33ec41246d00c2fcca812c5b))
* **strips:** add editable RNAV capability updates ([b38270e](https://github.com/flightstrips/FlightStrips/commit/b38270e523912442c60df23c8c61166fde1a3099))


### Bug Fixes

* **strips:** allow RNAV heading vectors ([afe8705](https://github.com/flightstrips/FlightStrips/commit/afe8705c6e1b8398a15a7414df83e4c3e62ca27d))
* **strips:** improve CLX validation updates ([9362b27](https://github.com/flightstrips/FlightStrips/commit/9362b279d9a0871d9c54047d14005f012cd2e73b))
* **strips:** resize validation status highlights ([8ad7a71](https://github.com/flightstrips/FlightStrips/commit/8ad7a71c54ccaa65fda076fbb90549516d72468f)), closes [#186](https://github.com/flightstrips/FlightStrips/issues/186)
* **websocket:** prevent stale client metrics ([06148a0](https://github.com/flightstrips/FlightStrips/commit/06148a0a10dd289694e04f51fcc366b5526a01ca))


### Performance Improvements

* **strips:** collapse redundant strip reload queries ([b43848e](https://github.com/flightstrips/FlightStrips/commit/b43848edae406478c4a2d04fbe67acbc387d90f7))

## [0.25.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.24.2...backend/v0.25.0) (2026-04-29)


### Features

* **strips:** add TWR+GND layout ([85b28a5](https://github.com/flightstrips/FlightStrips/commit/85b28a5e1d646e7e886dac6fa01fac78a5db6d72))


### Bug Fixes

* **strips:** preserve airborne sync ([1c09f5f](https://github.com/flightstrips/FlightStrips/commit/1c09f5f56fc0c0b7460c7240e2673569cd3b9be9))
* **strips:** preserve departure lineup sync ([18a2c34](https://github.com/flightstrips/FlightStrips/commit/18a2c349b0d5b0a43ed43c1e14dd1ccfaac3b3c6))


### Performance Improvements

* **strips:** reduce sync query fanout ([49ac666](https://github.com/flightstrips/FlightStrips/commit/49ac6667bf87efcbd76b785696717912dd4e7d9d))

## [0.24.2](https://github.com/flightstrips/FlightStrips/compare/backend/v0.24.1...backend/v0.24.2) (2026-04-28)


### Bug Fixes

* **sectors:** honor airborne owner order ([c30d7ae](https://github.com/flightstrips/FlightStrips/commit/c30d7ae4e68ba038d7d2a4abc0924d2d06230151))
* **strips:** correct final approach region loading ([13ac14a](https://github.com/flightstrips/FlightStrips/commit/13ac14a55c3123fd173ee5f64c1b7d3bdf4a2932))
* **strips:** preserve sequence on no-op sync updates ([5a4ab0b](https://github.com/flightstrips/FlightStrips/commit/5a4ab0bf20ce1f06dd192f7d3aceeaa5793d94ce))


### Performance Improvements

* **strips:** batch EuroScope sync processing ([f1ee21f](https://github.com/flightstrips/FlightStrips/commit/f1ee21fbd64c1e76a820452238d72385d8638e5e))
* **websocket:** reduce strip update db fanout ([f8a6a26](https://github.com/flightstrips/FlightStrips/commit/f8a6a2686cdd673f35e776543de17ea266496fc9))

## [0.24.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.24.0...backend/v0.24.1) (2026-04-27)


### Bug Fixes

* disable broken landing validation ([2df30a3](https://github.com/flightstrips/FlightStrips/commit/2df30a3f45192c29475dfead53fa912a4b8f06e1))

## [0.24.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.23.3...backend/v0.24.0) (2026-04-26)


### Features

* **strips:** add EKCH final approach funnels ([712c9c5](https://github.com/flightstrips/FlightStrips/commit/712c9c537a7519aabcd0444be857241a5a7deb6e))

## [0.23.3](https://github.com/flightstrips/FlightStrips/compare/backend/v0.23.2...backend/v0.23.3) (2026-04-26)


### Bug Fixes

* **strips:** clear arrival runway state on backward move ([b5b7f6c](https://github.com/flightstrips/FlightStrips/commit/b5b7f6cca3f7998e9627abb11e46a5c7e12e4fb5))
* **strips:** correct arrival coordination flow ([56f3669](https://github.com/flightstrips/FlightStrips/commit/56f366959cf8dbf7dea82febd41a3b5f36987243))
* **strips:** keep runway-cleared strips at top of runway bays ([d58dd3a](https://github.com/flightstrips/FlightStrips/commit/d58dd3a3eece1dc5d11b96e426be71be3e83c224))

## [0.23.2](https://github.com/flightstrips/FlightStrips/compare/backend/v0.23.1...backend/v0.23.2) (2026-04-25)


### Bug Fixes

* issues during connect ([8c62635](https://github.com/flightstrips/FlightStrips/commit/8c62635b9e7e19d159f3234ded482c2985874160))
* **sessions:** restrict EKCH sector ownership by callsign prefix ([9792979](https://github.com/flightstrips/FlightStrips/commit/9792979ae60e153abb8a660f249cab51f3ebfcfc))
* **strips:** allow force assume during validation ([8f4ff48](https://github.com/flightstrips/FlightStrips/commit/8f4ff485509501a2994f6256fd3cded33c5dcf20))
* **strips:** correct EKCH runway 12/30 vacate detection ([769c469](https://github.com/flightstrips/FlightStrips/commit/769c4694a134b4f7cdb48a92084ea04e5073b32b))

## [0.23.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.23.0...backend/v0.23.1) (2026-04-25)


### Bug Fixes

* **runway:** scope auto-confirm to same runway only ([1546ea8](https://github.com/flightstrips/FlightStrips/commit/1546ea85e370bb441d788f84d94783565c926f18))
* **strip:** trigger route recalculation when arrival stand is assigned ([febee1d](https://github.com/flightstrips/FlightStrips/commit/febee1d879cbe568f2134657a03c8ff6cefaa297))

## [0.23.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.22.0...backend/v0.23.0) (2026-04-25)


### Features

* observability update ([8c340fa](https://github.com/flightstrips/FlightStrips/commit/8c340fa83de91cc53133270e1f0bb06aa6d2f08a))

## [0.22.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.21.1...backend/v0.22.0) (2026-04-25)


### Features

* allow cross coupled frequencies to change sectors ([14ef576](https://github.com/flightstrips/FlightStrips/commit/14ef576e306cc061c8b46e9c332aa4cb1470f83a))
* duplicate squawk validation ([a556ebc](https://github.com/flightstrips/FlightStrips/commit/a556ebc6c71ba6456c8d184c122d7dd522987944))
* **observer:** add read-only observer mode ([f919dc4](https://github.com/flightstrips/FlightStrips/commit/f919dc49a259610def9366037f4a8e8075d3c555))
* **plugin-sync:** alert runway config mismatches ([e0363ad](https://github.com/flightstrips/FlightStrips/commit/e0363ad35be80a47a2d96d90212916cfd02f274e))
* **validation:** add ctot validation ([e8380c9](https://github.com/flightstrips/FlightStrips/commit/e8380c9419062edab2453cdcd22beacd4020ecd7))
* **validation:** add custom pdc validation ([ea84795](https://github.com/flightstrips/FlightStrips/commit/ea847950d6e3ff992af005a2e0833557519c9cfc))
* **validation:** add invalid pdc validation ([607b354](https://github.com/flightstrips/FlightStrips/commit/607b35444f6a443cdba85200df0c27855a4ec59c))
* **validation:** add landing clearance validation ([61429e5](https://github.com/flightstrips/FlightStrips/commit/61429e5120d434a6727b02fb4f25172bc9ca9169))
* **validation:** add no-stand validation ([0d20fe4](https://github.com/flightstrips/FlightStrips/commit/0d20fe4bb62b13bd44d95af3d5e491b263e9f4cc))
* **validation:** add runway type validation ([5e57b72](https://github.com/flightstrips/FlightStrips/commit/5e57b7282852597a13ec4e5d94c19edbb4054b9c))
* **validation:** add taxiway type validation ([dc62335](https://github.com/flightstrips/FlightStrips/commit/dc6233593b4d0dedd82febbc54c358edae9b75b4))
* wrong squawk validation ([026a3cc](https://github.com/flightstrips/FlightStrips/commit/026a3cc1803b925367317c25ae712df3d3d484e4))


### Bug Fixes

* block departure strips in arrival bays ([535ef92](https://github.com/flightstrips/FlightStrips/commit/535ef92d0b90fff078fe16e3976591ee953e03ec))
* **sync:** preserve advanced bays during blank failover sync ([6b8b344](https://github.com/flightstrips/FlightStrips/commit/6b8b344c77d3b0fa3e8757957ea40fd4f9ce3ecc))
* **sync:** preserve blank failover route identity ([6d24e0a](https://github.com/flightstrips/FlightStrips/commit/6d24e0ad20456da1372ae3b7b5970a3f68a2ae5f))
* use online position for master on vIFF network ([847a7a4](https://github.com/flightstrips/FlightStrips/commit/847a7a407ede06a31c4d59cbba4d886ec1e74fee))

## [0.21.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.21.0...backend/v0.21.1) (2026-04-23)


### Bug Fixes

* **backend:** suppress stale controller offline events ([3050042](https://github.com/flightstrips/FlightStrips/commit/3050042d22a45e90a0f032b18bf06acf97b282f3))
* misleading error log when controller log off ([010e746](https://github.com/flightstrips/FlightStrips/commit/010e746152f0dc613571365cb3318600884f2df1))
* **pdc:** stabilize clearance confirmation sync ([27e8314](https://github.com/flightstrips/FlightStrips/commit/27e83142aaed4fa0b3fe938ec530bcb8164fec4b))

## [0.21.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.20.0...backend/v0.21.0) (2026-04-21)


### Features

* new metrics ([69aa8ef](https://github.com/flightstrips/FlightStrips/commit/69aa8ef1d7fef2e874b042e6f0ef5bb4bcb00274))

## [0.20.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.19.1...backend/v0.20.0) (2026-04-18)


### Features

* add validation status framework (task-146-00) ([222d7bb](https://github.com/flightstrips/FlightStrips/commit/222d7bbe605c2395c1f02f8dc7836e3fafc7b25d))


### Bug Fixes

* observability ([9999859](https://github.com/flightstrips/FlightStrips/commit/99998599ab62b7f10747371cc57a981e3280e0a4))

## [0.19.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.19.0...backend/v0.19.1) (2026-04-18)


### Bug Fixes

* frontend client metrics ([d246ed1](https://github.com/flightstrips/FlightStrips/commit/d246ed1f34c09a49bb19c40f31d7cadf5c4439f1))

## [0.19.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.18.0...backend/v0.19.0) (2026-04-17)


### Features

* add web PDC flow ([237f75a](https://github.com/flightstrips/FlightStrips/commit/237f75a4fb7e50bf19580e54528b288a3f50531f))
* observability ([38d2669](https://github.com/flightstrips/FlightStrips/commit/38d26692d30322f12d9ca87b42c084588d5f37d9))


### Bug Fixes

* aircraft disconnected to debug ([354c895](https://github.com/flightstrips/FlightStrips/commit/354c895a69050e1a85a01a04ca0f1e5d0a933db9))

## [0.18.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.17.2...backend/v0.18.0) (2026-04-16)


### Features

* top down coverage ([48ddb6e](https://github.com/flightstrips/FlightStrips/commit/48ddb6ed52e1a1848093437aeae86417b0d89bfb))

## [0.17.2](https://github.com/flightstrips/FlightStrips/compare/backend/v0.17.1...backend/v0.17.2) (2026-04-14)


### Bug Fixes

* arrivals going to D_TWR when C_TWR is not online ([3260d1d](https://github.com/flightstrips/FlightStrips/commit/3260d1d3b133dd58f925dc2d2a00570eeeb8710a))
* do not delete aircraft due to reconect ([3bcb27c](https://github.com/flightstrips/FlightStrips/commit/3bcb27c278a7f17f9745dbc61d347184c31012bc))

## [0.17.1](https://github.com/flightstrips/FlightStrips/compare/backend/v0.17.0...backend/v0.17.1) (2026-04-13)


### Bug Fixes

* pdc not working ([8165dbf](https://github.com/flightstrips/FlightStrips/commit/8165dbf07df367bc09dc89626756dc2e3ff64855))

## [0.17.0](https://github.com/flightstrips/FlightStrips/compare/backend/v0.16.0...backend/v0.17.0) (2026-04-12)


### Features

* auto generate squawk for new strips ([8de203d](https://github.com/flightstrips/FlightStrips/commit/8de203d07c47f8b652879029d0ff229391301b19))
* CLR / PDC tag items and funnctions ([13a5f6e](https://github.com/flightstrips/FlightStrips/commit/13a5f6ebe590091812c9552ecfd054cdbfeafe5c))
* pdc remarks ([8179aea](https://github.com/flightstrips/FlightStrips/commit/8179aea9b93d8540d8041a12052566710a017b14))
* sync go around sound ([b4f6482](https://github.com/flightstrips/FlightStrips/commit/b4f64827dc7dd6dbbf42216c595bc64372030e6c))


### Bug Fixes

* departure frequency for PDCs ([cf6e858](https://github.com/flightstrips/FlightStrips/commit/cf6e858160b9ab8b4e928e5ce501e87b75206cb0))
* pdc setting clear flag ([25dfecd](https://github.com/flightstrips/FlightStrips/commit/25dfecd0c133904c2ec4dd30f7043b7ea0994d0b))
* wrong owner order for K_DEP ([2a4190e](https://github.com/flightstrips/FlightStrips/commit/2a4190ee58ce59a25163562f8104a193464c853c))

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
