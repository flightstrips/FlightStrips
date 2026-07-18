-- name: LockAMANCommand :exec
SELECT pg_advisory_xact_lock(hashtextextended($1, 0));

-- name: GetAMANAirportState :one
SELECT *
FROM aman_airport_states
WHERE airport = $1;

-- name: UpsertAMANAirportState :one
INSERT INTO aman_airport_states (
    airport, revision, generated_at, policy_version, mode, authoritative, runway_groups
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (airport) DO UPDATE
SET revision = EXCLUDED.revision,
    generated_at = EXCLUDED.generated_at,
    policy_version = EXCLUDED.policy_version,
    mode = EXCLUDED.mode,
    authoritative = EXCLUDED.authoritative,
    runway_groups = EXCLUDED.runway_groups,
    updated_at = NOW()
WHERE aman_airport_states.revision = $8
RETURNING *;

-- name: ListAMANFlights :many
SELECT *
FROM aman_flights
WHERE airport = $1
ORDER BY flight_id;

-- name: DeleteAMANFlightsForAirport :exec
DELETE FROM aman_flights
WHERE airport = $1;

-- name: UpsertAMANFlight :exec
INSERT INTO aman_flights (
    flight_id, airport, vatsim_cid, current_callsign, state, data_status, updated_at, payload
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (flight_id) DO UPDATE
SET airport = EXCLUDED.airport,
    vatsim_cid = EXCLUDED.vatsim_cid,
    current_callsign = EXCLUDED.current_callsign,
    state = EXCLUDED.state,
    data_status = EXCLUDED.data_status,
    updated_at = EXCLUDED.updated_at,
    payload = EXCLUDED.payload;

-- name: GetAMANCommandOutcome :one
SELECT *
FROM aman_command_outcomes
WHERE command_id = $1;

-- name: CreateAMANCommandOutcome :exec
INSERT INTO aman_command_outcomes (command_id, airport, revision, payload, recorded_at)
VALUES ($1, $2, $3, $4, $5);

-- name: CreateAMANAuditRecord :exec
INSERT INTO aman_audit_records (airport, revision, category, payload, recorded_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ListAMANAuditRecords :many
SELECT *
FROM aman_audit_records
WHERE airport = $1
ORDER BY revision, id;

-- name: CreateAMANValidationEvidence :exec
INSERT INTO aman_validation_evidence (evidence_id, airport, kind, payload, recorded_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ListAMANValidationEvidence :many
SELECT *
FROM aman_validation_evidence
WHERE airport = $1
ORDER BY evidence_id;
