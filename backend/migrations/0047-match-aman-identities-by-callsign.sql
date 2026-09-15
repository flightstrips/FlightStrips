-- Keep one active binding per network callsign. Retain older identities for
-- history; reconciliation removes their superseded AMAN aggregates.
UPDATE aman_vatsim_observation_identities
SET current_callsign = upper(trim(current_callsign));

WITH ranked AS (
    SELECT flight_id,
           row_number() OVER (
               PARTITION BY current_callsign
               ORDER BY updated_at DESC, created_at DESC, flight_id DESC
           ) AS rank
    FROM aman_vatsim_observation_identities
    WHERE retired_at IS NULL
)
UPDATE aman_vatsim_observation_identities AS identity
SET retired_at = NOW(), updated_at = NOW()
FROM ranked
WHERE identity.flight_id = ranked.flight_id AND ranked.rank > 1;

DROP INDEX ux_aman_vatsim_observation_identities_active_cid;
CREATE UNIQUE INDEX ux_aman_vatsim_observation_identities_active_callsign
    ON aman_vatsim_observation_identities (current_callsign)
    WHERE retired_at IS NULL;
