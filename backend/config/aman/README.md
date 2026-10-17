# AMAN terminal configuration

`ekch-terminal-2610.json` carries the operator-approved STAR-family and feeder-fix mappings from #553 and #605.

AIRAC 2610 applies from 1 October 2026 until 29 October 2026 (exclusive). Reviewed against Naviair's [AIRAC AMDT 10/26](https://aim.naviair.dk/media/files/wqojbp2c2gn/EK_Amdt_A_2026_10_en.pdf), [current EKCH aerodrome chart](https://aim.naviair.dk/media/files/juczxdemgih/EK_AD_2_EKCH_ADC_en.pdf), and official publication index on 2 October 2026. The EKCH amendment changes deicing areas and taxiway holding-position/stopline symbols; the configured STAR paths, airborne holdings, runway dimensions, threshold positions, and GEO courses remain unchanged. Runway provenance now references the 10/26 chart; retained STAR and holding sources keep their original effective dates.

The nominal holding-to-feeder duration is deliberately absent for these paths because no approved value is available:

- `MONAK/ARRIVAL-30`: `OLPIB` to `KUBIS`
- `TIDVU/ARRIVAL-12`: `TIDVU` to `WUPJA`
- `TIDVU/ARRIVAL-30`: `TIDVU` to `WUPJA`

TODO(#553): add `holdingToFeederSeconds` only after an operator supplies the missing values. Do not substitute zero, another runway's duration, or another ETA.


## Approach admission policy

`approachRelease` configures #887 independently from landing throughput. EKCH uses `targetBuffer: 10`, `taperStart: 6`, `maxAdvanceSeconds: 600`, and `lookaheadSeconds: 120`. These counts are desired buffer settings, not airspace hard limits. Unreleased holds are excluded; outstanding imminent/overdue releases consume the buffer before their aircraft enter the TMA. Disabling the policy stops new advances and retains existing release commitments.

Deploy reservation-aware AMAN servers before enabling the policy on every server. Older V1 clients can ignore the optional buffer and additional-approach fields. No migration is required: reservations and the session buffer use the existing persisted JSON state.

For recorded VATSIM validation, run `go run ./cmd/aman-accuracy -input <recording-directory> -cache-only -minimum-update-interval 1m -release-buffer -release-metrics <report.json>`. Pin the same navigation-cache database and configuration for both runs. Omit `-release-buffer` for a disabled-policy comparison. The API recordings contain no authoritative holding/direct clearances, so reports compare planned targets and observed traffic, not actual compliance with hypothetical EATs. The replay retains grounded observations through the following calculation tick.
