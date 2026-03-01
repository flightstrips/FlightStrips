-- name: InsertStrip :exec
INSERT INTO strips (version, callsign, session, origin, destination, alternative, route, remarks, assigned_squawk,
                    squawk, sid, cleared_altitude, heading, aircraft_type, runway, requested_altitude, capabilities,
                    communication_type, aircraft_category, stand, sequence, state, cleared, owner, bay,
                    position_latitude, position_longitude, position_altitude, tobt, eobt, registration)
VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23,
        $24, $25, $26, $27, $28, $29, $30);

-- name: UpdateStrip :execrows
UPDATE strips
SET (version, origin, destination, alternative, route, remarks, assigned_squawk, squawk, sid, cleared_altitude,
     heading, aircraft_type, runway, requested_altitude, capabilities, communication_type, aircraft_category, stand,
     sequence, state, cleared, owner, bay, position_latitude, position_longitude, position_altitude, tobt, eobt,
     registration
    ) = (
         version + 1, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22,
         $23, $24, $25, $26, $27, $28, $29, $30)
WHERE callsign = $1 AND session = $2;

-- name: GetStrip :one
SELECT *
FROM strips
WHERE callsign = $1 AND session = $2;

-- name: ListStrips :many
SELECT *
FROM strips
WHERE session = $1
ORDER BY callsign;

-- name: UpdateStripSquawkByID :execrows
UPDATE strips
SET squawk  = $1,
    version = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripAssignedSquawkByID :execrows
UPDATE strips
SET assigned_squawk = $1,
    version         = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripClearedAltitudeByID :execrows
UPDATE strips
SET cleared_altitude = $1,
    version          = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripRequestedAltitudeByID :execrows
UPDATE strips
SET requested_altitude = $1,
    version            = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripCommunicationTypeByID :execrows
UPDATE strips
SET communication_type = $1,
    version            = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripGroundStateByID :execrows
UPDATE strips
SET state   = $1,
    bay     = $2,
    version = version + 1
WHERE callsign = $3 AND session = $4 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripClearedFlagByID :execrows
UPDATE strips
SET cleared = $1,
    bay     = $2,
    version = version + 1
WHERE callsign = $3 AND session = $4 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripAircraftPositionByID :execrows
UPDATE strips
SET position_latitude  = $1,
    position_longitude = $2,
    position_altitude  = $3,
    bay                = $4
WHERE callsign = $5 AND session = $6 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripHeadingByID :execrows
UPDATE strips
SET heading = $1,
    version = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripStandByID :execrows
UPDATE strips
SET stand   = $1,
    version = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: RemoveStripByID :exec
DELETE
FROM strips
WHERE callsign = $1 AND session = $2;

-- name: ListStripsByOrigin :many
SELECT *
FROM strips
WHERE origin = $1 AND session = $2
ORDER BY callsign;

-- name: SetStripOwner :execrows
UPDATE strips
SET owner   = $1,
    version = version + 1
WHERE callsign = $2 AND session = $3 AND version = $4;

-- name: SetNextOwners :exec
UPDATE strips SET next_owners = $3 WHERE session = $1 AND callsign = $2;

-- name: SetNextAndPreviousOwners :exec
UPDATE strips SET next_owners = $3, previous_owners = $4 WHERE session = $1 AND callsign = $2;

-- name: UpdateStripSequence :execrows
UPDATE strips
SET sequence = sqlc.arg(sequence)::INT,
    version  = version + 1
WHERE session = $1 AND callsign = $2;

-- name: UpdateStripSequenceBulk :exec
UPDATE strips AS s
SET sequence = u.new_sequence,
    version  = s.version + 1
FROM (SELECT unnest(sqlc.arg(callsigns)::TEXT[]) AS callsign, unnest(sqlc.arg(sequences)::INT[]) AS new_sequence) AS u
WHERE s.session = $1 AND s.callsign = u.callsign;

-- name: RecalculateStripSequences :exec
WITH row_numbers AS (
    SELECT s.id, (ROW_NUMBER() OVER (ORDER BY s.sequence, s.callsign)) * sqlc.arg(spacing)::INT AS new_sequence
    FROM strips as s
    WHERE s.session = $1 AND s.bay = sqlc.arg(bay)::TEXT
)
UPDATE strips
SET sequence = row_numbers.new_sequence,
    version  = strips.version + 1
FROM row_numbers
WHERE strips.id = row_numbers.id AND strips.session = $1;

-- name: ListStripSequences :many
SELECT callsign, sequence
FROM strips
WHERE session = $1 AND bay = sqlc.arg(bay)::TEXT
ORDER BY sequence, callsign;

-- name: GetSequence :one
SELECT sequence::INT
FROM strips
WHERE session = $1 AND callsign = $2 AND bay = sqlc.arg(bay)::TEXT
LIMIT 1;

-- name: GetMaxSequenceInBay :one
SELECT COALESCE(max(sequence), 0)::INTEGER AS max_sequence
FROM strips
WHERE session = $1 AND bay = sqlc.arg(bay)::TEXT;

-- name: GetMinSequenceInBay :one
SELECT COALESCE(min(sequence), 0)::INTEGER AS min_sequence
FROM strips
WHERE session = $1 AND bay = sqlc.arg(bay)::TEXT;

-- name: GetNextSequence :one
SELECT sequence::INT
FROM strips
WHERE session = $1 AND bay = sqlc.arg(bay)::TEXT AND sequence > sqlc.arg(sequence)::INT;

-- name: GetBay :one
SELECT bay
FROM strips
WHERE session = $1 AND callsign = $2;

-- name: SetPreviousOwners :exec
UPDATE strips SET previous_owners = $3 WHERE session = $1 AND callsign = $2;

-- name: SetCdmStatus :execrows
UPDATE strips SET cdm_status = $3 WHERE session = $1 AND callsign = $2;

-- name: GetCdmData :many
SELECT callsign, tobt, tsat, ttot, ctot, aobt, asat, eobt, cdm_status FROM strips WHERE session = $1;

-- name: GetCdmDataForCallsign :one
SELECT callsign, tobt, tsat, ttot, ctot, aobt, asat, eobt, cdm_status FROM strips WHERE session = $1 and callsign = $2;

-- name: UpdateCdmData :execrows
UPDATE strips SET tobt = $3, tsat = $4, ttot = $5, ctot = $6, aobt = $7, eobt = $8, cdm_status = $9
              WHERE session = $1 AND callsign = $2;

-- name: UpdateReleasePoint :execrows
UPDATE strips SET release_point = $3 WHERE session = $1 AND callsign = $2;

-- name: UpdateStripMarkedByID :execrows
UPDATE strips
SET marked  = $1,
    version = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripRegistration :execrows
UPDATE strips
SET registration = $1,
    version      = version + 1
WHERE callsign = $2 AND session = $3;