-- Keep the persisted AMAN projection aligned with observation identities:
-- callsign is the active-flight identity and VATSIM CID is supporting metadata.
UPDATE aman_flights
SET current_callsign = upper(trim(current_callsign));

-- Older projections may contain more than one active row for a callsign. Keep
-- the newest projection; completed history remains available in audit records.
WITH ranked AS (
    SELECT flight_id,
           row_number() OVER (
               PARTITION BY current_callsign
               ORDER BY updated_at DESC, flight_id DESC
           ) AS rank
    FROM aman_flights
    WHERE state <> 'removed'
)
DELETE FROM aman_flights AS flight
USING ranked
WHERE flight.flight_id = ranked.flight_id AND ranked.rank > 1;

DROP INDEX ux_aman_flights_active_vatsim_cid;
CREATE UNIQUE INDEX ux_aman_flights_active_callsign
    ON aman_flights (current_callsign)
    WHERE state <> 'removed';
