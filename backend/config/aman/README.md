# AMAN terminal configuration

`ekch-terminal-2611.json` carries the operator-approved STAR-family and feeder-fix mappings from #553 and #605.

AIRAC 2611 applies from 29 October 2026 until 26 November 2026 (exclusive). Compared the existing configuration with Naviair's [AIRAC AMDT 11/26](https://aim.naviair.dk/media/files/ct2vgpz44h2/EK_Amdt_A_2026_11_en.pdf), [EKCH aerodrome text](https://aim.naviair.dk/media/files/3bd5mzetodu/EK_AD_2_EKCH_en.pdf), [aerodrome chart](https://aim.naviair.dk/media/files/uhy2wmfxmo2/EK_AD_2_EKCH_ADC_en.pdf), and updated RNAV STAR tabular charts for [04](https://aim.naviair.dk/media/files/3n5oclptsoa/EK_AD_2_EKCH_RNAV_STAR_04_L_R_3_en.pdf), [22](https://aim.naviair.dk/media/files/hjrskkheed0/EK_AD_2_EKCH_RNAV_STAR_22_L_R_3_en.pdf), [12](https://aim.naviair.dk/media/files/iapc1jhpfbd/EK_AD_2_EKCH_RNAV_STAR_12_3_en.pdf), and [30](https://aim.naviair.dk/media/files/gw1d052hbc1/EK_AD_2_EKCH_RNAV_STAR_30_3_en.pdf). Magnetic variation changed to 5° E, changing published magnetic runway directions, holding tracks, and STAR continuation headings. The configured true courses, runway geometry and holding characteristics are unchanged. The ERNOV runway 22 path now follows the published CH647, CH637, CH631 and CH626 routing; other path mappings and operator-approved durations are retained. The STARs end in radar vectoring; the subsequent connection to an ILS final fix is a planning approximation, not a published or assigned clearance.

The nominal holding-to-feeder duration is deliberately absent for these paths because no approved value is available:

- `MONAK/ARRIVAL-30`: `OLPIB` to `KUBIS`
- `TIDVU/ARRIVAL-12`: `TIDVU` to `WUPJA`
- `TIDVU/ARRIVAL-30`: `TIDVU` to `WUPJA`

TODO(#553): add `holdingToFeederSeconds` only after an operator supplies the missing values. Do not substitute zero, another runway's duration, or another ETA.


## Approach admission policy

`approachRelease` configures #887 independently from landing throughput. EKCH uses `targetBuffer: 10`, `taperStart: 6`, `maxAdvanceSeconds: 600`, and `lookaheadSeconds: 120`. These counts are desired buffer settings, not airspace hard limits. Unreleased holds are excluded; outstanding imminent/overdue releases consume the buffer before their aircraft enter the TMA. Disabling the policy stops new advances and retains existing release commitments.

Deploy reservation-aware AMAN servers before enabling the policy on every server. Older V1 clients can ignore the optional buffer and additional-approach fields. No migration is required: reservations and the session buffer use the existing persisted JSON state.

For recorded VATSIM validation, run `go run ./cmd/aman-accuracy -input <recording-directory> -cache-only -minimum-update-interval 1m -release-buffer -release-metrics <report.json>`. Pin the same navigation-cache database and configuration for both runs. Omit `-release-buffer` for a disabled-policy comparison. The API recordings contain no authoritative holding/direct clearances, so reports compare planned targets and observed traffic, not actual compliance with hypothetical EATs. The replay retains grounded observations through the following calculation tick.
