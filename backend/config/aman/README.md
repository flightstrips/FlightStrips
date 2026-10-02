# AMAN terminal configuration

`ekch-terminal-2610.json` carries the operator-approved STAR-family and feeder-fix mappings from #553 and #605.

AIRAC 2610 applies from 1 October 2026 until 29 October 2026 (exclusive). Reviewed against Naviair's [AIRAC AMDT 10/26](https://aim.naviair.dk/media/files/wqojbp2c2gn/EK_Amdt_A_2026_10_en.pdf), [current EKCH aerodrome chart](https://aim.naviair.dk/media/files/juczxdemgih/EK_AD_2_EKCH_ADC_en.pdf), and official publication index on 2 October 2026. The EKCH amendment changes deicing areas and taxiway holding-position/stopline symbols; the configured STAR paths, airborne holdings, runway dimensions, threshold positions, and GEO courses remain unchanged. Runway provenance now references the 10/26 chart; retained STAR and holding sources keep their original effective dates.

The nominal holding-to-feeder duration is deliberately absent for these paths because no approved value is available:

- `MONAK/ARRIVAL-30`: `OLPIB` to `KUBIS`
- `TIDVU/ARRIVAL-12`: `TIDVU` to `WUPJA`
- `TIDVU/ARRIVAL-30`: `TIDVU` to `WUPJA`

TODO(#553): add `holdingToFeederSeconds` only after an operator supplies the missing values. Do not substitute zero, another runway's duration, or another ETA.
