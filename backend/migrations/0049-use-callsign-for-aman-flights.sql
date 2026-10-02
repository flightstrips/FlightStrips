-- AMAN identifies flights exclusively by normalized callsign within an airport.
-- Existing AMAN sessions use an incompatible generated-identity payload shape.
-- They are intentionally not carried across this migration; live source data
-- rebuilds the projection after startup.
DELETE FROM aman_coordination_requests;
DELETE FROM aman_airport_states;

-- Remove the previous generated identity and VATSIM-correlation persistence.
ALTER TABLE aman_flights ADD COLUMN IF NOT EXISTS callsign VARCHAR;

DROP INDEX IF EXISTS ux_aman_flights_active_callsign;
DROP INDEX IF EXISTS ux_aman_flights_active_vatsim_cid;
DROP INDEX IF EXISTS idx_aman_flights_airport;
ALTER TABLE aman_flights DROP CONSTRAINT IF EXISTS aman_flights_pkey;
ALTER TABLE aman_flights ALTER COLUMN callsign SET NOT NULL;
ALTER TABLE aman_flights DROP COLUMN IF EXISTS flight_id;
ALTER TABLE aman_flights DROP COLUMN IF EXISTS vatsim_cid;
ALTER TABLE aman_flights DROP COLUMN IF EXISTS current_callsign;
ALTER TABLE aman_flights ADD PRIMARY KEY (airport, callsign);
CREATE INDEX idx_aman_flights_airport ON aman_flights (airport, callsign);

DROP TABLE IF EXISTS aman_vatsim_observation_identities;
DROP TABLE IF EXISTS aman_flight_identities;
