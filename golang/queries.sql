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

-- name: RemoveStripByID :exec
DELETE FROM strips WHERE id = $1;

-- name: ListStripsByOrigin :many
SELECT * FROM strips WHERE origin = $1 ORDER BY id;

