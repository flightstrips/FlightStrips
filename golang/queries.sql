-- name: InsertController :one
INSERT INTO controllers (
    cid, airport, position
) VALUES (
             $1, $2, $3
         )
RETURNING *;

-- name: ListControllers :many
SELECT * FROM controllers ORDER BY airport;

-- name: ListControllersByAirport :many
SELECT * FROM controllers WHERE airport = $1;

-- name: RemoveController :exec
DELETE FROM controllers WHERE cid = $1;

-- name: InsertStrip :exec
INSERT INTO strips (
    id, origin, destination, alternative, route, remarks, assigned_squawk, squawk, sid, cleared_altitude, heading, aircraft_type, runway, requested_altitude, capabilities, communication_type, aircraft_category, stand, sequence, state, cleared, positionFrequency, position_latitude, position_longitude, position_altitude, tobt, tsat, ttot, ctot, aobt, asat
) VALUES (
             $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31
         );

-- name: UpdateStripByID :exec
UPDATE strips SET (
        origin, destination, alternative, route, remarks, assigned_squawk, squawk, sid, cleared_altitude, heading, aircraft_type, runway, requested_altitude, capabilities, communication_type, aircraft_category, stand, sequence, state, cleared, positionFrequency, position_latitude, position_longitude, position_altitude, tobt, tsat, ttot, ctot, aobt, asat
) = (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30
    ) WHERE id = $31;

-- name: UpdateStripSquawkByID :exec
UPDATE strips SET squawk = $1 WHERE id = $2;

-- name: UpdateStripAssignedSquawkByID :exec
UPDATE strips SET assigned_squawk = $1 WHERE id = $2;

-- name: UpdateStripClearedAltitudeByID :exec
UPDATE strips SET cleared_altitude = $1 WHERE id = $2;

-- name: UpdateStripRequestedAltitudeByID :exec
UPDATE strips SET requested_altitude = $1 WHERE id = $2;

-- name: UpdateStripCommunicationTypeByID :exec
UPDATE strips SET communication_type = $1 WHERE id = $2;

-- name: UpdateStripGroundStateByID :exec
UPDATE strips SET state = $1 WHERE id = $2;

-- name: UpdateStripClearedFlagByID :exec
UPDATE strips SET cleared = $1 WHERE id = $2;

-- name: UpdateStripAircraftPositionByID :exec
UPDATE strips SET position_latitude = $1, position_longitude = $2, position_altitude = $3 WHERE id = $4;

-- name: UpdateStripHeadingByID :exec
UPDATE strips SET heading = $1 WHERE id = $2;

-- name: UpdateStripStandByID :exec
UPDATE strips SET stand = $1 WHERE id = $2;

-- name: RemoveStripByID :exec
DELETE FROM strips WHERE id = $1;

-- name: ListStripsByOrigin :many
SELECT * FROM strips WHERE origin = $1 ORDER BY id;

-- name: InsertIntoEvents :exec
INSERT INTO events (
    type, timestamp, cid, data
) VALUES (
             $1, $2, $3, $4
         );

-- name: ListEvents :many
SELECT * FROM events ORDER BY timestamp;

