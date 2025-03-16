-- name: InsertController :exec
INSERT INTO controllers (
    callsign, airport, position, master, connected
) VALUES (
             $1, $2, $3, $4, $5
         );

-- name: BulkInsertControllers :copyfrom
INSERT INTO controllers (
    callsign, airport, position, master, connected
) VALUES (
             $1, $2, $3, $4, $5
         );

-- name: UpdateController :execrows
UPDATE controllers SET (position, master, connected) = ($1, $2, $3) WHERE callsign = $4;

-- name: ListControllers :many
SELECT * FROM controllers ORDER BY airport;

-- name: ListControllersByAirport :many
SELECT * FROM controllers WHERE airport = $1;

-- name: RemoveController :execrows
DELETE FROM controllers WHERE callsign = $1;

-- name: GetController :one
SELECT * FROM controllers WHERE callsign = $1;

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

-- name: UpdateStripSquawkByID :execrows
UPDATE strips SET squawk = $1 WHERE id = $2;

-- name: UpdateStripAssignedSquawkByID :execrows
UPDATE strips SET assigned_squawk = $1 WHERE id = $2;

-- name: UpdateStripClearedAltitudeByID :execrows
UPDATE strips SET cleared_altitude = $1 WHERE id = $2;

-- name: UpdateStripRequestedAltitudeByID :execrows
UPDATE strips SET requested_altitude = $1 WHERE id = $2;

-- name: UpdateStripCommunicationTypeByID :execrows
UPDATE strips SET communication_type = $1 WHERE id = $2;

-- name: UpdateStripGroundStateByID :execrows
UPDATE strips SET state = $1 WHERE id = $2;

-- name: UpdateStripClearedFlagByID :execrows
UPDATE strips SET cleared = $1 WHERE id = $2;

-- name: UpdateStripAircraftPositionByID :execrows
UPDATE strips SET position_latitude = $1, position_longitude = $2, position_altitude = $3 WHERE id = $4;

-- name: UpdateStripHeadingByID :execrows
UPDATE strips SET heading = $1 WHERE id = $2;

-- name: UpdateStripStandByID :execrows
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

