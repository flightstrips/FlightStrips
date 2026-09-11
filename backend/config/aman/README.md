# AMAN terminal configuration

`ekch-terminal-2609.json` carries the operator-approved STAR-family and feeder-fix mappings from #553 and #605.

The nominal holding-to-feeder duration is deliberately absent for these paths because no approved value is available:

- `MONAK/ARRIVAL-30`: `OLPIB` to `KUBIS`
- `TIDVU/ARRIVAL-12`: `TIDVU` to `WUPJA`
- `TIDVU/ARRIVAL-30`: `TIDVU` to `WUPJA`

TODO(#553): add `holdingToFeederSeconds` only after an operator supplies the missing values. Do not substitute zero, another runway's duration, or another ETA.
